package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestProviderCommandsPersistAndReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webctx", "config.json")
	t.Setenv("WEBCTX_CONFIG_FILE", path)
	t.Setenv("BRAVE_API_KEY", "test-brave-secret")
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("EXA_API_KEY", "test-exa-secret")
	t.Setenv("FIRECRAWL_API_KEY", "")

	run := func(args ...string) (int, string, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := Run(args, &out, &errOut)
		return code, out.String(), errOut.String()
	}
	if code, text, errText := run("status"); code != 0 || !strings.Contains(text, "exa     enabled") || !strings.Contains(text, "brave   enabled") || errText != "" {
		t.Fatalf("default status: code=%d text=%q error=%q", code, text, errText)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("status should not create config file, got error %v", err)
	}
	if code, text, errText := run("providers", "disable", "ExA"); code != 0 || !strings.Contains(text, "exa disabled") || errText != "" {
		t.Fatalf("disable: code=%d text=%q error=%q", code, text, errText)
	}
	if code, text, _ := run("providers"); code != 0 || !strings.Contains(text, "exa     disabled") || !strings.Contains(text, "key configured") {
		t.Fatalf("persisted status: code=%d text=%q", code, text)
	} else if strings.Contains(text, "test-exa-secret") || strings.Contains(text, "test-brave-secret") {
		t.Fatalf("status exposed a credential: %q", text)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config providerConfig
	if err := json.Unmarshal(content, &config); err != nil || !strings.EqualFold(strings.Join(config.DisabledProviders, ","), "exa") {
		t.Fatalf("settings not persisted: %s; error=%v", content, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions: info=%v error=%v", info, err)
	}
	if code, text, errText := run("providers", "enable", "exa"); code != 0 || !strings.Contains(text, "exa enabled") || errText != "" {
		t.Fatalf("enable: code=%d text=%q error=%q", code, text, errText)
	}
	if code, text, _ := run("status"); code != 0 || !strings.Contains(text, "exa     enabled") {
		t.Fatalf("restored status: code=%d text=%q", code, text)
	}
}

func TestProviderCommandsRejectInvalidInputAndMalformedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("WEBCTX_CONFIG_FILE", path)
	for _, args := range [][]string{{"providers", "disable", "firecrawl"}, {"providers", "disable", "unknown"}, {"providers", "disable"}, {"status", "enable", "exa"}} {
		var out, errOut bytes.Buffer
		if Run(args, &out, &errOut) == 0 || errOut.Len() == 0 {
			t.Fatalf("invalid command %q succeeded: %q", args, out.String())
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid command wrote config: %v", err)
	}
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if Run([]string{"status"}, &out, &errOut) == 0 || !strings.Contains(errOut.String(), "invalid provider settings") {
		t.Fatalf("malformed config not reported: %q", errOut.String())
	}
	if Run([]string{"providers", "disable", "exa"}, &out, &errOut) == 0 {
		t.Fatal("malformed config should not be overwritten")
	}
}

type providerRequests struct {
	mu        sync.Mutex
	queries   map[string][]string
	failedExa bool
	emptyExa  bool
}

func mockSearchProviders(t *testing.T, exaFailure bool) *providerRequests {
	t.Helper()
	prev := http.DefaultClient.Transport
	stats := &providerRequests{queries: make(map[string][]string), failedExa: exaFailure}
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		provider, query := "", ""
		switch req.URL.Host {
		case "api.search.brave.com":
			provider, query = "brave", req.URL.Query().Get("q")
		case "api.tavily.com", "api.exa.ai":
			if req.URL.Host == "api.tavily.com" {
				provider = "tavily"
			} else {
				provider = "exa"
			}
			data, _ := io.ReadAll(req.Body)
			var payload map[string]any
			_ = json.Unmarshal(data, &payload)
			query, _ = payload["query"].(string)
		case "provider-test.example":
			return testHTTPResponse(req, http.StatusOK, "# Example documentation\n\nThis markdown page contains enough content for a test.", map[string]string{"Content-Type": "text/markdown"}), nil
		default:
			return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
		}
		stats.mu.Lock()
		stats.queries[provider] = append(stats.queries[provider], query)
		stats.mu.Unlock()
		switch provider {
		case "brave":
			return testHTTPResponse(req, http.StatusOK, `{"web":{"results":[{"url":"https://provider-test.example/guide.md","title":"Guide","description":"Documentation"}]}}`, nil), nil
		case "tavily":
			return testHTTPResponse(req, http.StatusOK, `{"results":[]}`, nil), nil
		case "exa":
			if stats.failedExa {
				return testHTTPResponse(req, http.StatusPaymentRequired, `{"error":"NO_MORE_CREDITS"}`, nil), nil
			}
			if stats.emptyExa {
				return testHTTPResponse(req, http.StatusOK, `{"results":[]}`, nil), nil
			}
			return testHTTPResponse(req, http.StatusOK, `{"results":[{"url":"https://provider-test.example/guide.md","title":"Guide","text":"Documentation"}]}`, nil), nil
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = prev })
	return stats
}

func (p *providerRequests) received(provider string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.queries[provider]...)
}

func searchTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEBCTX_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("BRAVE_API_KEY", "test-brave")
	t.Setenv("TAVILY_API_KEY", "test-tavily")
	t.Setenv("EXA_API_KEY", "test-exa")
	t.Setenv("BRAVE_API_KEYS", "")
	t.Setenv("TAVILY_API_KEYS", "")
	t.Setenv("EXA_API_KEYS", "")
	t.Setenv("FIRECRAWL_API_KEY", "")
}

func TestSearchDoesNotCallDisabledExa(t *testing.T) {
	searchTestEnv(t)
	stats := mockSearchProviders(t, false)
	if _, err := setSearchProviderEnabled("exa", false); err != nil {
		t.Fatal(err)
	}
	result, err := Search(SearchParams{Query: "runtime docs"})
	if err != nil || !strings.Contains(result, "Total Results: 1") {
		t.Fatalf("search without Exa failed: %v; %q", err, result)
	}
	if got := stats.received("exa"); len(got) != 0 {
		t.Fatalf("Exa was contacted despite being disabled: %v", got)
	}
	if got := stats.received("brave"); len(got) != 1 || got[0] != "runtime docs" {
		t.Fatalf("Brave was not used: %v", got)
	}
	if got := stats.received("tavily"); len(got) != 1 {
		t.Fatalf("Tavily was not used: %v", got)
	}
}

func TestSearchSupportsOnlyBraveEnabled(t *testing.T) {
	searchTestEnv(t)
	stats := mockSearchProviders(t, false)
	for _, name := range []string{"exa", "tavily"} {
		if _, err := setSearchProviderEnabled(name, false); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Search(SearchParams{Query: "runtime docs"})
	if err != nil || !strings.Contains(result, "Total Results: 1") {
		t.Fatalf("single-provider search failed: %v; %q", err, result)
	}
	if len(stats.received("exa")) != 0 || len(stats.received("tavily")) != 0 || len(stats.received("brave")) != 1 {
		t.Fatalf("unexpected requests: %#v", stats.queries)
	}
}

func TestKeywordFallbackSkipsDisabledExa(t *testing.T) {
	searchTestEnv(t)
	stats := mockSearchProviders(t, false)
	if _, err := setSearchProviderEnabled("exa", false); err != nil {
		t.Fatal(err)
	}
	result, err := Search(SearchParams{Query: "drizzle orm", IncludeKeyword: "migration guide"})
	if err != nil || !strings.Contains(result, "Keyword fallback") {
		t.Fatalf("keyword fallback failed: %v; %q", err, result)
	}
	if len(stats.received("exa")) != 0 {
		t.Fatal("disabled Exa received keyword request")
	}
	for _, provider := range []string{"brave", "tavily"} {
		got := stats.received(provider)
		if len(got) != 1 || !strings.Contains(got[0], `"migration guide"`) {
			t.Fatalf("%s did not receive a quoted phrase: %v", provider, got)
		}
	}
}

func TestKeywordFallbackOnExaCreditExhaustion(t *testing.T) {
	searchTestEnv(t)
	stats := mockSearchProviders(t, true)
	result, err := Search(SearchParams{Query: "drizzle orm", IncludeKeyword: "migration guide"})
	if err != nil || !strings.Contains(result, "Keyword fallback") || !strings.Contains(result, "Total Results: 1") {
		t.Fatalf("keyword fallback after Exa error failed: %v; %q", err, result)
	}
	if len(stats.received("exa")) != 1 || len(stats.received("brave")) != 1 {
		t.Fatalf("unexpected provider request sequence: %#v", stats.queries)
	}
}

func TestKeywordExaSuccessDoesNotContactOtherProviders(t *testing.T) {
	searchTestEnv(t)
	stats := mockSearchProviders(t, false)
	result, err := Search(SearchParams{Query: "drizzle orm", IncludeKeyword: "migration guide"})
	if err != nil || strings.Contains(result, "Keyword fallback") || !strings.Contains(result, "Total Results: 1") {
		t.Fatalf("unexpected keyword results: %v; %q", err, result)
	}
	if len(stats.received("exa")) != 1 || len(stats.received("brave")) != 0 || len(stats.received("tavily")) != 0 {
		t.Fatalf("unexpected provider requests: %#v", stats.queries)
	}
}

func TestKeywordExaNoMatchesDoesNotBroadenSearch(t *testing.T) {
	searchTestEnv(t)
	stats := mockSearchProviders(t, false)
	stats.emptyExa = true
	_, err := Search(SearchParams{Query: "drizzle orm", IncludeKeyword: "migration guide"})
	if err == nil || !strings.Contains(err.Error(), "failed to return results") {
		t.Fatalf("unexpected result for zero Exa matches: %v", err)
	}
	if len(stats.received("exa")) != 1 || len(stats.received("brave")) != 0 || len(stats.received("tavily")) != 0 {
		t.Fatalf("strict keyword search was broadened: %#v", stats.queries)
	}
}

func TestSearchWithAllProvidersDisabledFailsFast(t *testing.T) {
	searchTestEnv(t)
	stats := mockSearchProviders(t, false)
	for _, name := range []string{"brave", "tavily", "exa"} {
		if _, err := setSearchProviderEnabled(name, false); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Search(SearchParams{Query: "anything"})
	if err == nil || !strings.Contains(err.Error(), "all search providers are disabled") {
		t.Fatalf("unexpected error when all disabled: %v", err)
	}
	if len(stats.received("exa"))+len(stats.received("tavily"))+len(stats.received("brave")) != 0 {
		t.Fatal("search contacted provider with all disabled")
	}
}

func TestBraveSearchTransparentlyDecodesGzip(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "test-brave")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			t.Errorf("expected Go HTTP transport to request gzip, got %q", r.Header.Get("Accept-Encoding"))
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		writer := gzip.NewWriter(w)
		_, _ = writer.Write([]byte(`{"web":{"results":[{"url":"https://example.com/guide","title":"Guide","description":"Documentation"}]}}`))
		_ = writer.Close()
	}))
	defer server.Close()

	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Accept-Encoding") != "" {
			t.Errorf("Brave search forced Accept-Encoding instead of letting Go decode it: %q", req.Header.Get("Accept-Encoding"))
		}
		copy := req.Clone(req.Context())
		copy.URL.Scheme = "http"
		copy.URL.Host = strings.TrimPrefix(server.URL, "http://")
		copy.Host = copy.URL.Host
		return http.DefaultTransport.RoundTrip(copy)
	})
	t.Cleanup(func() { http.DefaultClient.Transport = originalTransport })

	result, err := searchWithBrave(context.Background(), "Go docs")
	if err != nil || len(result.Docs) != 1 || result.Docs[0].Title != "Guide" {
		t.Fatalf("Brave gzip response not decoded: %v; %#v", err, result)
	}
}
