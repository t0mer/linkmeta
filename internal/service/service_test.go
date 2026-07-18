package service

import (
	"context"
	"errors"
	"log/slog"
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
func (f fakeLLM) Version(ctx context.Context) error { return nil }

func testCfg() config.Config {
	return config.Config{
		Categories:   []string{"News", "Technology", "Other"},
		MaxTextChars: 3000,
	}
}

func TestExtractDeterministicWins(t *testing.T) {
	html := []byte(`<html><head><title>Real</title>
	<meta name="description" content="Real Desc">
	<meta name="keywords" content="go,rust"></head></html>`)
	f := fakeFetcher{html: html, url: "http://x.com"}
	l := fakeLLM{res: llm.Result{Description: "LLM Desc", Keywords: []string{"x"}, Category: "Technology"}}
	s := New(testCfg(), f, l, slog.Default())
	resp, err := s.Extract(context.Background(), "http://x.com")
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
	resp, _ := s.Extract(context.Background(), "http://x.com")
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
	resp, err := s.Extract(context.Background(), "http://x.com")
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
	resp, _ := s.Extract(context.Background(), "http://example.com/page")
	if resp.Title != "example.com" {
		t.Errorf("title = %q, want example.com", resp.Title)
	}
}

func TestExtractFetchFailureErrors(t *testing.T) {
	f := fakeFetcher{err: errors.New("boom")}
	s := New(testCfg(), f, fakeLLM{}, slog.Default())
	if _, err := s.Extract(context.Background(), "http://x.com"); err == nil {
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
