package extract

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRunesSafe(t *testing.T) {
	s := "אבגדה" // 5 Hebrew runes, 10 bytes
	out := TruncateRunes(s, 3)
	if utf8.RuneCountInString(out) != 3 {
		t.Errorf("rune count = %d, want 3", utf8.RuneCountInString(out))
	}
	if !utf8.ValidString(out) {
		t.Error("truncation produced invalid UTF-8")
	}
}

func TestTruncateRunesShorterThanMax(t *testing.T) {
	if got := TruncateRunes("hi", 10); got != "hi" {
		t.Errorf("got %q", got)
	}
}

func TestCollapseWhitespace(t *testing.T) {
	if got := CollapseWhitespace("a\n\n  b\t c "); got != "a b c" {
		t.Errorf("got %q", got)
	}
}

func TestReadableTextFallbackStripsScript(t *testing.T) {
	html := []byte(`<html><body><script>var x=1;</script>
	<nav>menu</nav><p>Hello world content here</p><footer>foot</footer></body></html>`)
	txt := ReadableText(html, "http://example.com")
	if !strings.Contains(txt, "Hello world content") {
		t.Errorf("missing content: %q", txt)
	}
	if strings.Contains(txt, "var x") {
		t.Errorf("script not stripped: %q", txt)
	}
}

func TestReadableTextRealArticle(t *testing.T) {
	// A rich article body that readability should recognize as main content.
	var sb strings.Builder
	sb.WriteString(`<html><head><title>Doc</title></head><body><article>`)
	for i := 0; i < 6; i++ {
		sb.WriteString(`<p>This is a meaningful paragraph of readable article text that carries substance and context for the reader to understand the topic.</p>`)
	}
	sb.WriteString(`</article></body></html>`)
	txt := ReadableText([]byte(sb.String()), "http://example.com/post")
	if !strings.Contains(txt, "meaningful paragraph") {
		t.Errorf("readable text missing article content: %q", txt)
	}
}
