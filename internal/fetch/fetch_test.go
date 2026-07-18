package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func TestFetchUTF8(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("UA = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body>שלום</body></html>"))
	}))
	defer srv.Close()

	f := New(5*time.Second, "test-agent", 3<<20)
	res, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.HTML), "שלום") {
		t.Errorf("body missing Hebrew: %q", res.HTML)
	}
}

func TestFetchWindows1255(t *testing.T) {
	enc := charmap.Windows1255.NewEncoder()
	body, err := enc.String("<html><body>שלום</body></html>")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=windows-1255")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	f := New(5*time.Second, "test-agent", 3<<20)
	res, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.HTML), "שלום") {
		t.Errorf("windows-1255 not normalized to UTF-8: %q", res.HTML)
	}
}

func TestFetchNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	f := New(5*time.Second, "ua", 3<<20)
	if _, err := f.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestFetchSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("a", 100)))
	}))
	defer srv.Close()
	f := New(5*time.Second, "ua", 10)
	res, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HTML) > 10 {
		t.Errorf("body not capped: %d bytes", len(res.HTML))
	}
}
