package app

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDirectOpenAPIJSONBypassesScraping(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "docs.example.com" || req.URL.Path != "/openapi.json" || req.Method != http.MethodGet {
			t.Fatalf("unexpected direct source request: %s", req.URL)
		}
		return testHTTPResponse(req, http.StatusOK, `{"openapi":"3.1.0","paths":{"/v2/scrape":{}}}`, nil), nil
	})}
	got, err := ReadLink("https://docs.example.com/openapi.json")
	if err != nil || !strings.Contains(got, `"openapi":"3.1.0"`) || !strings.Contains(got, "```json") {
		t.Fatalf("did not fetch raw OpenAPI: %q, %v", got, err)
	}
}

func TestDirectSourceRejectsPrivateRedirects(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "docs.example.com" {
			t.Fatalf("followed unsafe redirect to %s", req.URL)
		}
		return testHTTPResponse(req, http.StatusFound, "", map[string]string{"Location": "http://127.0.0.1/private"}), nil
	})}
	if _, err := fetchPublicText(t.Context(), "https://docs.example.com/openapi.json", 100); err == nil {
		t.Fatal("should reject public-to-private redirects")
	}
}

func TestAlexandriaDiscoveryFallsBackWhenNotEnabled(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	var calls int
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		calls++
		if calls == 1 {
			if !strings.Contains(string(body), `"domainTools":true`) {
				t.Fatalf("Alexandria discovery not enabled: %s", body)
			}
			return testHTTPResponse(req, http.StatusForbidden, `{"error":"Alexandria unavailable"}`, nil), nil
		}
		if !strings.Contains(string(body), `"domainTools":false`) {
			t.Fatalf("fallback did not disable domain matching: %s", body)
		}
		return testHTTPResponse(req, http.StatusOK, `{"success":true,"data":{"markdown":"# Good source\n\nAuthoritative page content."}}`, nil), nil
	})}
	result, err := firecrawlScrape("https://docs.example.com/reference", "fake-test-key", "")
	if err != nil || calls != 2 || !strings.Contains(result.Markdown, "Good source") {
		t.Fatalf("fallback broke regular scraping: %#v, %v, calls=%d", result, err, calls)
	}
}

func TestAlexandriaToolDescriptionIsDiscoveryOnly(t *testing.T) {
	got := formatAlexandriaTools([]any{
		map[string]any{"provider": "particle", "capability": "podcasts/episodes/search", "description": "Find episodes"},
	})
	if !strings.Contains(got, "particle/podcasts/episodes/search") || !strings.Contains(got, "not executed") {
		t.Fatalf("bad tool discovery metadata: %s", got)
	}
}
