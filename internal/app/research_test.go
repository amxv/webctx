package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestReadLinkFocusedKeepsURLOnlyContract(t *testing.T) {
	called := 0
	reader := func(url string) (string, error) {
		called++
		if url != "https://docs.example.com/api" {
			t.Fatalf("unexpected URL: %s", url)
		}
		return "# Entire document\n\nno focus needed", nil
	}
	got, err := readLinkFocused("https://docs.example.com/api", "", reader)
	if err != nil || got != "# Entire document\n\nno focus needed" || called != 1 {
		t.Fatalf("URL-only path changed: %q, %v, calls=%d", got, err, called)
	}
}

func TestFocusedReadExpandsRelevantSameHostLinks(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	urls := map[string]string{
		"https://docs.example.com/guide":      "# Guide\n[Authentication](./auth)\n[Pagination](/pagination)\n[Offsite](https://evil.example/pagination)\n[Secret](./auth?token=secret)\n[Self](#pagination)",
		"https://docs.example.com/auth":       "# Authentication\nSet the Authorization header with your token.",
		"https://docs.example.com/pagination": "# Pagination\nPass next_cursor to read the next page.",
	}
	requested := make(map[string]int)
	var requestedMu sync.Mutex
	reader := func(url string) (string, error) {
		requestedMu.Lock()
		requested[url]++
		requestedMu.Unlock()
		value, ok := urls[url]
		if !ok {
			return "", errors.New("missing URL")
		}
		return value, nil
	}
	got, err := readLinkFocused("https://docs.example.com/guide", "How do authentication and pagination work?", reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"next_cursor", "Authorization header", "https://docs.example.com/auth", "https://docs.example.com/pagination", "**Coverage:** 3 source document(s)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in result:\n%s", want, got)
		}
	}
	if len(requested) != 3 {
		t.Fatalf("followed unsafe/irrelevant link: %+v", requested)
	}
}

func TestFocusedExcerptPrioritizesRelevantHeadingAndCode(t *testing.T) {
	large := "# Docs\n" + strings.Repeat("Large unrelated introduction.\n", 600) + "\n## Authenticated requests\nUse an Authorization header.\n```go\nheaders.Set(\"Authorization\", token)\n```\n\n## Other\n" + strings.Repeat("Unrelated text\n", 200)
	got := focusExcerpt(large, "How to authenticate requests with Go?", 3000)
	if !strings.Contains(got, "headers.Set") || strings.Contains(got, "Large unrelated introduction.Large") {
		t.Fatalf("did not select API code example: %s", got)
	}
	if len(got) > 3500 {
		t.Fatalf("excerpt unexpectedly large: %d", len(got))
	}
}

func TestSearchResearchReturnsEvidenceAndSupportingSource(t *testing.T) {
	results := []SearchDoc{
		{URL: "https://docs.example.com/guide", Title: "Guide"},
		{URL: "https://github.com/example/project", Title: "Source"},
		{URL: "https://other.example.com/blog", Title: "Blog"},
	}
	reader := func(url string) (string, error) {
		switch url {
		case "https://docs.example.com/guide":
			return "# Auth\nUse OAuth.\n[Pagination](/pagination)", nil
		case "https://docs.example.com/pagination":
			return "# Pagination\nnext_cursor is optional.", nil
		case "https://github.com/example/project":
			return "# Source\nOAuth settings are configurable.", nil
		case "https://other.example.com/blog":
			return "# Blog\nOAuth authorization examples.", nil
		default:
			return "", fmt.Errorf("unexpected URL: %s", url)
		}
	}
	got := enrichSearchWithReader("OAuth pagination", results, reader)
	if !strings.Contains(got, "Use OAuth.") || !strings.Contains(got, "next_cursor") || !strings.Contains(got, "Supporting reference") {
		t.Fatalf("search lacked fetched source context: %s", got)
	}
}

func TestFollowLinksRejectsOtherHostsSecretsAndAssets(t *testing.T) {
	markdown := "[API auth](https://other.example.com/auth) [API auth](/auth?api_key=private) [API auth](/image.png) [API auth](http://127.0.0.1/auth) [API auth](/auth)"
	got := relatedSourceLinks(markdown, "https://docs.example.com/guide", "api auth", 5)
	if len(got) != 1 || got[0].url != "https://docs.example.com/auth" {
		t.Fatalf("unsafe or irrelevant candidates: %+v", got)
	}
}

func TestRelevanceMatchingDoesNotConfuseAPIWithScraping(t *testing.T) {
	if termScore("advanced scraping guide", []string{"api"}) != 0 {
		t.Fatal("API should not match the middle of the word scraping")
	}
	links := relatedSourceLinks("[Advanced scraping](/advanced-scraping-guide) [API endpoint reference](/api-reference/endpoint/scrape)", "https://docs.example.com/features/alexandria", "Alexandria API endpoints", 3)
	if len(links) != 1 || !strings.Contains(links[0].url, "api-reference") {
		t.Fatalf("irrelevant docs were followed: %+v", links)
	}
}

func TestSearchPrefersTechnicalReferenceOverCommunityPages(t *testing.T) {
	results := []SearchDoc{
		{URL: "https://modelcontextprotocol.io/community/interest-groups/tool-annotations", Title: "Tool annotations interest group"},
		{URL: "https://modelcontextprotocol.io/specification/2025-11-25/server/tools", Title: "Tools specification"},
		{URL: "https://github.com/modelcontextprotocol/go-sdk", Title: "Go SDK for MCP"},
		{URL: "https://pkg.go.dev/golang.org/x/tools/internal/mcp", Title: "Internal Go MCP"},
	}
	selected := chooseSearchSources("Go MCP SDK tool annotations", results, 2)
	if len(selected) != 2 || selected[0].url != "https://github.com/modelcontextprotocol/go-sdk" || !strings.Contains(selected[1].url, "/specification/") {
		t.Fatalf("selected irrelevant pages over official SDK/spec: %+v", selected)
	}
}

func TestFollowLinksSkipGenericMCPLinksForSpecificQuestion(t *testing.T) {
	content := "[Skills over MCP](/community/working-groups/skills-over-mcp) [Tool annotations](/specification/server/tools)"
	links := relatedSourceLinks(content, "https://modelcontextprotocol.io/docs/tools", "Go MCP SDK tool annotations", 2)
	if len(links) != 1 || !strings.Contains(links[0].url, "/specification/server/tools") {
		t.Fatalf("followed generic cross-topic link: %+v", links)
	}
}

func TestSelectJSONFieldsPreservesSyntaxAndPaths(t *testing.T) {
	obj := map[string]any{"paths": map[string]any{"/v2/scrape": map[string]any{"post": map[string]any{"summary": "Scrape an API document"}}}, "metadata": strings.Repeat("x", 5000)}
	got := selectedJSONFields(obj, "scrape", 1400)
	if !strings.Contains(got, "/paths/~1v2~1scrape") || !strings.Contains(got, "```json") || !strings.Contains(got, "Scrape an API document") {
		t.Fatalf("JSON selection missing source pointers: %s", got)
	}
}

func TestLargeOpenAPISchemaUsesSourceJSONPointers(t *testing.T) {
	source := `**URL:** https://docs.example.com/openapi.json` + "\n\n```json\n" + `{"paths":{"/v2/scrape":{"post":{"summary":"Scrape pages"}}},"large":"` + strings.Repeat("x", 12000) + `"}` + "\n```"
	got := focusExcerpt(source, "scrape endpoint", 2500)
	if !strings.Contains(got, "/paths/~1v2~1scrape") || !strings.Contains(got, "Scrape pages") || len(got) > 2700 {
		t.Fatalf("large raw JSON should be selected by pointer: %s", got)
	}
}
