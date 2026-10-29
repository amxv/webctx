package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExaUsesHighlightsAndSupportedFreshnessSettings(t *testing.T) {
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("EXA_API_KEY", "test-exa")
	t.Setenv("EXA_API_KEYS", "")
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var request map[string]any
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.exa.ai" {
			t.Errorf("unexpected Exa request: %s", req.URL)
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode Exa request: %v", err)
		}
		return testHTTPResponse(req, http.StatusOK, `{"results":[{"url":"https://example.com/guide","title":"Guide","highlights":["One relevant passage.","Another useful passage."],"text":"full page text","summary":"brief summary"},{"url":"https://example.com/other","title":"Other","summary":"summary fallback"}]}`, nil), nil
	})
	result, err := searchWithExa(context.Background(), "Go concurrency", []string{"excluded.example"}, "exact phrase")
	if err != nil || len(result.Docs) != 2 {
		t.Fatalf("Exa search failed: %+v, %v", result, err)
	}
	if got := result.Docs[0].Overview; got != "One relevant passage.\n\nAnother useful passage." {
		t.Errorf("highlights not used as description: %q", got)
	}
	if result.Docs[1].Overview != "summary fallback" {
		t.Errorf("missing fallback description: %+v", result.Docs[1])
	}
	if request["query"] != `Go concurrency "exact phrase"` || request["type"] != "auto" || request["numResults"] != float64(25) {
		t.Errorf("Exa search request changed unexpectedly: %#v", request)
	}
	if _, found := request["includeText"]; found {
		t.Error("deprecated Exa includeText was sent")
	}
	if got := request["excludeDomains"]; len(got.([]any)) != 1 {
		t.Errorf("excludeDomains lost: %#v", got)
	}
	contents := request["contents"].(map[string]any)
	if contents["highlights"] != true {
		t.Errorf("expected recommended relevance highlights: %#v", contents)
	}
	if _, found := contents["maxAgeHours"]; found {
		t.Errorf("default search must not force slow livecrawls of every Exa result: %#v", contents)
	}
	if _, found := contents["livecrawl"]; found {
		t.Error("deprecated Exa livecrawl was sent")
	}
}

func TestTavilyExactMatchOnlyForPhraseSearch(t *testing.T) {
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("TAVILY_API_KEY", "test-tavily")
	t.Setenv("TAVILY_API_KEYS", "")
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var requests []map[string]any
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.tavily.com" {
			t.Errorf("unexpected Tavily request: %s", req.URL)
		}
		raw, _ := io.ReadAll(req.Body)
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("decode Tavily request: %v", err)
		}
		requests = append(requests, payload)
		return testHTTPResponse(req, http.StatusOK, `{"results":[{"url":"https://example.com","title":"Example","content":"snippet"}]}`, nil), nil
	})
	_, err := searchWithTavily(context.Background(), "general docs", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = searchWithTavily(context.Background(), `"exact phrase" docs`, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := requests[0]["exact_match"]; exists {
		t.Errorf("normal searches should not enforce exact phrase: %#v", requests[0])
	}
	if requests[1]["exact_match"] != true {
		t.Errorf("phrase fallback should use exact_match: %#v", requests[1])
	}
	if requests[0]["search_depth"] != "advanced" || requests[1]["search_depth"] != "advanced" {
		t.Error("Tavily should preserve quality-oriented Advanced search depth")
	}
}

func TestRankingKeepsExaHighlightsForDuplicateURL(t *testing.T) {
	docs, removed := scoreAndRankResults([]providerDocs{
		{Provider: "Brave", Docs: []SearchDoc{{URL: "https://example.com/guide", Title: "Official guide", Overview: "Short search engine description."}}},
		{Provider: "Exa", Docs: []SearchDoc{{URL: "https://example.com/guide?utm_source=search", Title: "Extracted guide", Overview: "Query-specific source passage from Exa highlights."}}},
	})
	if len(docs) != 1 || removed != 1 {
		t.Fatalf("duplicate-aware ranking changed: docs=%+v removed=%d", docs, removed)
	}
	if docs[0].Title != "Official guide" {
		t.Errorf("the best-ranked title was lost: %+v", docs[0])
	}
	if docs[0].Overview != "Query-specific source passage from Exa highlights." {
		t.Errorf("duplicate merger discarded the more useful Exa source excerpt: %+v", docs[0])
	}
}

func TestSearchDoesNotCallStandaloneAlexandria(t *testing.T) {
	searchTestEnv(t)
	mockSearchProviders(t, false)
	t.Setenv("FIRECRAWL_API_KEY", "test-firecrawl")
	previous := http.DefaultClient.Transport
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.firecrawl.dev" && strings.HasPrefix(req.URL.Path, "/v2/search") {
			t.Errorf("regular web search issued a redundant Alexandria discovery request: %s", req.URL)
		}
		return previous.RoundTrip(req)
	})
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	output, err := Search(SearchParams{Query: "relevant documentation"})
	if err != nil || !strings.Contains(output, "## Search results") {
		t.Fatalf("normal search failed: %v, %q", err, output)
	}
	if strings.Contains(output, "## Related data providers") {
		t.Error("web search should not include Alexandria tool catalogue")
	}
}

func TestRegularSearchNeverReadsOrScrapesResultPages(t *testing.T) {
	searchTestEnv(t)
	t.Setenv("FIRECRAWL_API_KEY", "test-firecrawl")
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var pageReads int
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.search.brave.com":
			return testHTTPResponse(req, http.StatusOK, `{"web":{"results":[{"url":"https://docs.example.com/guide","title":"Official Guide","description":"Useful source snippet"}]}}`, nil), nil
		case "api.tavily.com", "api.exa.ai":
			return testHTTPResponse(req, http.StatusOK, `{"results":[]}`, nil), nil
		default:
			pageReads++
			return testHTTPResponse(req, http.StatusForbidden, "unexpected page read", nil), nil
		}
	})
	output, err := Search(SearchParams{Query: "example API documentation"})
	if err != nil || !strings.Contains(output, "Useful source snippet") || pageReads != 0 {
		t.Fatalf("search performed unwanted enrichment: reads=%d error=%v output=%q", pageReads, err, output)
	}
	if strings.Contains(output, "Retrieved source context") || strings.Contains(output, "Supporting reference") {
		t.Fatalf("search still emitted auto-enrichment sections: %s", output)
	}
}

func TestSearchEnrichmentScrapeSkipsAlexandriaButReadLinkKeepsIt(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "test-firecrawl")
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var requestedTools []bool
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.firecrawl.dev" || req.URL.Path != "/v2/scrape" {
			t.Errorf("unexpected scrape request: %s", req.URL)
		}
		raw, _ := io.ReadAll(req.Body)
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("decode scrape payload: %v", err)
		}
		requestedTools = append(requestedTools, payload["domainTools"].(bool))
		if payload["maxAge"] != float64(1800000) {
			t.Errorf("search must preserve Firecrawl's 30-minute cache: %#v", payload)
		}
		return testHTTPResponse(req, http.StatusOK, `{"success":true,"data":{"metadata":{"title":"Guide"},"markdown":"# Guide\n\nDocumented feature set for this technology."}}`, nil), nil
	})

	rawURL := "https://docs.example.com/reference.html"
	searchText, err := readLinkWithDomainTools(rawURL, false)
	if err != nil || !strings.Contains(searchText, "Documented feature set") {
		t.Fatalf("search enrichment failed: %v, %q", err, searchText)
	}
	readText, err := ReadLink(rawURL)
	if err != nil || !strings.Contains(readText, "Documented feature set") {
		t.Fatalf("explicit read-link failed: %v, %q", err, readText)
	}
	if len(requestedTools) != 2 || requestedTools[0] || !requestedTools[1] {
		t.Errorf("search should not discover Alexandria, but read-link should preserve existing behavior: %v", requestedTools)
	}
}
