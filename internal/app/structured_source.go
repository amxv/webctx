package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxDirectSourceBytes = 1024 * 1024

var (
	jsonLDPattern   = regexp.MustCompile(`(?is)<script\b[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script\s*>`)
	nextDataPattern = regexp.MustCompile(`(?is)<script\b[^>]*id\s*=\s*["']__NEXT_DATA__["'][^>]*>(.*?)</script\s*>`)
)

// These URLs are already structured source documents. Never scrape a raw
// OpenAPI schema through HTML rendering when we can retrieve its actual bytes.
func directSourceLanguage(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".json", ".jsonl":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".graphql", ".gql":
		return "graphql"
	case ".txt":
		return "text"
	default:
		return ""
	}
}

// A bounded, redirect-checked direct read prevents accidental traversal from
// an accepted public source to an internal address.
func fetchPublicText(ctx context.Context, rawURL string, maxBytes int64) (string, error) {
	if err := validateSourceURL(rawURL); err != nil {
		return "", err
	}
	client := *http.DefaultClient
	oldCheck := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := validateSourceURL(req.URL.String()); err != nil {
			return err
		}
		if oldCheck != nil {
			return oldCheck(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json, text/markdown, text/plain, text/html, application/yaml, */*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("source HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return "", fmt.Errorf("source is larger than the supported %d bytes", maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("source exceeded the supported %d bytes", maxBytes)
	}
	return string(body), nil
}

func readDirectSource(rawURL string) (string, bool) {
	language := directSourceLanguage(rawURL)
	if language == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	body, err := fetchPublicText(ctx, rawURL, maxDirectSourceBytes)
	if err != nil || strings.TrimSpace(body) == "" {
		return "", false
	}
	if strings.Contains(strings.ToLower(body[:min(len(body), 256)]), "<html") {
		return "", false
	}
	if language == "json" && !json.Valid([]byte(body)) { // don't label HTML/garbage as JSON
		return "", false
	}
	return fmt.Sprintf("**URL:** %s\n\n```%s\n%s\n```", rawURL, language, strings.TrimSpace(body)), true
}

// Extract only public machine-readable metadata actually embedded in the
// original HTML. Never execute arbitrary scripts or speculate about hidden APIs.
func embeddedStructuredContext(rawURL, question string) string {
	if question == "" || directSourceLanguage(rawURL) != "" {
		return ""
	}
	if u, err := url.Parse(rawURL); err == nil && strings.EqualFold(u.Hostname(), "github.com") {
		return "" // GitHub's native APIs are more faithful than page hydration data.
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	html, err := fetchPublicText(ctx, rawURL, maxDirectSourceBytes)
	if err != nil || !strings.Contains(strings.ToLower(html), "<script") {
		return ""
	}
	var results []string
	for _, pattern := range []*regexp.Regexp{jsonLDPattern, nextDataPattern} {
		for _, match := range pattern.FindAllStringSubmatch(html, 3) {
			var raw any
			if json.Unmarshal([]byte(strings.TrimSpace(match[1])), &raw) != nil {
				continue
			}
			selection := selectedJSONFields(raw, question, 2500)
			if selection != "" {
				results = append(results, selection)
			}
		}
	}
	if len(results) == 0 {
		return ""
	}
	return "### Embedded source metadata\n\nStructured JSON embedded in the source page (not interpreted as an API response):\n\n" + strings.Join(results, "\n\n")
}

// selectedJSONFields preserves exact JSON values and JSON Pointers rather
// than returning syntactically broken chunks of a large schema or payload.
func selectedJSONFields(raw any, question string, budget int) string {
	type field struct {
		path  string
		value any
		score int
	}
	terms := queryTerms(question)
	fields := make([]field, 0, 32)
	var walk func(any, string, int)
	walk = func(value any, parent string, depth int) {
		if depth > 9 || len(fields) >= 500 {
			return
		}
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				pointer := parent + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
				score := termScore(key, terms)*4 + termScore(pointer, terms)
				if score > 0 {
					fields = append(fields, field{path: pointer, value: child, score: score})
				}
				walk(child, pointer, depth+1)
			}
		case []any:
			for i, child := range v {
				if i >= 20 { // never flatten unbounded payload collections
					break
				}
				walk(child, fmt.Sprintf("%s/%d", parent, i), depth+1)
			}
		}
	}
	walk(raw, "", 0)
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].score == fields[j].score {
			return fields[i].path < fields[j].path
		}
		return fields[i].score > fields[j].score
	})
	var out []string
	for _, f := range fields {
		value, err := json.MarshalIndent(f.value, "", "  ")
		if err != nil || len(value) > budget/2 || len(value) < 2 {
			continue
		}
		item := fmt.Sprintf("**JSON Pointer:** `%s`\n\n```json\n%s\n```", f.path, value)
		if len(strings.Join(out, "\n\n"))+len(item) > budget {
			break
		}
		out = append(out, item)
		if len(out) >= 5 {
			break
		}
	}
	if len(out) > 0 {
		return strings.Join(out, "\n\n") + "\n\n*[Selected JSON fields; full document at source URL.]*"
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return ""
	}
	if len(pretty)+20 <= budget {
		return "```json\n" + string(pretty) + "\n```"
	}
	// No relevant key matched: expose the shape without claiming completeness.
	if obj, ok := raw.(map[string]any); ok {
		keys := make([]string, 0, len(obj))
		for key := range obj {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 25 {
			keys = keys[:25]
		}
		return "**Top-level JSON keys:** " + strings.Join(keys, ", ") + " (full document at source URL)"
	}
	return "*[Structured document is larger than the inline budget; inspect original source.]*"
}

func focusedRawJSON(markdown, question string, budget int) string {
	// Raw JSON documents have one JSON fence just after the URL. Looking for
	// JSON examples in a long Markdown guide previously allowed the entire
	// preceding guide to escape the excerpt budget.
	if !strings.HasPrefix(strings.TrimSpace(markdown), "**URL:**") {
		return ""
	}
	begin := strings.Index(markdown, "```json\n")
	if begin < 0 {
		return ""
	}
	prefix := strings.TrimSpace(markdown[:begin])
	if len(prefix) > 400 || strings.Contains(prefix, "\n#") {
		return ""
	}
	jsonStart := begin + len("```json\n")
	end := strings.LastIndex(markdown, "\n```")
	if end <= jsonStart {
		return ""
	}
	var source any
	if json.Unmarshal([]byte(markdown[jsonStart:end]), &source) != nil {
		return ""
	}
	selected := selectedJSONFields(source, question, budget-250)
	if selected == "" {
		return ""
	}
	return strings.TrimSpace(markdown[:begin]) + "\n\n**Focused JSON evidence:**\n\n" + selected
}
