package extract

import (
	"bytes"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Meta holds deterministically-parsed page metadata.
type Meta struct {
	Title       string
	Description string
	Keywords    []string
}

func attr(doc *goquery.Document, selector, attrName string) string {
	v, _ := doc.Find(selector).First().Attr(attrName)
	return strings.TrimSpace(v)
}

// ParseMeta reads title/description/keywords from HTML with og: fallbacks.
func ParseMeta(html []byte) (Meta, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return Meta{Keywords: []string{}}, err
	}

	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title == "" {
		title = attr(doc, `meta[property="og:title"]`, "content")
	}

	desc := attr(doc, `meta[name="description"]`, "content")
	if desc == "" {
		desc = attr(doc, `meta[property="og:description"]`, "content")
	}

	return Meta{
		Title:       title,
		Description: desc,
		Keywords:    NormalizeKeywords(strings.Split(attr(doc, `meta[name="keywords"]`, "content"), ",")),
	}, nil
}

// NormalizeKeywords trims, lowercases, dedupes and caps at 10. Never nil.
func NormalizeKeywords(in []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, p := range in {
		k := strings.ToLower(strings.TrimSpace(p))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
		if len(out) == 10 {
			break
		}
	}
	return out
}
