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

// anthropicStub serves a Messages API response and captures the request body.
func anthropicStub(t *testing.T, got *map[string]any, content string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/models") {
			w.Write([]byte(`{"id":"claude-opus-5","type":"model","display_name":"Claude Opus 5"}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, got)
		w.Header().Set("Content-Type", "application/json")
		if status != 200 {
			w.WriteHeader(status)
			w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
			return
		}
		resp := map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model": "claude-opus-5", "stop_reason": "end_turn",
			"content": []any{map[string]any{"type": "text", "text": content}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestAnthropicCompleteParsesStructuredOutput(t *testing.T) {
	var got map[string]any
	srv := anthropicStub(t, &got, `{"description":"תיאור","keywords":["a","b"],"category":"News"}`, 200)
	defer srv.Close()

	c := NewAnthropic("test-key", "claude-opus-5", 30*time.Second, WithAnthropicBaseURL(srv.URL))
	res, err := c.Complete(context.Background(), Request{
		Title: "t", NeedDescription: true, NeedKeywords: true,
		Categories: []string{"News", "Other"}, CategoryLanguage: "English",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Description != "תיאור" || res.Category != "News" || len(res.Keywords) != 2 {
		t.Errorf("res = %+v", res)
	}
}

func TestAnthropicSendsSchemaAndModel(t *testing.T) {
	var got map[string]any
	srv := anthropicStub(t, &got, `{"category":"News"}`, 200)
	defer srv.Close()

	c := NewAnthropic("test-key", "claude-opus-5", 30*time.Second, WithAnthropicBaseURL(srv.URL))
	if _, err := c.Complete(context.Background(), Request{
		Title: "t", Categories: []string{"News", "Other"}, CategoryLanguage: "English",
	}); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "claude-opus-5" {
		t.Errorf("model = %v", got["model"])
	}
	oc, _ := got["output_config"].(map[string]any)
	if oc == nil {
		t.Fatal("no output_config sent; structured output is what guarantees parseable JSON")
	}
	format, _ := oc["format"].(map[string]any)
	if format == nil || format["type"] != "json_schema" {
		t.Fatalf("format = %v, want json_schema", oc["format"])
	}
	schema, _ := format["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["category"]; !ok {
		t.Errorf("schema properties = %v, want category", props)
	}
	if _, ok := props["description"]; ok {
		t.Error("description requested when the page already supplied one")
	}
}

func TestAnthropicAPIErrorReturnsError(t *testing.T) {
	var got map[string]any
	srv := anthropicStub(t, &got, "", 400)
	defer srv.Close()
	c := NewAnthropic("test-key", "claude-opus-5", 30*time.Second, WithAnthropicBaseURL(srv.URL))
	if _, err := c.Complete(context.Background(), Request{Title: "t", Categories: []string{"News"}}); err == nil {
		t.Fatal("want error on API failure")
	}
}

func TestAnthropicMissingKeyFailsFast(t *testing.T) {
	c := NewAnthropic("", "claude-opus-5", 30*time.Second)
	if err := c.Version(context.Background()); err == nil {
		t.Error("want error when no API key is configured")
	}
}
