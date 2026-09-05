package llm

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type scriptedClient struct {
	res    Result
	err    error
	calls  int
	verErr error
}

func (s *scriptedClient) Complete(ctx context.Context, r Request) (Result, error) {
	s.calls++
	return s.res, s.err
}
func (s *scriptedClient) Version(ctx context.Context) error    { return s.verErr }
func (s *scriptedClient) CheckModel(ctx context.Context) error { return s.verErr }

func TestFallbackUsesPrimaryWhenItWorks(t *testing.T) {
	primary := &scriptedClient{res: Result{Category: "News"}}
	secondary := &scriptedClient{res: Result{Category: "Other"}}
	f := NewFallback(primary, secondary, slog.Default())

	res, err := f.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Category != "News" {
		t.Errorf("category = %q, want the primary's answer", res.Category)
	}
	if secondary.calls != 0 {
		t.Error("secondary called while the primary succeeded")
	}
}

func TestFallbackFallsBackOnPrimaryError(t *testing.T) {
	primary := &scriptedClient{err: errors.New("anthropic 429")}
	secondary := &scriptedClient{res: Result{Category: "Technology"}}
	f := NewFallback(primary, secondary, slog.Default())

	res, err := f.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatalf("fallback must absorb the primary error: %v", err)
	}
	if res.Category != "Technology" {
		t.Errorf("category = %q, want the secondary's answer", res.Category)
	}
	if secondary.calls != 1 {
		t.Errorf("secondary calls = %d, want 1", secondary.calls)
	}
}

func TestFallbackBothFailReturnsError(t *testing.T) {
	primary := &scriptedClient{err: errors.New("anthropic down")}
	secondary := &scriptedClient{err: errors.New("ollama down")}
	f := NewFallback(primary, secondary, slog.Default())

	_, err := f.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("want an error when both backends fail")
	}
	// The service degrades to "Other" on this error, so it must name both causes.
	if !errors.Is(err, primary.err) {
		t.Errorf("err = %v, want it to wrap the primary failure", err)
	}
}

func TestFallbackHealthyIfEitherBackendIsHealthy(t *testing.T) {
	f := NewFallback(&scriptedClient{verErr: errors.New("no key")}, &scriptedClient{}, slog.Default())
	if err := f.Version(context.Background()); err != nil {
		t.Errorf("Version = %v, want nil when the secondary is usable", err)
	}
	if err := f.CheckModel(context.Background()); err != nil {
		t.Errorf("CheckModel = %v, want nil when the secondary is usable", err)
	}

	bad := NewFallback(&scriptedClient{verErr: errors.New("a")}, &scriptedClient{verErr: errors.New("b")}, slog.Default())
	if err := bad.Version(context.Background()); err == nil {
		t.Error("Version = nil, want error when neither backend is usable")
	}
}
