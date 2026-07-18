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
