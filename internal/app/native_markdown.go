package app

import (
	"context"
	"net/url"
	"path"
	"strings"
	"time"
)

const maxNativeMarkdownBytes = 1024 * 1024

// Documentation sites frequently expose canonical HTML and an authored
// Markdown representation. Use bounded GET probes instead of HEAD:
// Stripe sends Content-Length: 0 on a successful Markdown HEAD response,
// and Cloudflare uses /index.md for directory URLs.
func markdownCandidateURLs(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	p := u.Path
	if strings.HasSuffix(strings.ToLower(p), ".md") {
		return []string{rawURL}
	}
	var candidates []string
	if strings.HasSuffix(p, "/") || p == "" {
		candidates = append(candidates, strings.TrimSuffix(p, "/")+"/index.md")
	} else if path.Ext(p) == "" {
		candidates = append(candidates, p+".md", p+"/index.md")
	} else {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, candidate := range candidates {
		next := *u
		next.Path = candidate
		next.RawPath = ""
		next.Fragment = ""
		next.RawFragment = ""
		value := next.String()
		if !seen[value] && validateSourceURL(value) == nil {
			out = append(out, value)
			seen[value] = true
		}
	}
	return out
}

func usableNativeMarkdown(content string) bool {
	text := strings.TrimSpace(content)
	if len(text) < 18 || strings.IndexByte(text, 0) >= 0 || challengeReason(text) != "" {
		return false
	}
	prefix := strings.ToLower(text[:min(len(text), 700)])
	if strings.Contains(prefix, "<!doctype html") || strings.Contains(prefix, "<html") ||
		strings.Contains(prefix, "<head") || strings.Contains(prefix, "<body") {
		return false
	}
	return true
}

func readNativeMarkdown(rawURL string) (*MarkdownResult, error) {
	for _, candidate := range markdownCandidateURLs(rawURL) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		body, err := fetchPublicText(ctx, candidate, maxNativeMarkdownBytes)
		cancel()
		if err != nil || !usableNativeMarkdown(body) {
			continue
		}
		title := firstHeadingOrFallback(body, path.Base(rawURL))
		return &MarkdownResult{URL: rawURL, SourceURL: candidate, Title: title, Markdown: body}, nil
	}
	return nil, nil
}
