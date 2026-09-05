package config

import (
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// DefaultUserAgent is a realistic desktop Chrome UA used for page fetches.
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

const defaultCategories = "Technology,News,Social,Food,Health,Shopping,Finance," +
	"Education,Entertainment,Travel,Science,Sports,Home,Other"

// Config holds all runtime settings.
type Config struct {
	Port             int
	OllamaURL        string
	OllamaModel      string
	OllamaKeepAlive  string
	Categories       []string
	CategoryLanguage string
	FetchTimeout     time.Duration
	LLMTimeout       time.Duration
	MaxTextChars     int
	// LLMMaxTokens caps generation length (Ollama num_predict). 0 = uncapped.
	LLMMaxTokens int
	UserAgent    string
	AllowPrivate bool
	// ForceLLM makes the model generate description and keywords even when the
	// page supplies them; its answers then win the merge. Title stays deterministic.
	ForceLLM bool
}

// viper keys double as env var names (AutomaticEnv upper-cases the key).
const (
	kPort         = "PORT"
	kOllamaURL    = "OLLAMA_URL"
	kOllamaModel  = "OLLAMA_MODEL"
	kOllamaKeep   = "OLLAMA_KEEP_ALIVE"
	kCategories   = "CATEGORIES"
	kCategoryLang = "CATEGORY_LANGUAGE"
	kFetchTimeout = "FETCH_TIMEOUT"
	kLLMTimeout   = "LLM_TIMEOUT"
	kMaxTextChars = "MAX_TEXT_CHARS"
	kLLMMaxTokens = "LLM_MAX_TOKENS"
	kUserAgent    = "USER_AGENT"
	kAllowPrivate = "ALLOW_PRIVATE_TARGETS"
	kForceLLM     = "FORCE_LLM"
)

func setDefaults(v *viper.Viper) {
	v.SetDefault(kPort, 8080)
	v.SetDefault(kOllamaURL, "http://localhost:11434")
	v.SetDefault(kOllamaModel, "qwen2.5:3b-instruct")
	v.SetDefault(kOllamaKeep, "24h")
	v.SetDefault(kCategories, defaultCategories)
	v.SetDefault(kCategoryLang, "English")
	v.SetDefault(kFetchTimeout, "20s")
	v.SetDefault(kLLMTimeout, "180s")
	v.SetDefault(kMaxTextChars, 3000)
	v.SetDefault(kLLMMaxTokens, 256)
	v.SetDefault(kUserAgent, DefaultUserAgent)
	v.SetDefault(kAllowPrivate, false)
	v.SetDefault(kForceLLM, false)
}

// SetDefaults applies default values on v (exported wrapper for main).
func SetDefaults(v *viper.Viper) { setDefaults(v) }

// BindFlags registers pflags and binds them to viper keys (flags > env > yaml).
func BindFlags(v *viper.Viper, fs *pflag.FlagSet) {
	fs.Int("port", 8080, "HTTP listen port")
	fs.String("ollama-url", "http://localhost:11434", "Ollama base URL")
	fs.String("ollama-model", "qwen2.5:3b-instruct", "Ollama model")
	fs.String("ollama-keep-alive", "24h", "Ollama keep_alive")
	fs.String("categories", defaultCategories, "comma-separated category list")
	fs.String("category-language", "English", "language for the returned category label")
	fs.Duration("fetch-timeout", 20*time.Second, "page fetch timeout")
	fs.Duration("llm-timeout", 180*time.Second, "LLM call timeout")
	fs.Int("max-text-chars", 3000, "max readable chars sent to model")
	fs.Int("llm-max-tokens", 256, "cap on generated tokens (Ollama num_predict); 0 = uncapped")
	fs.String("user-agent", DefaultUserAgent, "fetch User-Agent")
	fs.Bool("allow-private-targets", false, "allow fetching private/loopback/link-local URLs (SSRF guard off)")
	fs.Bool("force-llm", false, "always let the LLM write description and keywords, overriding the page's own meta tags")

	_ = v.BindPFlag(kPort, fs.Lookup("port"))
	_ = v.BindPFlag(kOllamaURL, fs.Lookup("ollama-url"))
	_ = v.BindPFlag(kOllamaModel, fs.Lookup("ollama-model"))
	_ = v.BindPFlag(kOllamaKeep, fs.Lookup("ollama-keep-alive"))
	_ = v.BindPFlag(kCategories, fs.Lookup("categories"))
	_ = v.BindPFlag(kCategoryLang, fs.Lookup("category-language"))
	_ = v.BindPFlag(kFetchTimeout, fs.Lookup("fetch-timeout"))
	_ = v.BindPFlag(kLLMTimeout, fs.Lookup("llm-timeout"))
	_ = v.BindPFlag(kMaxTextChars, fs.Lookup("max-text-chars"))
	_ = v.BindPFlag(kLLMMaxTokens, fs.Lookup("llm-max-tokens"))
	_ = v.BindPFlag(kUserAgent, fs.Lookup("user-agent"))
	_ = v.BindPFlag(kAllowPrivate, fs.Lookup("allow-private-targets"))
	_ = v.BindPFlag(kForceLLM, fs.Lookup("force-llm"))
}

// categoryLanguage trims the configured value and defaults blanks to English.
func categoryLanguage(raw string) string {
	if s := strings.TrimSpace(raw); s != "" {
		return s
	}
	return "English"
}

func parseCategories(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Load materializes a Config from viper.
func Load(v *viper.Viper) (Config, error) {
	return Config{
		Port:             v.GetInt(kPort),
		OllamaURL:        strings.TrimRight(v.GetString(kOllamaURL), "/"),
		OllamaModel:      v.GetString(kOllamaModel),
		OllamaKeepAlive:  v.GetString(kOllamaKeep),
		Categories:       parseCategories(v.GetString(kCategories)),
		CategoryLanguage: categoryLanguage(v.GetString(kCategoryLang)),
		FetchTimeout:     v.GetDuration(kFetchTimeout),
		LLMTimeout:       v.GetDuration(kLLMTimeout),
		MaxTextChars:     v.GetInt(kMaxTextChars),
		LLMMaxTokens:     v.GetInt(kLLMMaxTokens),
		UserAgent:        v.GetString(kUserAgent),
		AllowPrivate:     v.GetBool(kAllowPrivate),
		ForceLLM:         v.GetBool(kForceLLM),
	}, nil
}
