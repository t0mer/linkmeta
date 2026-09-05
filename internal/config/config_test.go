package config

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestLoadDefaults(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	c, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
	if c.OllamaModel != "qwen2.5:3b-instruct" {
		t.Errorf("model = %q", c.OllamaModel)
	}
	if c.LLMTimeout != 180*time.Second {
		t.Errorf("LLMTimeout = %v", c.LLMTimeout)
	}
	if len(c.Categories) != 14 || c.Categories[0] != "Technology" {
		t.Errorf("categories = %v", c.Categories)
	}
	if c.CategoryLanguage != "English" {
		t.Errorf("CategoryLanguage = %q, want English", c.CategoryLanguage)
	}
}

func TestCategoryLanguageEnvOverride(t *testing.T) {
	t.Setenv("CATEGORY_LANGUAGE", "Hebrew")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	c, _ := Load(v)
	if c.CategoryLanguage != "Hebrew" {
		t.Errorf("CategoryLanguage = %q, want Hebrew", c.CategoryLanguage)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "llama3.2:3b")
	t.Setenv("MAX_TEXT_CHARS", "1500")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	c, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if c.OllamaModel != "llama3.2:3b" {
		t.Errorf("model = %q, want llama3.2:3b", c.OllamaModel)
	}
	if c.MaxTextChars != 1500 {
		t.Errorf("MaxTextChars = %d, want 1500", c.MaxTextChars)
	}
}

func TestCategoriesCSVParsed(t *testing.T) {
	t.Setenv("CATEGORIES", "A, B ,C")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	c, _ := Load(v)
	if len(c.Categories) != 3 || c.Categories[1] != "B" {
		t.Errorf("categories = %v, want [A B C]", c.Categories)
	}
}

func TestForceLLMDefaultsFalse(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	c, _ := Load(v)
	if c.ForceLLM {
		t.Error("ForceLLM = true, want false by default")
	}
}

func TestForceLLMEnvOverride(t *testing.T) {
	t.Setenv("FORCE_LLM", "true")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	c, _ := Load(v)
	if !c.ForceLLM {
		t.Error("ForceLLM = false, want true from FORCE_LLM=true")
	}
}

func TestForceLLMFlagOverridesEnv(t *testing.T) {
	t.Setenv("FORCE_LLM", "false")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	BindFlags(v, fs)
	if err := fs.Parse([]string{"--force-llm"}); err != nil {
		t.Fatal(err)
	}
	c, _ := Load(v)
	if !c.ForceLLM {
		t.Error("ForceLLM = false, want true from --force-llm")
	}
}

func TestLLMMaxTokensDefaultAndOverride(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	c, _ := Load(v)
	if c.LLMMaxTokens != 256 {
		t.Errorf("LLMMaxTokens = %d, want 256", c.LLMMaxTokens)
	}

	t.Setenv("LLM_MAX_TOKENS", "96")
	v2 := viper.New()
	setDefaults(v2)
	v2.AutomaticEnv()
	c2, _ := Load(v2)
	if c2.LLMMaxTokens != 96 {
		t.Errorf("LLMMaxTokens = %d, want 96 from env", c2.LLMMaxTokens)
	}
}

func TestLLMProviderDefaultsToOllama(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	c, _ := Load(v)
	if c.LLMProvider != "ollama" {
		t.Errorf("LLMProvider = %q, want ollama (self-hosted stays the default)", c.LLMProvider)
	}
	if c.AnthropicModel != "claude-opus-5" {
		t.Errorf("AnthropicModel = %q, want claude-opus-5", c.AnthropicModel)
	}
	if c.AnthropicAPIKey != "" {
		t.Errorf("AnthropicAPIKey = %q, want empty", c.AnthropicAPIKey)
	}
}

func TestLLMProviderEnvOverride(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "ANTHROPIC")
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_MODEL", "claude-sonnet-5")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	c, _ := Load(v)
	if c.LLMProvider != "anthropic" {
		t.Errorf("LLMProvider = %q, want normalized to anthropic", c.LLMProvider)
	}
	if c.AnthropicAPIKey != "sk-test" || c.AnthropicModel != "claude-sonnet-5" {
		t.Errorf("anthropic config = %q/%q", c.AnthropicAPIKey, c.AnthropicModel)
	}
}

func TestCacheDefaults(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	c, _ := Load(v)
	if !c.CacheEnabled {
		t.Error("CacheEnabled = false, want true by default")
	}
	if c.CacheBackend != "memory" {
		t.Errorf("CacheBackend = %q, want memory", c.CacheBackend)
	}
	if c.CacheTTL != 24*time.Hour {
		t.Errorf("CacheTTL = %v, want 24h", c.CacheTTL)
	}
	if c.CacheDegradedTTL != 5*time.Minute {
		t.Errorf("CacheDegradedTTL = %v, want 5m", c.CacheDegradedTTL)
	}
	if c.CacheMaxEntries != 1000 {
		t.Errorf("CacheMaxEntries = %d, want 1000", c.CacheMaxEntries)
	}
}

func TestCacheEnvOverride(t *testing.T) {
	t.Setenv("CACHE_BACKEND", "REDIS")
	t.Setenv("CACHE_TTL", "1h")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("REDIS_URL", "redis://cache:6379/1")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	c, _ := Load(v)
	if c.CacheBackend != "redis" {
		t.Errorf("CacheBackend = %q, want normalized to redis", c.CacheBackend)
	}
	if c.CacheTTL != time.Hour || c.CacheEnabled || c.RedisURL != "redis://cache:6379/1" {
		t.Errorf("cache config = %v/%v/%q", c.CacheTTL, c.CacheEnabled, c.RedisURL)
	}
}

func TestCacheFingerprintTracksExtractionSettings(t *testing.T) {
	base := Config{ForceLLM: false, LLMProvider: "ollama", OllamaModel: "m", Categories: []string{"A", "B"}}
	if base.CacheFingerprint() == "" {
		t.Fatal("empty fingerprint")
	}
	forced := base
	forced.ForceLLM = true
	if base.CacheFingerprint() == forced.CacheFingerprint() {
		t.Error("fingerprint ignores FORCE_LLM; cached results would survive the flag flip")
	}
	other := base
	other.Categories = []string{"A", "C"}
	if base.CacheFingerprint() == other.CacheFingerprint() {
		t.Error("fingerprint ignores the category list")
	}
	claude := base
	claude.LLMProvider = "anthropic"
	if base.CacheFingerprint() == claude.CacheFingerprint() {
		t.Error("fingerprint ignores the provider")
	}
	if base.CacheFingerprint() != base.CacheFingerprint() {
		t.Error("fingerprint is not deterministic")
	}
}

func TestAIGatewayDefaultsEmpty(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	c, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if c.AIGatewayAccountID != "" || c.AIGatewayID != "" || c.AIGatewayToken != "" {
		t.Errorf("gateway config = %q/%q/%q, want empty", c.AIGatewayAccountID, c.AIGatewayID, c.AIGatewayToken)
	}
	if c.AIGatewayEnabled() {
		t.Error("AIGatewayEnabled = true with nothing configured")
	}
}

func TestAIGatewayEnvOverride(t *testing.T) {
	t.Setenv("AI_GATEWAY_ACCOUNT_ID", " acct123 ")
	t.Setenv("AI_GATEWAY_ID", "my-gw")
	t.Setenv("AI_GATEWAY_TOKEN", "cf-token")
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()
	c, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if c.AIGatewayAccountID != "acct123" || c.AIGatewayID != "my-gw" || c.AIGatewayToken != "cf-token" {
		t.Errorf("gateway config = %q/%q/%q", c.AIGatewayAccountID, c.AIGatewayID, c.AIGatewayToken)
	}
	if !c.AIGatewayEnabled() {
		t.Error("AIGatewayEnabled = false with account+gateway set")
	}
}

func TestHalfConfiguredGatewayIsAStartupError(t *testing.T) {
	// Silently going direct would send traffic and spend outside the gateway.
	for _, env := range []map[string]string{
		{"AI_GATEWAY_ACCOUNT_ID": "acct123"},
		{"AI_GATEWAY_ID": "my-gw"},
	} {
		v := viper.New()
		setDefaults(v)
		for k, val := range env {
			t.Setenv(k, val)
		}
		v.AutomaticEnv()
		if _, err := Load(v); err == nil {
			t.Errorf("Load with %v = nil error, want a startup error", env)
		}
		for k := range env {
			t.Setenv(k, "")
		}
	}
}
