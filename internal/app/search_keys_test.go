package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSearchKeyOrderDeduplicationAndMissingKeys(t *testing.T) {
	t.Setenv("EXA_API_KEYS", ` ["new","new","old"] `)
	t.Setenv("EXA_API_KEY", "old")
	if got, want := searchKeysForProvider("exa"), []string{"new", "old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deduplication: got %v, want %v", got, want)
	}
	t.Setenv("EXA_API_KEYS", "new,another\n old\n")
	if got, want := searchKeysForProvider("exa"), []string{"new", "another", "old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CSV/newline parsing: got %v, want %v", got, want)
	}
	t.Setenv("EXA_API_KEYS", `[invalid`)
	t.Setenv("EXA_API_KEY", "")
	if got := searchKeysForProvider("exa"); len(got) != 0 {
		t.Fatalf("malformed JSON should not be used as credential: %v", got)
	}
	if got := searchKeysForProvider("unknown"); len(got) != 0 {
		t.Fatalf("unknown provider should have no keys: %v", got)
	}
}

func TestStatusReportsMultipleKeysWithoutRevealingThem(t *testing.T) {
	t.Setenv("WEBCTX_CONFIG_FILE", filepath.Join(t.TempDir(), "webctx", "config.json"))
	t.Setenv("EXA_API_KEYS", "extra-secret-a,extra-secret-b")
	t.Setenv("EXA_API_KEY", "legacy-secret")
	t.Setenv("BRAVE_API_KEY", "")
	t.Setenv("BRAVE_API_KEYS", "")
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("TAVILY_API_KEYS", "")
	t.Setenv("FIRECRAWL_API_KEY", "")
	var out, errOut bytes.Buffer
	if code := Run([]string{"status"}, &out, &errOut); code != 0 {
		t.Fatalf("status error: %s", errOut.String())
	}
	if !strings.Contains(out.String(), "exa     enabled   key configured (3 keys)") {
		t.Fatalf("incorrect key count: %q", out.String())
	}
	for _, secret := range []string{"extra-secret-a", "extra-secret-b", "legacy-secret"} {
		if strings.Contains(out.String()+errOut.String(), secret) {
			t.Fatal("status exposed a configured API key")
		}
	}
}

func TestManagedSearchKeychainAddRemoveAndInputValidation(t *testing.T) {
	oldLookup, oldStore := keychainLookup, keychainStore
	t.Cleanup(func() { keychainLookup, keychainStore = oldLookup, oldStore })
	managed := map[string]string{}
	keychainLookup = func(account string) (string, error) { return managed[account], nil }
	keychainStore = func(account, secret string) error { managed[account] = secret; return nil }

	for _, candidate := range []string{"", "one two", "line1\nline2", strings.Repeat("z", 4097)} {
		if _, err := addManagedSearchKey("exa", strings.NewReader(candidate)); err == nil {
			t.Fatalf("invalid secret accepted")
		}
	}
	if len(managed) != 0 {
		t.Fatal("invalid keys must not modify Keychain")
	}
	if _, err := addManagedSearchKey("unknown", strings.NewReader("token")); err == nil {
		t.Fatal("unknown provider accepted")
	}
	for _, pair := range []struct {
		key   string
		count int
	}{{"older", 1}, {"newer", 2}, {"newer", 2}} {
		count, err := addManagedSearchKey("exa", strings.NewReader(pair.key+"\n"))
		if err != nil || count != pair.count {
			t.Fatalf("add: count=%d error=%v", count, err)
		}
	}
	list, err := readManagedKeys("exa")
	if err != nil || !reflect.DeepEqual(list, []string{"newer", "older"}) {
		t.Fatalf("priority order changed: %v %v", list, err)
	}
	if strings.Contains(managed["EXA_API_KEYS"], "[REDACTED]") {
		t.Fatal("managed keys stored as placeholder instead of actual tokens")
	}
	if _, err := removeManagedSearchKey("exa", "3"); err == nil {
		t.Fatal("out-of-range removal accepted")
	}
	if count, err := removeManagedSearchKey("exa", "1"); err != nil || count != 1 {
		t.Fatalf("remove: count=%d error=%v", count, err)
	}
	if list, err := readManagedKeys("exa"); err != nil || !reflect.DeepEqual(list, []string{"older"}) {
		t.Fatalf("wrong managed key removed: %v %v", list, err)
	}
	managed["EXA_API_KEYS"] = "{invalid"
	if _, err := addManagedSearchKey("exa", strings.NewReader("another")); err == nil {
		t.Fatal("malformed Keychain JSON silently overwritten")
	}
}

func TestManagedSearchKeyCommandNeverPrintsSecrets(t *testing.T) {
	oldLookup, oldStore, oldStdin := keychainLookup, keychainStore, os.Stdin
	t.Cleanup(func() { keychainLookup, keychainStore, os.Stdin = oldLookup, oldStore, oldStdin })
	keychainLookup = func(account string) (string, error) { return "", nil }
	keychainStore = func(account, value string) error { return nil }
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "sensitive-api-token\n"); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	os.Stdin = input
	var out, errOut bytes.Buffer
	code := Run([]string{"keys", "add", "exa", "--stdin"}, &out, &errOut)
	_ = input.Close()
	if code != 0 || strings.Contains(out.String()+errOut.String(), "sensitive-api-token") {
		t.Fatalf("unsafe CLI output or command failed: code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

// Exercise the real HTTP request construction and key-selection order for all
// three providers, without making network requests or disclosing API keys.
func TestSearchProviderFallbackAcrossKeys(t *testing.T) {
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("BRAVE_API_KEY", "legacy-brave")
	t.Setenv("TAVILY_API_KEY", "legacy-tavily")
	t.Setenv("EXA_API_KEY", "legacy-exa")
	for _, name := range []string{"BRAVE", "TAVILY", "EXA"} {
		t.Setenv(name+"_API_KEYS", "bad-"+strings.ToLower(name)+",good-"+strings.ToLower(name))
	}
	previous := http.DefaultClient.Transport
	requests := map[string][]string{}
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		provider, key := "", ""
		switch req.URL.Host {
		case "api.search.brave.com":
			provider, key = "brave", req.Header.Get("x-subscription-token")
		case "api.exa.ai":
			provider, key = "exa", req.Header.Get("x-api-key")
		case "api.tavily.com":
			provider = "tavily"
			var payload map[string]any
			_ = json.NewDecoder(req.Body).Decode(&payload)
			key, _ = payload["api_key"].(string)
		default:
			t.Errorf("unexpected host: %s", req.URL.Host)
		}
		requests[provider] = append(requests[provider], key)
		if strings.HasPrefix(key, "bad-") {
			code := http.StatusUnauthorized
			switch provider {
			case "brave":
				return testHTTPResponse(req, 422, `{"error":{"code":"SUBSCRIPTION_TOKEN_INVALID"}}`, nil), nil
			case "tavily":
				code = 432
			case "exa":
				code = http.StatusPaymentRequired
			}
			return testHTTPResponse(req, code, `{"error":"credits exhausted"}`, nil), nil
		}
		if strings.HasPrefix(key, "good-") {
			switch provider {
			case "brave":
				return testHTTPResponse(req, 200, `{"web":{"results":[{"url":"https://example.com/guide","title":"Brave result"}]}}`, nil), nil
			case "tavily", "exa":
				return testHTTPResponse(req, 200, `{"results":[{"url":"https://example.com/guide","title":"API result"}]}`, nil), nil
			}
		}
		return testHTTPResponse(req, 500, "unexpected key", nil), nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = previous })

	for _, test := range []struct {
		provider string
		call     func() (SearchResult, error)
	}{
		{"brave", func() (SearchResult, error) { return searchWithBrave(context.Background(), "test query") }},
		{"tavily", func() (SearchResult, error) { return searchWithTavily(context.Background(), "test query", nil, false) }},
		{"exa", func() (SearchResult, error) {
			return searchWithExa(context.Background(), "test query", nil, "exact phrase")
		}},
	} {
		results, err := test.call()
		if err != nil || len(results.Docs) != 1 {
			t.Fatalf("%s: search failed after key rotation: %v %+v", test.provider, err, results)
		}
		want := []string{"bad-" + test.provider, "good-" + test.provider}
		if got := requests[test.provider]; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: key rotation order got %v, want %v", test.provider, got, want)
		}
	}
}

func TestSearchKeyRetryDoesNotMaskBadInputOrLeakKeys(t *testing.T) {
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("EXA_API_KEYS", "first-secret,second-secret")
	t.Setenv("EXA_API_KEY", "")
	previous := http.DefaultClient.Transport
	count, code := 0, http.StatusBadRequest
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		count++
		return testHTTPResponse(req, code, fmt.Sprintf(`{"error":"key %s failed"}`, req.Header.Get("x-api-key")), nil), nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	_, err := searchWithExa(context.Background(), "test", nil, "")
	if err == nil || count != 1 || strings.Contains(err.Error(), "first-secret") {
		t.Fatalf("400 should not retry or leak a credential: count=%d error=%v", count, err)
	}
	count, code = 0, http.StatusUnprocessableEntity
	_, err = searchWithExa(context.Background(), "test", nil, "")
	if err == nil || count != 1 {
		t.Fatalf("non-credential 422 should not retry: count=%d error=%v", count, err)
	}
	count, code = 0, http.StatusTooManyRequests
	_, err = searchWithExa(context.Background(), "test", nil, "")
	if err == nil || count != 2 || strings.Contains(err.Error(), "first-secret") || strings.Contains(err.Error(), "second-secret") || !strings.Contains(err.Error(), "2 configured") {
		t.Fatalf("exhausted-key error is unsafe or not actionable: count=%d error=%v", count, err)
	}
}
