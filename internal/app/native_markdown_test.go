package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestCanonicalStripeGuidePrefersMarkdownWithZeroHeadLength(t *testing.T) {
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	calls := []string{}
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		if req.Method == http.MethodHead {
			t.Fatal("native Markdown detection must not require HEAD metadata")
		}
		if req.Method == http.MethodGet && req.URL.Path == "/payments/checkout.md" {
			r := testHTTPResponse(req, http.StatusOK, "# Build a payments page\n\nStripe-hosted checkout is supported.", map[string]string{"Content-Type": "text/markdown"})
			r.ContentLength = 0 // Publishers can return zero or unknown length metadata.
			return r, nil
		}
		t.Fatalf("unexpected extra request: %s %s", req.Method, req.URL)
		return nil, nil
	})}
	out, err := ReadLink("https://docs.stripe.com/payments/checkout")
	if err != nil || !strings.Contains(out, "**Markdown source:** https://docs.stripe.com/payments/checkout.md") {
		t.Fatalf("native source not selected: %v %s", err, out)
	}
	if len(calls) != 1 {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestCanonicalCloudflareDirectoryReadsIndexMarkdown(t *testing.T) {
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/durable-objects/index.md" {
			t.Fatalf("unexpected URL: %s %s", req.Method, req.URL)
		}
		return testHTTPResponse(req, http.StatusOK,
			"# Cloudflare Durable Objects\n\n[Get started](https://developers.cloudflare.com/durable-objects/get-started/)",
			map[string]string{"Content-Type": "text/markdown"}), nil
	})}
	out, err := ReadLink("https://developers.cloudflare.com/durable-objects/")
	if err != nil || !strings.Contains(out, "**Markdown source:** https://developers.cloudflare.com/durable-objects/index.md") {
		t.Fatalf("directory-index native Markdown failed: %v %s", err, out)
	}
}

func TestCAPTCHADisguisedAsMarkdownIsRejected(t *testing.T) {
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	var scrapeCalled bool
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/guide.md" {
			return testHTTPResponse(req, http.StatusOK, "# hCaptcha\nPlease verify you are human. "+
				strings.Repeat("English French Deutsch Español Chinese 日本語 ", 200), map[string]string{"Content-Type": "text/markdown"}), nil
		}
		if req.URL.Path == "/guide/index.md" {
			return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
		}
		if req.Method == http.MethodPost && req.URL.Path == "/v2/scrape" {
			scrapeCalled = true
			return testHTTPResponse(req, http.StatusOK,
				`{"success":true,"data":{"markdown":"# Legitimate guide\n\nFull instructions for a public API."}}`,
				nil), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})}
	t.Setenv("FIRECRAWL_API_KEY", "test")
	out, err := ReadLink("https://docs.example.com/guide")
	if err != nil || !scrapeCalled || strings.Contains(out, "Deutsch Español") || !strings.Contains(out, "Full instructions") {
		t.Fatalf("CAPTCHA disguised as Markdown leaked or fallback failed: %v %s", err, out)
	}
}
