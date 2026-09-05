package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Request describes which fields the model must produce.
type Request struct {
	Title           string
	Description     string
	Text            string
	NeedDescription bool
	NeedKeywords    bool
	Categories      []string
	// CategoryLanguage is the language the category label is returned in.
	// Empty or "English" (case-insensitive) keeps the strict enum guarantee;
	// any other value renders a best-effort translated label (free string).
	CategoryLanguage string
}

// Result holds only requested fields.
type Result struct {
	Description string
	Keywords    []string
	Category    string
}

// Client is the LLM abstraction (mockable in tests).
type Client interface {
	Complete(ctx context.Context, req Request) (Result, error)
	Version(ctx context.Context) error
	// CheckModel reports whether the configured model is actually installed.
	// Version() only proves the server is up: Ollama answers /api/version
	// happily with zero models pulled, while every /api/chat then 404s.
	CheckModel(ctx context.Context) error
}

// Ollama talks to an Ollama server over HTTP.
type Ollama struct {
	baseURL   string
	model     string
	keepAlive string
	client    *http.Client
}

// NewOllama builds an Ollama client.
func NewOllama(baseURL, model, keepAlive string, timeout time.Duration) *Ollama {
	return &Ollama{
		baseURL:   strings.TrimRight(baseURL, "/"),
		model:     model,
		keepAlive: keepAlive,
		client:    &http.Client{Timeout: timeout},
	}
}

// isEnglish reports whether the category language is English (the default).
// Blank counts as English.
func isEnglish(lang string) bool {
	s := strings.ToLower(strings.TrimSpace(lang))
	return s == "" || s == "english" || s == "en"
}

// buildSchema builds a JSON schema with only the needed properties; category is
// always present and enum-constrained to the configured list.
func buildSchema(req Request) map[string]any {
	props := map[string]any{}
	required := []string{}
	if req.NeedDescription {
		props["description"] = map[string]any{"type": "string"}
		required = append(required, "description")
	}
	if req.NeedKeywords {
		props["keywords"] = map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		}
		required = append(required, "keywords")
	}
	// English keeps the strict enum guarantee; other languages need a free
	// string because the returned label is translated out of the English list.
	if isEnglish(req.CategoryLanguage) {
		props["category"] = map[string]any{"type": "string", "enum": req.Categories}
	} else {
		props["category"] = map[string]any{"type": "string"}
	}
	required = append(required, "category")
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func buildPrompt(req Request) string {
	var b strings.Builder
	b.WriteString("You extract structured metadata from a web page. ")
	b.WriteString("Respond ONLY with JSON matching the schema.\n")
	if req.NeedDescription {
		b.WriteString("- description: one concise sentence in the page's ORIGINAL language.\n")
	}
	if req.NeedKeywords {
		b.WriteString("- keywords: 3-10 lowercase topical keywords in the page's ORIGINAL language.\n")
	}
	catLang := req.CategoryLanguage
	if catLang == "" {
		catLang = "English"
	}
	fmt.Fprintf(&b, "- category: classify into exactly one of the allowed English categories, "+
		"then return the category name in %s.\n\n", catLang)
	b.WriteString("Title: " + req.Title + "\n")
	// Only as context. When the model is asked to write the description, feeding
	// it the page's own text invites a verbatim copy of what we are replacing.
	if req.Description != "" && !req.NeedDescription {
		b.WriteString("Description: " + req.Description + "\n")
	}
	if req.Text != "" {
		b.WriteString("\nPage text:\n" + req.Text + "\n")
	}
	return b.String()
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string         `json:"model"`
	Messages  []chatMessage  `json:"messages"`
	Stream    bool           `json:"stream"`
	Format    map[string]any `json:"format"`
	KeepAlive string         `json:"keep_alive"`
	Options   map[string]any `json:"options"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Error   string      `json:"error"`
}

// Complete performs a single structured /api/chat call.
func (o *Ollama) Complete(ctx context.Context, req Request) (Result, error) {
	payload := chatRequest{
		Model:     o.model,
		Messages:  []chatMessage{{Role: "user", Content: buildPrompt(req)}},
		Stream:    false,
		Format:    buildSchema(req),
		KeepAlive: o.keepAlive,
		Options:   map[string]any{"temperature": 0.2, "num_ctx": 4096},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/api/chat", bytes.NewReader(buf))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("ollama status %d: %s", resp.StatusCode, string(body))
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return Result{}, fmt.Errorf("decode chat response: %w", err)
	}
	if cr.Error != "" {
		return Result{}, fmt.Errorf("ollama error: %s", cr.Error)
	}

	var out struct {
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
		Category    string   `json:"category"`
	}
	if err := json.Unmarshal([]byte(cr.Message.Content), &out); err != nil {
		return Result{}, fmt.Errorf("decode structured content: %w", err)
	}
	return Result{Description: out.Description, Keywords: out.Keywords, Category: out.Category}, nil
}

// Version checks Ollama reachability via GET /api/version.
func (o *Ollama) Version(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/version", nil)
	if err != nil {
		return err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama version status %d", resp.StatusCode)
	}
	return nil
}

// CheckModel verifies the configured model exists on the server via POST
// /api/show. It only reads metadata — the model is not loaded into memory.
func (o *Ollama) CheckModel(ctx context.Context) error {
	buf, err := json.Marshal(map[string]string{"model": o.model})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/api/show", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama show: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model %q unavailable (status %d): %s",
			o.model, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
