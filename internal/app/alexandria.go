package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// formatAlexandriaTools describes discovered capabilities, not their data.
// Discovery is free but actual provider execution is not, and is never
// silently authorized by reading a link or searching the web.
func formatAlexandriaTools(raw any) string {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	parts := []string{"**Available structured-data capabilities (not executed):**"}
	count := 0
	for _, item := range items {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provider := safeToolField(tool["provider"], 80)
		capability := safeToolField(tool["capability"], 160)
		if provider == "" || capability == "" {
			continue
		}
		description := safeToolField(tool["description"], 180)
		if description == "" {
			description = safeToolField(tool["name"], 180)
		}
		line := fmt.Sprintf("- `%s/%s`", provider, capability)
		if description != "" {
			line += ": " + description
		}
		parts = append(parts, line)
		count++
		if count >= 3 {
			break
		}
	}
	if count == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func safeToolField(value any, limit int) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "`", "'")
	if len(s) > limit {
		s = s[:limit] + "…"
	}
	return s
}

func withSourceTools(markdown, toolSummary string) string {
	if toolSummary == "" {
		return markdown
	}
	return markdown + "\n\n---\n\n" + toolSummary + "\n\n*Matched through Firecrawl Alexandria. Discovery does not verify, license, or execute these providers.*"
}

func discoverAlexandria(query string) string {
	apiKey := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
	if apiKey == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	response, err := doJSONRequest(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/search", map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}, map[string]any{
		"query":      query,
		"sources":    []string{"alexandria"},
		"limit":      3,
		"toolDetail": "compact",
	})
	if err != nil {
		return "" // unavailable Alexandria is never a search failure
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Tools []any `json:"tools"`
		} `json:"data"`
	}
	if json.Unmarshal(response, &result) != nil || !result.Success {
		return ""
	}
	return formatAlexandriaTools(result.Data.Tools)
}
