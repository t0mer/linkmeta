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

// cloudflareGatewayHost is the fixed host for Cloudflare AI Gateway.
const cloudflareGatewayHost = "https://gateway.ai.cloudflare.com/v1"

// CloudflareGatewayURL builds the Anthropic base URL for a Cloudflare AI Gateway:
// https://gateway.ai.cloudflare.com/v1/{account_id}/{gateway_id}/anthropic
// The SDK appends /v1/messages to it.
func CloudflareGatewayURL(accountID, gatewayID string) string {
	return fmt.Sprintf("%s/%s/%s/anthropic",
		cloudflareGatewayHost, strings.TrimSpace(accountID), strings.TrimSpace(gatewayID))
}

// anthropicSettings collects option-applied settings.
type anthropicSettings struct {
	reqOpts      []option.RequestOption
	gatewayToken string
}

// AnthropicOption configures the client (base URL, gateway credentials).
type AnthropicOption func(*anthropicSettings)

// WithAnthropicBaseURL points the client at an alternate endpoint, such as a
// Cloudflare AI Gateway (see CloudflareGatewayURL) or another proxy.
func WithAnthropicBaseURL(url string) AnthropicOption {
	return func(s *anthropicSettings) {
		s.reqOpts = append(s.reqOpts, option.WithBaseURL(url))
	}
}

// WithAnthropicGatewayToken sends cf-aig-authorization on every request, for a
// Cloudflare gateway with authentication enabled. It also counts as a credential:
// with Cloudflare storing the provider key (BYOK), no local API key is needed.
func WithAnthropicGatewayToken(token string) AnthropicOption {
	return func(s *anthropicSettings) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		s.gatewayToken = token
		s.reqOpts = append(s.reqOpts, option.WithHeader("cf-aig-authorization", "Bearer "+token))
	}
}

// NewAnthropic builds a Claude-backed client. An empty apiKey is allowed so the
// process can still start; calls then fail and the fallback backend takes over.
func NewAnthropic(apiKey, model string, timeout time.Duration, opts ...AnthropicOption) *Anthropic {
	if strings.TrimSpace(model) == "" {
		model = DefaultAnthropicModel
	}
	settings := anthropicSettings{
		reqOpts: []option.RequestOption{option.WithRequestTimeout(timeout)},
	}
	if apiKey != "" {
		settings.reqOpts = append(settings.reqOpts, option.WithAPIKey(apiKey))
	}
	for _, o := range opts {
		o(&settings)
	}
	if apiKey == "" && settings.gatewayToken != "" {
		// BYOK: Cloudflare holds the provider key. The SDK refuses to send a
		// request with no credentials at all, so satisfy that check and then drop
		// the header - forwarding a placeholder key upstream would fail auth.
		settings.reqOpts = append(settings.reqOpts,
			option.WithAPIKey("byok-via-gateway"), option.WithHeaderDel("x-api-key"))
	}
	return &Anthropic{
		client:    anthropic.NewClient(settings.reqOpts...),
		model:     model,
		maxTokens: 1024,
		// A Cloudflare gateway token is a credential in its own right: with BYOK
		// the gateway holds the provider key and no local key is required.
		hasKey: apiKey != "" || settings.gatewayToken != "",
	}
}

// Complete performs a single structured-output call. The same JSON schema that
// constrains Ollama is passed as output_config.format, so the response parses
// identically on both backends.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Result, error) {
	if !a.hasKey {
		return Result{}, fmt.Errorf("anthropic: no credentials configured (need ANTHROPIC_API_KEY or AI_GATEWAY_TOKEN)")
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
		return fmt.Errorf("anthropic: no credentials configured (need ANTHROPIC_API_KEY or AI_GATEWAY_TOKEN)")
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
