package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompleteStructuredHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		if req["format"] == nil {
			t.Error("no format schema in request")
		}
		if req["stream"] != false {
			t.Error("stream should be false")
		}
		content := `{"description":"תיאור","keywords":["a","b"],"category":"News"}`
		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": content},
		})
	}))
	defer srv.Close()

	c := NewOllama(srv.URL, "test-model", "24h", 5*time.Second)
	res, err := c.Complete(context.Background(), Request{
		Title:           "כותרת",
		NeedDescription: true,
		NeedKeywords:    true,
		Categories:      []string{"News", "Other"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Description != "תיאור" || res.Category != "News" || len(res.Keywords) != 2 {
		t.Errorf("res = %+v", res)
	}
}

func TestCompleteServerErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":"model not found"}`)
	}))
	defer srv.Close()
	c := NewOllama(srv.URL, "m", "24h", 5*time.Second)
	if _, err := c.Complete(context.Background(), Request{Categories: []string{"Other"}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCompleteUnreachableReturnsError(t *testing.T) {
	c := NewOllama("http://127.0.0.1:1", "m", "24h", 500*time.Millisecond)
	if _, err := c.Complete(context.Background(), Request{Categories: []string{"Other"}}); err == nil {
		t.Fatal("expected error on unreachable server")
	}
}

func TestSchemaOnlyIncludesNeededFields(t *testing.T) {
	schema := buildSchema(Request{NeedDescription: false, NeedKeywords: true, Categories: []string{"A"}})
	b, _ := json.Marshal(schema)
	s := string(b)
	if strings.Contains(s, "description") {
		t.Error("schema should omit description when not needed")
	}
	if !strings.Contains(s, "keywords") || !strings.Contains(s, "category") {
		t.Error("schema must include keywords and category")
	}
}

func TestSchemaCategoryEnumForEnglish(t *testing.T) {
	// English (default/blank/any case) -> category enum-constrained.
	for _, lang := range []string{"", "English", "english"} {
		schema := buildSchema(Request{Categories: []string{"News", "Other"}, CategoryLanguage: lang})
		if !hasCategoryEnum(schema) {
			t.Errorf("lang=%q: category should be enum-constrained", lang)
		}
	}
}

func TestSchemaCategoryFreeStringForNonEnglish(t *testing.T) {
	schema := buildSchema(Request{Categories: []string{"News", "Other"}, CategoryLanguage: "Hebrew"})
	if hasCategoryEnum(schema) {
		t.Error("non-English category should be a free string, not enum")
	}
}

// hasCategoryEnum reports whether the category property carries an enum.
func hasCategoryEnum(schema map[string]any) bool {
	props := schema["properties"].(map[string]any)
	cat := props["category"].(map[string]any)
	_, ok := cat["enum"]
	return ok
}

func TestPromptIncludesCategoryLanguage(t *testing.T) {
	p := buildPrompt(Request{Title: "t", Categories: []string{"News"}, CategoryLanguage: "Hebrew"})
	if !strings.Contains(p, "Hebrew") {
		t.Errorf("prompt missing category language: %q", p)
	}
}

func TestVersionReachability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Errorf("path = %s", r.URL.Path)
		}
		io.WriteString(w, `{"version":"0.1.0"}`)
	}))
	defer srv.Close()
	c := NewOllama(srv.URL, "m", "24h", 2*time.Second)
	if err := c.Version(context.Background()); err != nil {
		t.Fatalf("Version = %v", err)
	}
}

func TestPromptOmitsPageDescriptionWhenGenerating(t *testing.T) {
	p := buildPrompt(Request{
		Title:           "T",
		Description:     "stale page description",
		NeedDescription: true,
		Categories:      []string{"News"},
	})
	if strings.Contains(p, "stale page description") {
		t.Error("page description fed back to the model it is meant to replace")
	}
}

func TestPromptKeepsPageDescriptionAsContextWhenNotGenerating(t *testing.T) {
	p := buildPrompt(Request{
		Title:       "T",
		Description: "page description",
		Categories:  []string{"News"},
	})
	if !strings.Contains(p, "page description") {
		t.Error("page description should stay as context when not regenerating it")
	}
}

func TestCheckModelPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			t.Errorf("path = %s, want /api/show", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "m1" {
			t.Errorf("model = %v, want m1", body["model"])
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"details":{}}`))
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, "m1", "24h", 5*time.Second)
	if err := o.CheckModel(context.Background()); err != nil {
		t.Errorf("CheckModel = %v, want nil", err)
	}
}

func TestCheckModelMissingReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"model 'm1' not found"}`))
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, "m1", "24h", 5*time.Second)
	err := o.CheckModel(context.Background())
	if err == nil {
		t.Fatal("CheckModel = nil, want error for a missing model")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want the server message", err)
	}
}

func TestCompleteSendsNumPredictCap(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"message":{"role":"assistant","content":"{\"category\":\"News\"}"}}`))
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, "m", "24h", 5*time.Second)
	o.SetMaxTokens(128)
	if _, err := o.Complete(context.Background(), Request{Title: "t", Categories: []string{"News"}}); err != nil {
		t.Fatal(err)
	}
	opts, _ := got["options"].(map[string]any)
	if opts == nil {
		t.Fatal("no options sent")
	}
	if n, _ := opts["num_predict"].(float64); int(n) != 128 {
		t.Errorf("num_predict = %v, want 128 (generation must be bounded)", opts["num_predict"])
	}
}

func TestNonPositiveMaxTokensLeavesGenerationUncapped(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"message":{"role":"assistant","content":"{\"category\":\"News\"}"}}`))
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, "m", "24h", 5*time.Second)
	o.SetMaxTokens(0)
	if _, err := o.Complete(context.Background(), Request{Title: "t", Categories: []string{"News"}}); err != nil {
		t.Fatal(err)
	}
	opts, _ := got["options"].(map[string]any)
	if _, ok := opts["num_predict"]; ok {
		t.Error("num_predict sent for a non-positive cap; want the option omitted")
	}
}
