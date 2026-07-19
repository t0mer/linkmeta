package extract

import (
	"bytes"
	nurl "net/url"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
)

// TruncateRunes returns at most max runes, never splitting a UTF-8 sequence.
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// CollapseWhitespace collapses any run of whitespace to a single space, trimmed.
func CollapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ReadableText extracts main readable text, falling back to stripped body text.
func ReadableText(html []byte, pageURL string) string {
	if txt := readabilityText(html, pageURL); txt != "" {
		return CollapseWhitespace(txt)
	}
	return CollapseWhitespace(bodyText(html))
}

// readabilityText isolates ALL go-readability usage. The v2 fork exposes text
// via Article.RenderText(w) (fields became methods). Returns "" on any error or
// empty result so ReadableText can fall back to bodyText.
func readabilityText(html []byte, pageURL string) string {
	u, err := nurl.Parse(pageURL)
	if err != nil {
		u = nil
	}
	art, err := readability.FromReader(bytes.NewReader(html), u)
	if err != nil || art.Node == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := art.RenderText(&buf); err != nil {
		return ""
	}
	return buf.String()
}

// bodyText is the crude fallback: body text minus non-content elements.
func bodyText(html []byte) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return ""
	}
	doc.Find("script, style, nav, header, footer, noscript").Remove()
	return doc.Find("body").Text()
}
