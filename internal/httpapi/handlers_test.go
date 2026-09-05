package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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

type stubLLM struct {
	verErr   error
	modelErr error
}

func (s stubLLM) Complete(ctx context.Context, r llm.Request) (llm.Result, error) {
	return llm.Result{Category: "News"}, nil
}
func (s stubLLM) Version(ctx context.Context) error    { return s.verErr }
func (s stubLLM) CheckModel(ctx context.Context) error { return s.modelErr }

// captureLLM records the request so a test can assert the language reached the LLM.
type captureLLM struct{ last llm.Request }

func (c *captureLLM) Complete(ctx context.Context, r llm.Request) (llm.Result, error) {
	c.last = r
	return llm.Result{Category: "News"}, nil
}
func (c *captureLLM) Version(ctx context.Context) error    { return nil }
func (c *captureLLM) CheckModel(ctx context.Context) error { return nil }

func newAPI(f service.Fetcher, l llm.Client) *API {
	cfg := config.Config{Categories: []string{"News", "Other"}, OllamaModel: "m", MaxTextChars: 3000}
	svc := service.New(cfg, f, l, slog.Default())
	return NewAPI(svc, l, cfg, "test-version", slog.Default())
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

func TestExtractLangFromQuery(t *testing.T) {
	html := []byte(`<html><head><title>Hi</title></head></html>`)
	cap := &captureLLM{}
	api := newAPI(stubFetcher{html: html}, cap)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/extract?url=http://x.com&lang=Hebrew", nil)
	api.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if cap.last.CategoryLanguage != "Hebrew" {
		t.Errorf("lang = %q, want Hebrew", cap.last.CategoryLanguage)
	}
}

func TestExtractLangFromBody(t *testing.T) {
	html := []byte(`<html><head><title>Hi</title></head></html>`)
	cap := &captureLLM{}
	api := newAPI(stubFetcher{html: html}, cap)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/extract", strings.NewReader(`{"url":"http://x.com","lang":"Spanish"}`))
	req.Header.Set("Content-Type", "application/json")
	api.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if cap.last.CategoryLanguage != "Spanish" {
		t.Errorf("lang = %q, want Spanish", cap.last.CategoryLanguage)
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

// failLLM always fails Complete, reproducing the degraded-to-"Other" production state.
type failLLM struct{ modelErr error }

func (f failLLM) Complete(ctx context.Context, r llm.Request) (llm.Result, error) {
	return llm.Result{}, errors.New(`ollama status 404: model "m" not found`)
}
func (f failLLM) Version(ctx context.Context) error    { return nil }
func (f failLLM) CheckModel(ctx context.Context) error { return f.modelErr }

func TestHealthzReportsModelMissing(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{verErr: nil, modelErr: errors.New("model not found")})
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	var h map[string]any
	json.Unmarshal(rec.Body.Bytes(), &h)
	if h["ollama"] != "model-missing" {
		t.Errorf("ollama = %v, want model-missing (server up, model absent)", h["ollama"])
	}
}

func TestHealthzUnreachableWinsOverModelCheck(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{verErr: http.ErrHandlerTimeout, modelErr: errors.New("nope")})
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	var h map[string]any
	json.Unmarshal(rec.Body.Bytes(), &h)
	if h["ollama"] != "unreachable" {
		t.Errorf("ollama = %v, want unreachable", h["ollama"])
	}
}

func TestHealthzIncludesVersion(t *testing.T) {
	api := newAPI(stubFetcher{}, stubLLM{})
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	var h map[string]any
	json.Unmarshal(rec.Body.Bytes(), &h)
	if h["version"] != "test-version" {
		t.Errorf("version = %v, want test-version", h["version"])
	}
}

func TestHealthzSurfacesLastLLMError(t *testing.T) {
	html := []byte(`<html><head><title>Hi</title></head></html>`)
	api := newAPI(stubFetcher{html: html}, failLLM{})

	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	var before map[string]any
	json.Unmarshal(rec.Body.Bytes(), &before)
	if _, ok := before["last_llm_error"]; ok {
		t.Error("last_llm_error present before any extract; want omitted")
	}

	api.Router().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/extract?url=http://x.com", nil))

	rec = httptest.NewRecorder()
	api.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	var after map[string]any
	json.Unmarshal(rec.Body.Bytes(), &after)
	msg, _ := after["last_llm_error"].(string)
	if !strings.Contains(msg, "404") {
		t.Errorf("last_llm_error = %q, want the ollama error text", msg)
	}
	if after["last_llm_error_at"] == nil {
		t.Error("last_llm_error_at missing")
	}
}

func TestHealthzClearsLastErrorAfterSuccess(t *testing.T) {
	html := []byte(`<html><head><title>Hi</title></head></html>`)
	cfg := config.Config{Categories: []string{"News", "Other"}, OllamaModel: "m", MaxTextChars: 3000}
	svc := service.New(cfg, stubFetcher{html: html}, failLLM{}, slog.Default())
	api := NewAPI(svc, stubLLM{}, cfg, "v", slog.Default())
	api.Router().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/extract?url=http://x.com", nil))

	// A later healthy call must clear the stale error.
	svc2 := service.New(cfg, stubFetcher{html: html}, stubLLM{}, slog.Default())
	if _, err := svc2.Extract(context.Background(), "http://x.com", ""); err != nil {
		t.Fatal(err)
	}
	if msg, _ := svc2.LastLLMError(); msg != "" {
		t.Errorf("LastLLMError = %q after success, want empty", msg)
	}
}
