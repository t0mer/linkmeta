package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/t0mer/linkmeta/internal/config"
	"github.com/t0mer/linkmeta/internal/fetch"
	"github.com/t0mer/linkmeta/internal/llm"
)

type fakeFetcher struct {
	html []byte
	url  string
	err  error
}

func (f fakeFetcher) Fetch(ctx context.Context, url string) (fetch.Result, error) {
	if f.err != nil {
		return fetch.Result{}, f.err
	}
	return fetch.Result{HTML: f.html, FinalURL: f.url}, nil
}

type fakeLLM struct {
	res llm.Result
	err error
}

func (f fakeLLM) Complete(ctx context.Context, r llm.Request) (llm.Result, error) {
	return f.res, f.err
}
func (f fakeLLM) Version(ctx context.Context) error    { return nil }
func (f fakeLLM) CheckModel(ctx context.Context) error { return nil }

// captureLLM records the last request so tests can assert the effective language.
type captureLLM struct{ last llm.Request }

func (c *captureLLM) Complete(ctx context.Context, r llm.Request) (llm.Result, error) {
	c.last = r
	return llm.Result{Category: "News"}, nil
}
func (c *captureLLM) Version(ctx context.Context) error    { return nil }
func (c *captureLLM) CheckModel(ctx context.Context) error { return nil }

func testCfg() config.Config {
	return config.Config{
		Categories:       []string{"News", "Technology", "Other"},
		CategoryLanguage: "English",
		MaxTextChars:     3000,
	}
}

func TestExtractRequestLangOverridesConfig(t *testing.T) {
	html := []byte(`<html><head><title>T</title></head></html>`)
	cap := &captureLLM{}
	s := New(testCfg(), fakeFetcher{html: html, url: "http://x.com"}, cap, slog.Default())
	if _, err := s.Extract(context.Background(), "http://x.com", "Hebrew"); err != nil {
		t.Fatal(err)
	}
	if cap.last.CategoryLanguage != "Hebrew" {
		t.Errorf("effective lang = %q, want Hebrew", cap.last.CategoryLanguage)
	}
}

func TestExtractBlankLangUsesConfigDefault(t *testing.T) {
	cfg := testCfg()
	cfg.CategoryLanguage = "Spanish"
	html := []byte(`<html><head><title>T</title></head></html>`)
	cap := &captureLLM{}
	s := New(cfg, fakeFetcher{html: html, url: "http://x.com"}, cap, slog.Default())
	if _, err := s.Extract(context.Background(), "http://x.com", "  "); err != nil {
		t.Fatal(err)
	}
	if cap.last.CategoryLanguage != "Spanish" {
		t.Errorf("effective lang = %q, want Spanish (config default)", cap.last.CategoryLanguage)
	}
}

func TestExtractBlankEverywhereFallsBackToEnglish(t *testing.T) {
	cfg := testCfg()
	cfg.CategoryLanguage = ""
	html := []byte(`<html><head><title>T</title></head></html>`)
	cap := &captureLLM{}
	s := New(cfg, fakeFetcher{html: html, url: "http://x.com"}, cap, slog.Default())
	if _, err := s.Extract(context.Background(), "http://x.com", ""); err != nil {
		t.Fatal(err)
	}
	if cap.last.CategoryLanguage != "English" {
		t.Errorf("effective lang = %q, want English", cap.last.CategoryLanguage)
	}
}

func TestExtractDeterministicWins(t *testing.T) {
	html := []byte(`<html><head><title>Real</title>
	<meta name="description" content="Real Desc">
	<meta name="keywords" content="go,rust"></head></html>`)
	f := fakeFetcher{html: html, url: "http://x.com"}
	l := fakeLLM{res: llm.Result{Description: "LLM Desc", Keywords: []string{"x"}, Category: "Technology"}}
	s := New(testCfg(), f, l, slog.Default())
	resp, err := s.Extract(context.Background(), "http://x.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Real" || resp.Description != "Real Desc" {
		t.Errorf("deterministic overwritten: %+v", resp)
	}
	if len(resp.Keywords) != 2 || resp.Keywords[0] != "go" {
		t.Errorf("keywords = %v", resp.Keywords)
	}
	if resp.Category != "Technology" {
		t.Errorf("category = %q", resp.Category)
	}
}

func TestExtractLLMFillsGaps(t *testing.T) {
	html := []byte(`<html><head><title>Only Title</title></head><body>text</body></html>`)
	f := fakeFetcher{html: html, url: "http://x.com"}
	l := fakeLLM{res: llm.Result{Description: "Filled", Keywords: []string{"k1", "k2"}, Category: "News"}}
	s := New(testCfg(), f, l, slog.Default())
	resp, _ := s.Extract(context.Background(), "http://x.com", "")
	if resp.Description != "Filled" || resp.Category != "News" {
		t.Errorf("resp = %+v", resp)
	}
	if len(resp.Keywords) != 2 {
		t.Errorf("keywords = %v", resp.Keywords)
	}
}

func TestExtractLLMFailureDegrades(t *testing.T) {
	html := []byte(`<html><head><title>T</title></head></html>`)
	f := fakeFetcher{html: html, url: "http://x.com"}
	l := fakeLLM{err: errors.New("ollama down")}
	s := New(testCfg(), f, l, slog.Default())
	resp, err := s.Extract(context.Background(), "http://x.com", "")
	if err != nil {
		t.Fatalf("LLM failure must not error: %v", err)
	}
	if resp.Category != "Other" {
		t.Errorf("category = %q, want Other", resp.Category)
	}
	if resp.Keywords == nil {
		t.Error("keywords nil, want []")
	}
}

func TestExtractEmptyTitleUsesHostname(t *testing.T) {
	html := []byte(`<html><body>no title</body></html>`)
	f := fakeFetcher{html: html, url: "http://example.com/page"}
	l := fakeLLM{res: llm.Result{Category: "Other"}}
	s := New(testCfg(), f, l, slog.Default())
	resp, _ := s.Extract(context.Background(), "http://example.com/page", "")
	if resp.Title != "example.com" {
		t.Errorf("title = %q, want example.com", resp.Title)
	}
}

func TestExtractFetchFailureErrors(t *testing.T) {
	f := fakeFetcher{err: errors.New("boom")}
	s := New(testCfg(), f, fakeLLM{}, slog.Default())
	if _, err := s.Extract(context.Background(), "http://x.com", ""); err == nil {
		t.Fatal("expected error on fetch failure")
	}
}

func TestValidateURL(t *testing.T) {
	if err := ValidateURL("http://ok.com"); err != nil {
		t.Errorf("http should be valid: %v", err)
	}
	if err := ValidateURL("ftp://x.com"); err == nil {
		t.Error("ftp should be invalid")
	}
	if err := ValidateURL("http://"); err == nil {
		t.Error("empty host should be invalid")
	}
	if err := ValidateURL("not a url"); err == nil {
		t.Error("garbage should be invalid")
	}
}

func TestExtractForceLLMOverridesPageMeta(t *testing.T) {
	html := []byte(`<html><head><title>Real</title>
	<meta name="description" content="Page Desc">
	<meta name="keywords" content="go,rust"></head><body>body text</body></html>`)
	cfg := testCfg()
	cfg.ForceLLM = true
	f := fakeFetcher{html: html, url: "http://x.com"}
	l := fakeLLM{res: llm.Result{Description: "LLM Desc", Keywords: []string{"LLM-KW"}, Category: "Technology"}}
	s := New(cfg, f, l, slog.Default())
	resp, err := s.Extract(context.Background(), "http://x.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Description != "LLM Desc" {
		t.Errorf("description = %q, want LLM Desc", resp.Description)
	}
	if len(resp.Keywords) != 1 || resp.Keywords[0] != "llm-kw" {
		t.Errorf("keywords = %v, want [llm-kw]", resp.Keywords)
	}
	if resp.Category != "Technology" {
		t.Errorf("category = %q, want Technology", resp.Category)
	}
	if resp.Title != "Real" {
		t.Errorf("title = %q, want Real (title stays deterministic)", resp.Title)
	}
}

func TestExtractForceLLMRequestsAllFieldsAndSendsText(t *testing.T) {
	html := []byte(`<html><head><title>Real</title>
	<meta name="description" content="Page Desc">
	<meta name="keywords" content="go"></head><body><p>readable body text</p></body></html>`)
	cfg := testCfg()
	cfg.ForceLLM = true
	cap := &captureLLM{}
	s := New(cfg, fakeFetcher{html: html, url: "http://x.com"}, cap, slog.Default())
	if _, err := s.Extract(context.Background(), "http://x.com", ""); err != nil {
		t.Fatal(err)
	}
	if !cap.last.NeedDescription || !cap.last.NeedKeywords {
		t.Errorf("need flags = %v/%v, want true/true when forced", cap.last.NeedDescription, cap.last.NeedKeywords)
	}
	if cap.last.Text == "" {
		t.Error("page text not sent to the model when forced")
	}
}

func TestExtractForceLLMFailureKeepsPageValues(t *testing.T) {
	html := []byte(`<html><head><title>Real</title>
	<meta name="description" content="Page Desc"></head></html>`)
	cfg := testCfg()
	cfg.ForceLLM = true
	f := fakeFetcher{html: html, url: "http://x.com"}
	l := fakeLLM{err: errors.New("ollama down")}
	s := New(cfg, f, l, slog.Default())
	resp, err := s.Extract(context.Background(), "http://x.com", "")
	if err != nil {
		t.Fatalf("LLM failure must not error even when forced: %v", err)
	}
	if resp.Description != "Page Desc" {
		t.Errorf("description = %q, want the page value on LLM failure", resp.Description)
	}
	if resp.Category != "Other" {
		t.Errorf("category = %q, want Other", resp.Category)
	}
}

func TestExtractUnforcedStillPrefersPageMeta(t *testing.T) {
	html := []byte(`<html><head><title>Real</title>
	<meta name="description" content="Page Desc"></head></html>`)
	f := fakeFetcher{html: html, url: "http://x.com"}
	l := fakeLLM{res: llm.Result{Description: "LLM Desc", Category: "News"}}
	s := New(testCfg(), f, l, slog.Default())
	resp, _ := s.Extract(context.Background(), "http://x.com", "")
	if resp.Description != "Page Desc" {
		t.Errorf("description = %q, want Page Desc when not forced", resp.Description)
	}
}

func TestLastLLMErrorRecordedAndCleared(t *testing.T) {
	html := []byte(`<html><head><title>T</title></head></html>`)
	f := fakeFetcher{html: html, url: "http://x.com"}

	s := New(testCfg(), f, fakeLLM{err: errors.New("ollama status 404: model not found")}, slog.Default())
	if _, err := s.Extract(context.Background(), "http://x.com", ""); err != nil {
		t.Fatal(err)
	}
	msg, at := s.LastLLMError()
	if !strings.Contains(msg, "404") {
		t.Errorf("LastLLMError = %q, want the ollama error", msg)
	}
	if at.IsZero() {
		t.Error("LastLLMError timestamp is zero")
	}

	ok := New(testCfg(), f, fakeLLM{res: llm.Result{Category: "News"}}, slog.Default())
	if _, err := ok.Extract(context.Background(), "http://x.com", ""); err != nil {
		t.Fatal(err)
	}
	if msg, _ := ok.LastLLMError(); msg != "" {
		t.Errorf("LastLLMError = %q after a successful call, want empty", msg)
	}
}
