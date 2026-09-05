package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/t0mer/linkmeta/internal/config"
	"github.com/t0mer/linkmeta/internal/fetch"
	"github.com/t0mer/linkmeta/internal/httpapi"
	"github.com/t0mer/linkmeta/internal/llm"
	"github.com/t0mer/linkmeta/internal/metrics"
	"github.com/t0mer/linkmeta/internal/service"
)

// version is injected at build time via -ldflags "-X main.version=<v>".
var version = "dev"

func main() {
	v := viper.New()
	root := &cobra.Command{
		Use:     "linkmeta",
		Short:   "Self-hosted URL metadata extraction service",
		Version: version,
		RunE:    func(cmd *cobra.Command, _ []string) error { return run(v) },
	}
	config.BindFlags(v, root.Flags())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(v *viper.Viper) error {
	v.AutomaticEnv()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) {
			return fmt.Errorf("read config: %w", err)
		}
	}
	config.SetDefaults(v)

	cfg, err := config.Load(v)
	if err != nil {
		return err
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	metrics.MustRegister(prometheus.DefaultRegisterer)

	f := fetch.New(cfg.FetchTimeout, cfg.UserAgent, 3<<20, cfg.AllowPrivate)
	ollama := llm.NewOllama(cfg.OllamaURL, cfg.OllamaModel, cfg.OllamaKeepAlive, cfg.LLMTimeout)
	ollama.SetMaxTokens(cfg.LLMMaxTokens)

	// Anthropic runs as the primary with Ollama kept as the fallback, so a cloud
	// outage, rate limit, or bad key degrades to the local model instead of
	// straight to category "Other".
	var llmClient llm.Client = ollama
	if cfg.LLMProvider == "anthropic" {
		if cfg.AnthropicAPIKey == "" {
			log.Warn("llm provider is anthropic but ANTHROPIC_API_KEY is empty; every call will fall back to ollama")
		}
		claude := llm.NewAnthropic(cfg.AnthropicAPIKey, cfg.AnthropicModel, cfg.LLMTimeout)
		llmClient = llm.NewFallback(claude, ollama, log)
	}
	svc := service.New(cfg, f, llmClient, log)
	api := httpapi.NewAPI(svc, llmClient, cfg, version, log)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           api.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("listening", "port", cfg.Port, "provider", cfg.LLMProvider,
			"model", cfg.OllamaModel, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}
