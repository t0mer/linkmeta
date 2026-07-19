package extract

import "testing"

func TestParseMetaPrefersTitleTag(t *testing.T) {
	html := []byte(`<html><head><title>Real Title</title>
	<meta property="og:title" content="OG Title"></head></html>`)
	m, err := ParseMeta(html)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Real Title" {
		t.Errorf("Title = %q", m.Title)
	}
}

func TestParseMetaFallsBackToOG(t *testing.T) {
	html := []byte(`<html><head>
	<meta property="og:title" content="OG Title">
	<meta property="og:description" content="OG Desc"></head></html>`)
	m, _ := ParseMeta(html)
	if m.Title != "OG Title" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Description != "OG Desc" {
		t.Errorf("Description = %q", m.Description)
	}
}

func TestParseMetaDescriptionPrefersNameMeta(t *testing.T) {
	html := []byte(`<html><head>
	<meta name="description" content="Name Desc">
	<meta property="og:description" content="OG Desc"></head></html>`)
	m, _ := ParseMeta(html)
	if m.Description != "Name Desc" {
		t.Errorf("Description = %q", m.Description)
	}
}

func TestParseKeywordsDedupeLowercaseCap(t *testing.T) {
	kw := "Go, go, Rust, rust, A, B, C, D, E, F, G, H"
	html := []byte(`<html><head><meta name="keywords" content="` + kw + `"></head></html>`)
	m, _ := ParseMeta(html)
	if len(m.Keywords) != 10 {
		t.Fatalf("len = %d, want 10 (cap)", len(m.Keywords))
	}
	if m.Keywords[0] != "go" || m.Keywords[1] != "rust" {
		t.Errorf("keywords = %v", m.Keywords)
	}
}

func TestParseKeywordsNeverNil(t *testing.T) {
	m, _ := ParseMeta([]byte(`<html></html>`))
	if m.Keywords == nil {
		t.Error("Keywords is nil, want empty slice")
	}
}

func TestNormalizeKeywords(t *testing.T) {
	got := NormalizeKeywords([]string{" Go ", "go", "", "Rust"})
	if len(got) != 2 || got[0] != "go" || got[1] != "rust" {
		t.Errorf("got %v, want [go rust]", got)
	}
	if NormalizeKeywords(nil) == nil {
		t.Error("nil input should yield empty slice, not nil")
	}
}
