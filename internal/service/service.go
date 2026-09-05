package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/t0mer/linkmeta/internal/cache"
	"github.com/t0mer/linkmeta/internal/config"
	"github.com/t0mer/linkmeta/internal/extract"
	"github.com/t0mer/linkmeta/internal/fetch"
	"github.com/t0mer/linkmeta/internal/llm"
	"github.com/t0mer/linkmeta/internal/metrics"
)

// Response is the frozen /extract output.
type Response struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Keywords    []string `json:"keywords"`
	// CacheStatus is transport metadata (HIT/MISS/BYPASS/DISABLED) surfaced as
	// the X-Cache header. It is excluded from the JSON body, which is frozen.
	CacheStatus string `json:"-"`
}

// Fetcher abstracts the page fetcher (real one is *fetch.Fetcher).
type Fetcher interface {
	Fetch(ctx context.Context, url string) (fetch.Result, error)
}

// Service orchestrates extraction.
type Service struct {
	cfg config.Config
	f   Fetcher
	llm llm.Client
	log *slog.Logger

	// Last LLM failure, surfaced by /healthz. Degradation is silent by design
	// (the caller still gets metadata), so without this a broken model looks
	// healthy from the outside.
	mu        sync.RWMutex
	lastErr   string
	lastErrAt time.Time

	// cache is optional; nil means caching is off.
	cache       cache.Store
	fingerprint string
}

// SetCache attaches a cache store. Passing nil disables caching.
func (s *Service) SetCache(store cache.Store) {
	s.cache = store
	s.fingerprint = s.cfg.CacheFingerprint()
}

// New builds a Service.
func New(cfg config.Config, f Fetcher, l llm.Client, log *slog.Logger) *Service {
	return &Service{cfg: cfg, f: f, llm: l, log: log}
}

// ValidateURL enforces http/https scheme and non-empty host.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("host is empty")
	}
	return nil
}

// Extract runs the full pipeline. lang overrides the configured category
// language for this request (blank uses the configured default). Returns an
// error ONLY when the fetch fails; LLM failure degrades gracefully to "Other".
func (s *Service) Extract(ctx context.Context, rawURL, lang string, fresh bool) (Response, error) {
	key, status := s.cacheKey(rawURL, lang), "DISABLED"
	if key != "" {
		if fresh {
			status = "BYPASS"
			metrics.CacheTotal.WithLabelValues("bypass").Inc()
		} else if cached, ok := s.cacheGet(ctx, key); ok {
			cached.CacheStatus = "HIT"
			return cached, nil
		} else {
			status = "MISS"
		}
	}

	res, err := s.f.Fetch(ctx, rawURL)
	if err != nil {
		s.log.Warn("fetch failed", "url", rawURL, "stage", "fetch", "err", err)
		return Response{}, fmt.Errorf("fetch: %w", err)
	}

	meta, err := extract.ParseMeta(res.HTML)
	if err != nil {
		s.log.Warn("meta parse error", "url", rawURL, "stage", "parse", "err", err)
	}

	// ForceLLM ignores what the page supplied: ask the model for both fields so
	// its answers win the merge below. Title always stays deterministic.
	needDesc := meta.Description == "" || s.cfg.ForceLLM
	needKeywords := len(meta.Keywords) == 0 || s.cfg.ForceLLM

	llmReq := llm.Request{
		Title:            meta.Title,
		Description:      meta.Description,
		NeedDescription:  needDesc,
		NeedKeywords:     needKeywords,
		Categories:       s.cfg.Categories,
		CategoryLanguage: s.effectiveLang(lang),
	}
	// Only pay for readable-text extraction when the model needs page content.
	if needDesc || needKeywords {
		txt := extract.ReadableText(res.HTML, res.FinalURL)
		llmReq.Text = extract.TruncateRunes(txt, s.cfg.MaxTextChars)
	}

	resp := Response{
		Title:       meta.Title,
		Description: meta.Description,
		Keywords:    meta.Keywords,
		Category:    "Other",
	}
	if resp.Keywords == nil {
		resp.Keywords = []string{}
	}

	degraded := false
	start := time.Now()
	llmRes, err := s.llm.Complete(ctx, llmReq)
	metrics.LLMDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.LLMCallsTotal.WithLabelValues("error").Inc()
		metrics.FallbackTotal.Inc()
		s.log.Warn("llm failed; degrading", "url", rawURL, "stage", "llm", "err", err)
		s.recordLLMError(err)
		degraded = true
	} else {
		metrics.LLMCallsTotal.WithLabelValues("ok").Inc()
		s.recordLLMError(nil)
		if llmRes.Category != "" {
			resp.Category = llmRes.Category
		}
		if needDesc && llmRes.Description != "" {
			resp.Description = llmRes.Description
		}
		if needKeywords && len(llmRes.Keywords) > 0 {
			resp.Keywords = extract.NormalizeKeywords(llmRes.Keywords)
		}
	}

	if resp.Title == "" {
		resp.Title = hostname(res.FinalURL, rawURL)
	}

	// Degraded results (the LLM failed, category fell back to "Other") get a much
	// shorter TTL so a broken model is not frozen into the cache for a full day.
	if key != "" {
		ttl := s.cfg.CacheTTL
		if degraded {
			ttl = s.cfg.CacheDegradedTTL
		}
		s.cachePut(ctx, key, resp, ttl)
	}
	resp.CacheStatus = status
	return resp, nil
}

// CacheHealth reports the cache backend state for /healthz: "disabled" when no
// store is attached, "unavailable" when the backend fails its ping, else "ok".
// A cache outage does not break extraction, which is exactly why it needs to be
// visible rather than silent.
func (s *Service) CacheHealth(ctx context.Context) string {
	if s.cache == nil || !s.cfg.CacheEnabled {
		return "disabled"
	}
	pinger, ok := s.cache.(interface{ Ping(context.Context) error })
	if !ok {
		return "ok" // in-process store; nothing to reach
	}
	if err := pinger.Ping(ctx); err != nil {
		return "unavailable"
	}
	return "ok"
}

// cacheKey returns the entry key, or "" when caching is off.
func (s *Service) cacheKey(rawURL, lang string) string {
	if s.cache == nil || !s.cfg.CacheEnabled {
		return ""
	}
	return cache.Key(rawURL, s.effectiveLang(lang), s.fingerprint)
}

// cacheGet reads a cached response. Backend errors are logged and counted, then
// treated as a miss: a cache outage must never fail an extraction.
func (s *Service) cacheGet(ctx context.Context, key string) (Response, bool) {
	raw, ok, err := s.cache.Get(ctx, key)
	if err != nil {
		s.log.Warn("cache read failed", "stage", "cache", "err", err)
		metrics.CacheTotal.WithLabelValues("error").Inc()
		return Response{}, false
	}
	if !ok {
		metrics.CacheTotal.WithLabelValues("miss").Inc()
		return Response{}, false
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		s.log.Warn("cache entry undecodable; ignoring", "stage", "cache", "err", err)
		metrics.CacheTotal.WithLabelValues("error").Inc()
		return Response{}, false
	}
	metrics.CacheTotal.WithLabelValues("hit").Inc()
	return resp, true
}

// cachePut stores a response, best-effort.
func (s *Service) cachePut(ctx context.Context, key string, resp Response, ttl time.Duration) {
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	if err := s.cache.Set(ctx, key, raw, ttl); err != nil {
		s.log.Warn("cache write failed", "stage", "cache", "err", err)
		metrics.CacheTotal.WithLabelValues("error").Inc()
	}
}

// recordLLMError stores the latest LLM failure, or clears it on success so a
// stale error never outlives a recovered Ollama.
func (s *Service) recordLLMError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.lastErr, s.lastErrAt = "", time.Time{}
		return
	}
	s.lastErr, s.lastErrAt = err.Error(), time.Now().UTC()
}

// LastLLMError returns the most recent LLM failure and when it happened.
// The message is empty when the last call succeeded.
func (s *Service) LastLLMError() (string, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastErr, s.lastErrAt
}

// effectiveLang resolves the category language: request lang (trimmed) →
// configured default → "English".
func (s *Service) effectiveLang(reqLang string) string {
	if l := strings.TrimSpace(reqLang); l != "" {
		return l
	}
	if l := strings.TrimSpace(s.cfg.CategoryLanguage); l != "" {
		return l
	}
	return "English"
}

func hostname(finalURL, rawURL string) string {
	for _, c := range []string{finalURL, rawURL} {
		if u, err := url.Parse(c); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return ""
}
