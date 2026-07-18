package config

import (
	"testing"
	"time"

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
