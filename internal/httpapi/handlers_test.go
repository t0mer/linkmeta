package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/t0mer/linkmeta/internal/config"
	"github.com/t0mer/linkmeta/internal/fetch"
	"github.com/t0mer/linkmeta/internal/llm"
	"github.com/t0mer/linkmeta/internal/service"
)

type stubFetcher struct{ html []byte }

func (s stubFetcher) Fetch(ctx context.Context, url string) (fetch.Result, error) {
	return fetch.Result{HTML: s.html, FinalURL: url}, nil
}

type stubLLM struct{ verErr error }

func (s stubLLM) Complete(ctx context.Context, r llm.Request) (llm.Result, error) {
	return llm.Result{Category: "News"}, nil
}
func (s stubLLM) Version(ctx context.Context) error { return s.verErr }

func newAPI(f service.Fetcher, l llm.Client) *API {
	cfg := config.Config{Categories: []string{"News", "Other"}, OllamaModel: "m", MaxTextChars: 3000}
	svc := service.New(cfg, f, l, slog.Default())
	return NewAPI(svc, l, cfg, slog.Default())
}

func TestExtractGET(t *testing.T) {
	html := []byte(`<html><head><title>Hi</title><meta name="description" content="d"><meta name="keywords" content="a,b"></head></html>`)
	api := newAPI(stubFetcher{html: html}, stubLLM{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/extract?url=http://x.com", nil)
	api.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp service.Response
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Title != "Hi" || resp.Category != "News" {
		t.Errorf("resp = %+v", resp)
	}
	if resp.Keywords == nil {
		t.Error("keywords should never be null")
	}
}

func TestExtractPOST(t *testing.T) {
	html := []byte(`<html><head><title>Hi</title></head></html>`)
	api := newAPI(stubFetcher{html: html}, stubLLM{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/extract", strings.NewReader(`{"url":"http://x.com"}`))
	req.Header.Set("Content-Type", "application/json")
	api.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestExtractInvalidURL400(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/extract?url=ftp://x.com", nil)
	api.Router().ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestExtractMissingURL400(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/extract", nil)
	api.Router().ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestHealthzOK(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{verErr: nil})
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var h map[string]string
	json.Unmarshal(rec.Body.Bytes(), &h)
	if h["ollama"] != "ok" || h["model"] != "m" || h["status"] != "ok" {
		t.Errorf("health = %v", h)
	}
}

func TestHealthzOllamaUnreachable(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{verErr: http.ErrHandlerTimeout})
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	var h map[string]string
	json.Unmarshal(rec.Body.Bytes(), &h)
	if h["ollama"] != "unreachable" {
		t.Errorf("ollama = %q", h["ollama"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{})
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("metrics code = %d", rec.Code)
	}
}
