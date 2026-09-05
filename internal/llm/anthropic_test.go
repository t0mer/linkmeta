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

func TestCloudflareGatewayURL(t *testing.T) {
	got := CloudflareGatewayURL("acct123", "my-gw")
	want := "https://gateway.ai.cloudflare.com/v1/acct123/my-gw/anthropic"
	if got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
	if CloudflareGatewayURL(" acct123 ", " my-gw ") != want {
		t.Error("URL builder does not trim whitespace")
	}
}

func TestGatewayRoutesRequestAndSendsToken(t *testing.T) {
	var gotPath, gotAuth, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotKey = r.URL.Path, r.Header.Get("cf-aig-authorization"), r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model": "claude-opus-5", "stop_reason": "end_turn",
			"content": []any{map[string]any{"type": "text", "text": `{"category":"News"}`}},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	// Point the gateway base path at the stub, mirroring Cloudflare's layout.
	base := srv.URL + "/v1/acct123/my-gw/anthropic"
	c := NewAnthropic("sk-test", "claude-opus-5", 30*time.Second,
		WithAnthropicBaseURL(base), WithAnthropicGatewayToken("cf-token"))

	if _, err := c.Complete(context.Background(), Request{Title: "t", Categories: []string{"News"}}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/acct123/my-gw/anthropic/v1/messages" {
		t.Errorf("path = %q, want the gateway prefix plus /v1/messages", gotPath)
	}
	if gotAuth != "Bearer cf-token" {
		t.Errorf("cf-aig-authorization = %q, want Bearer cf-token", gotAuth)
	}
	if gotKey != "sk-test" {
		t.Errorf("x-api-key = %q, want the anthropic key to still be sent", gotKey)
	}
}

func TestGatewayBYOKWorksWithoutAnthropicKey(t *testing.T) {
	var gotAuth, gotKey string
	var keyPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("cf-aig-authorization")
		gotKey, keyPresent = r.Header.Get("x-api-key"), len(r.Header.Values("x-api-key")) > 0
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model": "claude-opus-5", "stop_reason": "end_turn",
			"content": []any{map[string]any{"type": "text", "text": `{"category":"News"}`}},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	// No Anthropic key: Cloudflare holds it (BYOK / Store Keys).
	c := NewAnthropic("", "claude-opus-5", 30*time.Second,
		WithAnthropicBaseURL(srv.URL), WithAnthropicGatewayToken("cf-token"))

	res, err := c.Complete(context.Background(), Request{Title: "t", Categories: []string{"News"}})
	if err != nil {
		t.Fatalf("BYOK request must proceed without a local key: %v", err)
	}
	if res.Category != "News" {
		t.Errorf("category = %q", res.Category)
	}
	if gotAuth != "Bearer cf-token" {
		t.Errorf("cf-aig-authorization = %q", gotAuth)
	}
	// A placeholder key must never reach the gateway: Cloudflare supplies the
	// real provider key under BYOK, and a bogus one would be forwarded upstream.
	if keyPresent {
		t.Errorf("x-api-key = %q was sent in BYOK mode; want the header absent", gotKey)
	}
}

func TestNoCredentialsAtAllStillFailsFast(t *testing.T) {
	c := NewAnthropic("", "claude-opus-5", 30*time.Second)
	if _, err := c.Complete(context.Background(), Request{Title: "t"}); err == nil {
		t.Error("want an error when neither an API key nor a gateway token is configured")
	}
}
