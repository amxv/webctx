package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type scrapedPage struct {
	Title       string
	Markdown    string
	ToolSummary string
}

func usableScrapedMarkdown(markdown string) bool {
	return usableSourceContent(markdown)
}

func firecrawlScrape(rawURL, apiKey, proxy string, domainTools bool) (scrapedPage, error) {
	requestBody := map[string]any{
		"url":                rawURL,
		"formats":            []string{"markdown"},
		"domainTools":        domainTools, // Only explicit read-link should discover Alexandria capabilities.
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
	// Waiting for a shared 10-RPM permit can legitimately take a full minute.
	// HTTP transport still has a separate 32s timeout after admission.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	body, err := doJSONRequest(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/scrape", map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}, requestBody)
	// Alexandria is a separately enabled Firecrawl capability. A 403 from
	// domain matching must never break ordinary page extraction for users that
	// haven't enabled Alexandria or have zero-data-retention configured.
	if err != nil && strings.Contains(err.Error(), "403") {
		requestBody["domainTools"] = false
		body, err = doJSONRequest(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/scrape", map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		}, requestBody)
	}
	if err != nil {
		return scrapedPage{}, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return scrapedPage{}, fmt.Errorf("decode Firecrawl scrape: %w", err)
	}
	if success, _ := parsed["success"].(bool); !success {
		return scrapedPage{}, fmt.Errorf("Firecrawl scrape failed: %v", parsed["error"])
	}
	// The shared Firecrawl admission layer is applied in doRawRequest, so
	// retries and other Firecrawl endpoints are accounted for independently.
	pageData, _ := parsed["data"].(map[string]any)
	metadata, _ := pageData["metadata"].(map[string]any)
	page := scrapedPage{
		Title:       stringValue(metadata["title"]),
		Markdown:    stringValue(pageData["markdown"]),
		ToolSummary: formatAlexandriaTools(pageData["tools"]),
	}
	if status, ok := metadata["statusCode"].(float64); ok && status >= 400 {
		return page, fmt.Errorf("source responded with HTTP %d", int(status))
	}
	return page, nil
}

func firecrawlJSON(ctx context.Context, method, endpoint, apiKey string, data map[string]any) (map[string]any, error) {
	return firecrawlJSONWithPermit(ctx, method, endpoint, apiKey, data, nil)
}

// The interact session's creation quota and browser slot are both reserved by
// firecrawlBrowserAccount. A dummy permit skips double-charging the quota;
// the actual slot stays held until session teardown.
func firecrawlJSONWithPermit(ctx context.Context, method, endpoint, apiKey string, data map[string]any, preselected *firecrawlPermit) (map[string]any, error) {
	var body []byte
	var err error
	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}
	var response []byte
	if preselected == nil {
		response, err = doRawRequest(ctx, method, endpoint, map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		}, body)
	} else {
		req, createErr := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(body)))
		if createErr != nil {
			return nil, createErr
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		response, err = executeRawRequest(req, preselected)
	}
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(response, &parsed); err != nil {
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
	// Prefer an account with an available browser session; all independent CLI
	// processes share the same two-slot pool for each configured account.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	selectedKey, session, sessionPermit, err := startFirecrawlBrowser(ctx)
	if err != nil {
		return "", "", err
	}
	apiKey = selectedKey
	defer sessionPermit.release()
	// This disposable session has already reserved its per-account quota and
	// browser slot. Its subsequent commands must use the originating key.
	sessionID := stringValue(session["id"])
	if sessionID == "" {
		return "", "", fmt.Errorf("Firecrawl browser returned no session ID")
	}
	endpoint := "https://api.firecrawl.dev/v2/interact/" + url.PathEscape(sessionID)
	defer stopFirecrawlBrowser(endpoint, apiKey)
	result, err := firecrawlJSON(ctx, http.MethodPost, endpoint+"/execute", apiKey, map[string]any{
		// The sandbox's agent-browser does not expose a "scrape" subcommand
		// even though Firecrawl's examples use it. DOM innerText is supported.
		// Suppress navigation's banner so stdout contains only the evaluated text.
		"code":     "agent-browser open " + quoteShellURL(rawURL) + " >/dev/null && agent-browser eval 'document.body.innerText'",
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
	// agent-browser eval serializes JS strings as JSON with surrounding quotes.
	// Decode those escapes rather than handing the model a JSON string literal.
	if parsed, err := strconv.Unquote(strings.TrimSpace(text)); err == nil {
		text = parsed
	}
	if reason := challengeReason(text); reason != "" {
		return "", "", fmt.Errorf("browser fallback returned %s rather than documentation", reason)
	}
	return "Browser extraction", text, nil
}

func stopFirecrawlBrowser(endpoint, apiKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	// Best-effort cleanup regardless of whether extraction succeeded.
	_, _ = firecrawlJSON(ctx, http.MethodDelete, endpoint, apiKey, nil)
}
