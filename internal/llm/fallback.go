package llm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/t0mer/linkmeta/internal/metrics"
)

// Fallback runs a primary backend and falls back to a secondary one when the
// primary errors. It exists so a cloud outage, rate limit, or expired key
// degrades to the local model rather than straight to category "Other".
type Fallback struct {
	primary   Client
	secondary Client
	log       *slog.Logger
}

// NewFallback wires a primary backend with a secondary one.
func NewFallback(primary, secondary Client, log *slog.Logger) *Fallback {
	return &Fallback{primary: primary, secondary: secondary, log: log}
}

// Complete tries the primary, then the secondary. The returned error wraps the
// primary failure so callers can see why the fallback was needed.
func (f *Fallback) Complete(ctx context.Context, req Request) (Result, error) {
	res, err := f.primary.Complete(ctx, req)
	if err == nil {
		return res, nil
	}
	f.log.Warn("primary llm failed; falling back", "stage", "llm", "err", err)
	metrics.LLMCallsTotal.WithLabelValues("fallback").Inc()

	res, secErr := f.secondary.Complete(ctx, req)
	if secErr != nil {
		return Result{}, fmt.Errorf("primary: %w; fallback: %v", err, secErr)
	}
	return res, nil
}

// Version reports healthy when either backend is usable.
func (f *Fallback) Version(ctx context.Context) error {
	return f.eitherHealthy(f.primary.Version(ctx), f.secondary.Version(ctx))
}

// CheckModel reports healthy when either backend has a usable model.
func (f *Fallback) CheckModel(ctx context.Context) error {
	return f.eitherHealthy(f.primary.CheckModel(ctx), f.secondary.CheckModel(ctx))
}

func (f *Fallback) eitherHealthy(primaryErr, secondaryErr error) error {
	if primaryErr == nil || secondaryErr == nil {
		return nil
	}
	return fmt.Errorf("primary: %w; fallback: %v", primaryErr, secondaryErr)
}
