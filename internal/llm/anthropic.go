package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultAnthropicModel is the model used when none is configured.
const DefaultAnthropicModel = "claude-opus-5"

// Anthropic talks to the Claude Messages API. It is an alternative backend to
// Ollama for deployments that accept a cloud dependency in exchange for latency:
// a request that takes ~100s on CPU-only hardware returns in seconds here.
type Anthropic struct {
	client    anthropic.Client
	model     string
	maxTokens int64
	hasKey    bool

	// probe result cache: /healthz runs on a timer, and the credential check
	// costs an API round trip, so don't repeat it on every scrape.
	mu        sync.Mutex
	probedAt  time.Time
	probeErr  error
	probeDone bool
}

// AnthropicOption configures the client (used by tests to retarget the base URL).
type AnthropicOption func(*[]option.RequestOption)

// WithAnthropicBaseURL points the client at an alternate endpoint.
func WithAnthropicBaseURL(url string) AnthropicOption {
	return func(opts *[]option.RequestOption) {
		*opts = append(*opts, option.WithBaseURL(url))
	}
}

// NewAnthropic builds a Claude-backed client. An empty apiKey is allowed so the
// process can still start; calls then fail and the fallback backend takes over.
func NewAnthropic(apiKey, model string, timeout time.Duration, opts ...AnthropicOption) *Anthropic {
	if strings.TrimSpace(model) == "" {
		model = DefaultAnthropicModel
	}
	reqOpts := []option.RequestOption{option.WithRequestTimeout(timeout)}
	if apiKey != "" {
		reqOpts = append(reqOpts, option.WithAPIKey(apiKey))
	}
	for _, o := range opts {
		o(&reqOpts)
	}
	return &Anthropic{
		client:    anthropic.NewClient(reqOpts...),
		model:     model,
		maxTokens: 1024,
		hasKey:    apiKey != "",
	}
}

// Complete performs a single structured-output call. The same JSON schema that
// constrains Ollama is passed as output_config.format, so the response parses
// identically on both backends.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Result, error) {
	if !a.hasKey {
		return Result{}, fmt.Errorf("anthropic: no API key configured")
	}
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: a.maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildPrompt(req))),
		},
		OutputConfig: anthropic.OutputConfigParam{
			// Metadata extraction is not a reasoning problem: low effort keeps
			// latency and spend down without hurting the answer.
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: buildSchema(req)},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("anthropic request: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return Result{}, fmt.Errorf("anthropic refused: %s", resp.StopDetails.Category)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	if text.Len() == 0 {
		return Result{}, fmt.Errorf("anthropic: empty response")
	}

	var out struct {
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
		Category    string   `json:"category"`
	}
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return Result{}, fmt.Errorf("decode structured content: %w", err)
	}
	return Result{Description: out.Description, Keywords: out.Keywords, Category: out.Category}, nil
}

// Version reports whether the API is usable: credentials present and the
// configured model resolvable. Result is cached briefly (see probe).
func (a *Anthropic) Version(ctx context.Context) error { return a.probe(ctx) }

// CheckModel verifies the configured model exists for these credentials.
func (a *Anthropic) CheckModel(ctx context.Context) error { return a.probe(ctx) }

const anthropicProbeTTL = 60 * time.Second

// probe resolves the model through the Models API, caching the outcome so a
// health-check timer cannot turn into a request-per-scrape.
func (a *Anthropic) probe(ctx context.Context) error {
	if !a.hasKey {
		return fmt.Errorf("anthropic: no API key configured")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.probeDone && time.Since(a.probedAt) < anthropicProbeTTL {
		return a.probeErr
	}
	_, err := a.client.Models.Get(ctx, a.model, anthropic.ModelGetParams{})
	if err != nil {
		err = fmt.Errorf("anthropic model %q unavailable: %w", a.model, err)
	}
	a.probeErr, a.probedAt, a.probeDone = err, time.Now(), true
	return err
}
