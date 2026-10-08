package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type scrapedPage struct {
	Title    string
	Markdown string
}

func usableScrapedMarkdown(markdown string) bool {
	text := strings.ToLower(strings.TrimSpace(markdown))
	if len(text) < 10 {
		return false
	}
	for _, marker := range []string{
		"just a moment...", "checking your browser", "verify you are human",
		"enable javascript and cookies", "access denied | cloudflare",
		"attention required! | cloudflare", "no content extracted",
	} {
		if strings.Contains(text, marker) && len(text) < 2000 {
			return false
		}
	}
	return true
}

func firecrawlScrape(rawURL, apiKey, proxy string) (scrapedPage, error) {
	requestBody := map[string]any{
		"url":                rawURL,
		"formats":            []string{"markdown"},
		"onlyMainContent":    true,
		"blockAds":           true,
		"removeBase64Images": true,
		"maxAge":             30 * 60 * 1000, // Firecrawl's built-in cache, 30 minutes.
		"excludeTags":        []string{"script", "style", "meta", "noscript", "svg", "img", "nav", "footer", "header", "aside", ".advertisement", "#ad"},
	}
	if proxy != "" {
		requestBody["proxy"] = proxy
		// A blocked cached result must not mask an actual enhanced retry.
		requestBody["maxAge"] = 0
	}
	if strings.HasSuffix(strings.ToLower(rawURL), ".pdf") {
		requestBody["parsers"] = []string{"pdf"}
	}
	data, err := getFirecrawlQueue().enqueue(rawURL, func() (map[string]any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 32*time.Second)
		defer cancel()
		body, err := doJSONRequest(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/scrape", map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		}, requestBody)
		if err != nil {
			return nil, err
		}
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode Firecrawl scrape: %w", err)
		}
		if success, _ := parsed["success"].(bool); !success {
			return nil, fmt.Errorf("Firecrawl scrape failed: %v", parsed["error"])
		}
		return parsed, nil
	})
	if err != nil {
		return scrapedPage{}, err
	}
	pageData, _ := data["data"].(map[string]any)
	metadata, _ := pageData["metadata"].(map[string]any)
	page := scrapedPage{
		Title:    stringValue(metadata["title"]),
		Markdown: stringValue(pageData["markdown"]),
	}
	if status, ok := metadata["statusCode"].(float64); ok && status >= 400 {
		return page, fmt.Errorf("source responded with HTTP %d", int(status))
	}
	return page, nil
}

func firecrawlJSON(ctx context.Context, method, endpoint, apiKey string, data map[string]any) (map[string]any, error) {
	body, err := doJSONRequest(ctx, method, endpoint, map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}, data)
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if success, ok := parsed["success"].(bool); ok && !success {
		return nil, fmt.Errorf("Firecrawl browser: %v", parsed["error"])
	}
	return parsed, nil
}

// quoteShellURL escapes a validated URL for the Firecrawl sandbox's bash mode.
// This invokes only the installed agent-browser CLI, never model-written code.
func quoteShellURL(rawURL string) string {
	return "'" + strings.ReplaceAll(rawURL, "'", "'\\''") + "'"
}

func firecrawlBrowserFallback(rawURL, apiKey string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 28*time.Second)
	defer cancel()
	// Failed scrape responses do not reliably include a scrape-bound browser ID.
	// Open a disposable session with the standalone browser API instead.
	session, err := firecrawlJSON(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/interact", apiKey, map[string]any{
		"ttl": 60, "activityTtl": 30,
	})
	if err != nil {
		return "", "", err
	}
	sessionID := stringValue(session["id"])
	if sessionID == "" {
		return "", "", fmt.Errorf("Firecrawl browser returned no session ID")
	}
	endpoint := "https://api.firecrawl.dev/v2/interact/" + url.PathEscape(sessionID)
	defer stopFirecrawlBrowser(endpoint, apiKey)
	result, err := firecrawlJSON(ctx, http.MethodPost, endpoint+"/execute", apiKey, map[string]any{
		"code":     "agent-browser open " + quoteShellURL(rawURL) + " && agent-browser scrape",
		"language": "bash",
		"timeout":  20,
	})
	if err != nil {
		return "", "", err
	}
	if code, ok := result["exitCode"].(float64); ok && code != 0 {
		return "", "", fmt.Errorf("browser command exited with %d", int(code))
	}
	text := stringValue(result["stdout"])
	if text == "" {
		text = stringValue(result["output"])
	}
	if text == "" {
		text = stringValue(result["result"])
	}
	return "Browser extraction", text, nil
}

func stopFirecrawlBrowser(endpoint, apiKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	// Best-effort cleanup regardless of whether extraction succeeded.
	_, _ = firecrawlJSON(ctx, http.MethodDelete, endpoint, apiKey, nil)
}
