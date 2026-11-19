package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const usableArticle = "# Article\n\nLonger useful body returned by the Firecrawl scrape for this page."

// shrinkReadBudgets scales the CLI read budgets down so tests run in seconds.
func shrinkReadBudgets(t *testing.T, quick, wait, scrape, browserMin time.Duration) {
	t.Helper()
	saved := []time.Duration{readLinkQuickBudget, readLinkWaitBudget, readLinkWatchdogGrace,
		firecrawlQuickScrapeTimeout, firecrawlWaitScrapeTimeout, firecrawlMinScrapeTimeout,
		firecrawlResponseGrace, firecrawlBrowserMinBudget, firecrawlBrowserMaxBudget}
	t.Cleanup(func() {
		readLinkQuickBudget, readLinkWaitBudget, readLinkWatchdogGrace = saved[0], saved[1], saved[2]
		firecrawlQuickScrapeTimeout, firecrawlWaitScrapeTimeout, firecrawlMinScrapeTimeout = saved[3], saved[4], saved[5]
		firecrawlResponseGrace, firecrawlBrowserMinBudget, firecrawlBrowserMaxBudget = saved[6], saved[7], saved[8]
	})
	readLinkQuickBudget, readLinkWaitBudget, readLinkWatchdogGrace = quick, wait, 500*time.Millisecond
	firecrawlQuickScrapeTimeout, firecrawlWaitScrapeTimeout = scrape, scrape
	firecrawlMinScrapeTimeout, firecrawlResponseGrace = 100*time.Millisecond, 200*time.Millisecond
	firecrawlBrowserMinBudget, firecrawlBrowserMaxBudget = browserMin, wait
}

func stubFirecrawl(t *testing.T, handler func(req *http.Request, body map[string]any) *http.Response) {
	t.Helper()
	original := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = original })
	t.Setenv("FIRECRAWL_API_KEY", "unit-test-read-budget")
	t.Setenv("FIRECRAWL_API_KEYS", "")
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() != "api.firecrawl.dev" {
			// Native Markdown and direct-source probes miss instantly.
			return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
		}
		body := map[string]any{}
		if req.Body != nil {
			raw, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(raw, &body)
		}
		return handler(req, body), nil
	})}
}

func scrapeOK(req *http.Request, markdown string) *http.Response {
	content, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"markdown": markdown}})
	return testHTTPResponse(req, http.StatusOK, string(content), nil)
}

func TestQuickReadSendsFirecrawlTimeoutButLegacyDoesNot(t *testing.T) {
	var timeouts []any
	stubFirecrawl(t, func(req *http.Request, body map[string]any) *http.Response {
		timeouts = append(timeouts, body["timeout"])
		return scrapeOK(req, usableArticle)
	})
	if _, _, err := scrapeLinkWithFirecrawlWithin("https://example.com/quick", true, newReadBudget(false)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := scrapeLinkWithFirecrawl("https://example.com/legacy"); err != nil {
		t.Fatal(err)
	}
	quick, ok := timeouts[0].(float64)
	if !ok || quick < 20000 || quick > 25000 {
		t.Fatalf("quick read must pass Firecrawl a timeout that fits the 30s budget, got %v", timeouts[0])
	}
	if timeouts[1] != nil {
		t.Fatalf("legacy/Origo scrape must keep Firecrawl's default timeout, got %v", timeouts[1])
	}
}

func TestWaitReadOutlivesDefaultNetworkTimeout(t *testing.T) {
	var deadlineIn time.Duration
	var serverTimeout any
	stubFirecrawl(t, func(req *http.Request, body map[string]any) *http.Response {
		serverTimeout = body["timeout"]
		if deadline, ok := req.Context().Deadline(); ok {
			deadlineIn = time.Until(deadline)
		}
		return scrapeOK(req, usableArticle)
	})
	if _, _, err := scrapeLinkWithFirecrawlWithin("https://example.com/slow", true, newReadBudget(true)); err != nil {
		t.Fatal(err)
	}
	if serverTimeout != float64(120000) {
		t.Fatalf("--wait should give Firecrawl 120s, got %v", serverTimeout)
	}
	if deadlineIn < 3*time.Minute {
		t.Fatalf("--wait request was cut by the 32s default network timeout: deadline in %s", deadlineIn)
	}
}

func TestQuickReadServerTimeoutReturnsPendingWithoutBrowser(t *testing.T) {
	shrinkReadBudgets(t, 1500*time.Millisecond, 3*time.Second, time.Second, 500*time.Millisecond)
	var browser atomic.Bool
	stubFirecrawl(t, func(req *http.Request, body map[string]any) *http.Response {
		if strings.HasPrefix(req.URL.Path, "/v2/interact") {
			browser.Store(true)
			return testHTTPResponse(req, http.StatusInternalServerError, "", nil)
		}
		// Firecrawl reports its own timeout as a generic engine failure.
		ms, _ := body["timeout"].(float64)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return testHTTPResponse(req, http.StatusInternalServerError, `{"success":false,"error":"All scraping engines failed to retrieve content from this URL."}`, nil)
	})
	var stdout, stderr bytes.Buffer
	started := time.Now()
	code := Run([]string{"read-link", "https://example.com/it's-slow"}, &stdout, &stderr)
	if code != readLinkPendingExitCode {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("quick read exceeded its budget: %s", elapsed)
	}
	if browser.Load() {
		t.Fatal("a timed-out quick read must not start a browser session")
	}
	notice := stderr.String()
	for _, want := range []string{"timed out", "nothing to cancel", `webctx read-link 'https://example.com/it'\''s-slow' --wait`, "skip this page"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("pending notice missing %q:\n%s", want, notice)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("pending notice must not be written to stdout: %q", stdout.String())
	}
}

func TestQuickReadClientDeadlineReturnsPending(t *testing.T) {
	shrinkReadBudgets(t, 1200*time.Millisecond, 3*time.Second, 5*time.Second, 500*time.Millisecond)
	stubFirecrawl(t, func(req *http.Request, body map[string]any) *http.Response {
		<-req.Context().Done() // Firecrawl hangs past its own timeout.
		return testHTTPResponse(req, http.StatusGatewayTimeout, "", nil)
	})
	started := time.Now()
	_, _, err := scrapeLinkWithFirecrawlWithin("https://example.com/hang", true, newReadBudget(false))
	var pending *readLinkPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("expected pending error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("client deadline not enforced: %s", elapsed)
	}
}

func TestBrowserFallbackOnlyRunsWhenBudgetFits(t *testing.T) {
	var interacts atomic.Int32
	stubFirecrawl(t, func(req *http.Request, body map[string]any) *http.Response {
		switch req.Method + " " + req.URL.Path {
		case "POST /v2/scrape":
			return scrapeOK(req, "Just a moment... checking your browser")
		case "POST /v2/interact":
			interacts.Add(1)
			return testHTTPResponse(req, http.StatusOK, `{"success":true,"id":"budget123"}`, nil)
		case "POST /v2/interact/budget123/execute":
			out, _ := json.Marshal(map[string]any{"success": true, "exitCode": 0, "stdout": strings.Repeat("Rendered documentation text. ", 20)})
			return testHTTPResponse(req, http.StatusOK, string(out), nil)
		default:
			return testHTTPResponse(req, http.StatusOK, `{"success":true}`, nil)
		}
	})

	shrinkReadBudgets(t, 2*time.Second, 5*time.Second, time.Second, 4*time.Second)
	_, _, err := scrapeLinkWithFirecrawlWithin("https://example.com/blocked", true, newReadBudget(false))
	var pending *readLinkPendingError
	if !errors.As(err, &pending) || !strings.Contains(pending.reason, "browser-extraction") {
		t.Fatalf("expected browser pending notice, got %v", err)
	}
	if interacts.Load() != 0 {
		t.Fatal("browser fallback started without enough quick budget")
	}

	_, text, err := scrapeLinkWithFirecrawlWithin("https://example.com/blocked", true, newReadBudget(true))
	if err != nil || !strings.Contains(text, "Rendered documentation") || interacts.Load() != 1 {
		t.Fatalf("--wait should run the browser fallback: %v %q (interacts=%d)", err, text, interacts.Load())
	}
}

func TestReadLinkWaitFlagParsing(t *testing.T) {
	for _, args := range [][]string{{"https://example.com", "--wait"}, {"--wait", "https://example.com"}} {
		flags, positional := parseArgs(args)
		if flags["wait"] != "true" || len(positional) != 1 || positional[0] != "https://example.com" {
			t.Fatalf("%v parsed as flags=%v positional=%v", args, flags, positional)
		}
	}
}

func TestPendingNoticeQuotesQuestion(t *testing.T) {
	notice := formatReadLinkPending("https://example.com/a", "what's new?", &readLinkPendingError{reason: "x", budget: 27 * time.Second})
	if !strings.Contains(notice, `--question 'what'\''s new?' --wait`) || !strings.Contains(notice, "within 27s") {
		t.Fatalf("unexpected notice:\n%s", notice)
	}
}

func TestFirecrawlAttemptTimedOut(t *testing.T) {
	generic := errors.New("API request failed: 500 - All scraping engines failed")
	if firecrawlAttemptTimedOut(generic, time.Now(), 25*time.Second) {
		t.Fatal("a fast engine failure is not a timeout")
	}
	if !firecrawlAttemptTimedOut(generic, time.Now().Add(-25*time.Second), 25*time.Second) {
		t.Fatal("an engine failure at Firecrawl's timeout is a timeout")
	}
	if !firecrawlAttemptTimedOut(errors.New(`Post "https://api.firecrawl.dev/v2/scrape": context deadline exceeded`), time.Now(), 0) {
		t.Fatal("redacted client deadline errors are timeouts")
	}
}
