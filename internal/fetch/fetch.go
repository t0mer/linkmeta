package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/net/html/charset"
)

// Result is a fetched, UTF-8 normalized page.
type Result struct {
	HTML     []byte
	FinalURL string
}

// Fetcher performs size-capped, charset-normalized HTTP GETs.
type Fetcher struct {
	client    *http.Client
	userAgent string
	maxBytes  int64
}

// New builds a Fetcher. maxBytes caps the read body (e.g. 3<<20).
func New(timeout time.Duration, userAgent string, maxBytes int64) *Fetcher {
	return &Fetcher{
		client:    &http.Client{Timeout: timeout},
		userAgent: userAgent,
		maxBytes:  maxBytes,
	}
}

// Fetch GETs rawURL, follows redirects, caps the body, and normalizes to UTF-8.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept-Language", "he,en;q=0.8")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("http status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, f.maxBytes)
	// charset.NewReader sniffs Content-Type + <meta> + BOM and converts to UTF-8.
	utf8Reader, err := charset.NewReader(limited, resp.Header.Get("Content-Type"))
	if err != nil {
		return Result{}, fmt.Errorf("charset reader: %w", err)
	}
	body, err := io.ReadAll(utf8Reader)
	if err != nil {
		return Result{}, fmt.Errorf("read body: %w", err)
	}
	return Result{HTML: body, FinalURL: resp.Request.URL.String()}, nil
}
