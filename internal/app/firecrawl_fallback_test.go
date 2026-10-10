package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSourceURLSafety(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "http://localhost:9999", "http://127.0.0.1:80", "http://[::1]", "http://user:pass@example.com", "http://192.168.0.1", "http://169.254.169.254"} {
		if validateSourceURL(raw) == nil {
			t.Fatalf("allowed unsafe URL %q", raw)
		}
	}
	if err := validateSourceURL("https://example.com/guide"); err != nil {
		t.Fatalf("rejected public URL: %v", err)
	}
}

func TestLongBrowserCAPTCHAFallbackFailsClearly(t *testing.T) {
	original := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = original })
	challenge := "hCaptcha Please verify you are human. " +
		strings.Repeat("French Deutsch Español English 日本語 中文 ", 150)
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method + " " + req.URL.Path {
		case "POST /v2/scrape":
			content, _ := json.Marshal(map[string]any{
				"success": true,
				"data":    map[string]any{"markdown": challenge},
			})
			return testHTTPResponse(req, http.StatusOK, string(content), nil), nil
		case "POST /v2/interact":
			return testHTTPResponse(req, http.StatusOK, `{"success":true,"id":"test-captcha"}`, nil), nil
		case "POST /v2/interact/test-captcha/execute":
			body, _ := json.Marshal(map[string]any{
				"success": true, "stdout": challenge, "exitCode": 0,
			})
			return testHTTPResponse(req, http.StatusOK, string(body), nil), nil
		case "DELETE /v2/interact/test-captcha":
			return testHTTPResponse(req, http.StatusOK, `{"success":true}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
			return nil, nil
		}
	})}
	t.Setenv("FIRECRAWL_API_KEY", "unit-test")
	_, returned, err := scrapeLinkWithFirecrawl("https://docs.example.com/challenged")
	if err == nil || returned != "" || !strings.Contains(err.Error(), "challenge") {
		t.Fatalf("long browser challenge leaked into result or lacked clear failure: %q %v", returned, err)
	}
	if strings.Contains(err.Error(), "French Deutsch") {
		t.Fatalf("challenge payload should not be logged verbatim in errors: %s", err)
	}
}

func TestScrapeEscalatesToEnhancedOnBlockedBody(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	var proxies []string
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPost && req.URL.String() == "https://api.firecrawl.dev/v2/scrape" {
			body, _ := io.ReadAll(req.Body)
			if strings.Contains(string(body), `"proxy":"enhanced"`) {
				proxies = append(proxies, "enhanced")
				if !strings.Contains(string(body), `"maxAge":0`) {
					t.Errorf("enhanced retry must bypass blocked cached results: %s", body)
				}
				return testHTTPResponse(req, http.StatusOK, `{"success":true,"data":{"metadata":{"title":"Article"},"markdown":"# Article\n\nLonger useful body from enhanced proxy."}}`, nil), nil
			}
			proxies = append(proxies, "auto")
			if !strings.Contains(string(body), `"maxAge":1800000`) {
				t.Errorf("Firecrawl cache age not 30 minutes: %s", body)
			}
			return testHTTPResponse(req, http.StatusOK, `{"success":true,"data":{"markdown":"Just a moment... checking your browser"}}`, nil), nil
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	t.Setenv("FIRECRAWL_API_KEY", "test")
	_, markdown, err := scrapeLinkWithFirecrawl("https://example.com/article")
	if err != nil || !strings.Contains(markdown, "Longer useful body") {
		t.Fatalf("expected enhanced proxy content, got %q: %v", markdown, err)
	}
	if strings.Join(proxies, ",") != "auto,enhanced" {
		t.Fatalf("unexpected proxy stages %v", proxies)
	}
}

func TestScrapeEscalatesToBrowserWhenScrapeFails(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	var calls []string
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		switch req.Method + " " + req.URL.Path {
		case "POST /v2/scrape":
			return testHTTPResponse(req, http.StatusOK, `{"success":false,"error":"blocked"}`, nil), nil
		case "POST /v2/interact":
			return testHTTPResponse(req, http.StatusOK, `{"success":true,"id":"abc123"}`, nil), nil
		case "POST /v2/interact/abc123/execute":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), "agent-browser eval") || strings.Contains(string(body), "agent-browser scrape") {
				t.Errorf("missing browser extraction command: %s", body)
			}
			return testHTTPResponse(req, http.StatusOK, `{"success":true,"stdout":"\"Real page content from browser.\\nNew paragraph.\"","exitCode":0}`, nil), nil
		case "DELETE /v2/interact/abc123":
			return testHTTPResponse(req, http.StatusOK, `{}`, nil), nil
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	t.Setenv("FIRECRAWL_API_KEY", "test")
	_, markdown, err := scrapeLinkWithFirecrawl("https://example.com/dynamic")
	if err != nil || !strings.Contains(markdown, "Real page content") || !strings.Contains(markdown, "browser.\nNew paragraph.") {
		t.Fatalf("expected browser fallback content, got %q: %v", markdown, err)
	}
	if strings.Join(calls, ",") != "POST /v2/scrape,POST /v2/interact,POST /v2/interact/abc123/execute,DELETE /v2/interact/abc123" {
		t.Fatalf("unexpected browser lifecycle: %v", calls)
	}
}
