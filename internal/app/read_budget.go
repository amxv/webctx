package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Agents usually run `webctx read-link` inside a synchronous shell tool, so a
// slow Firecrawl scrape must not hold that tool for minutes. The CLI uses a
// quick budget by default and an explicit --wait mode that agents run as a
// long-running command. Origo and other library callers pass a nil budget and
// keep their existing timing.
//
// Firecrawl does not bill scrapes that hit its own server-side timeout, so the
// quick budget sends an explicit Firecrawl timeout rather than only cutting the
// connection client-side (which would leave Firecrawl working and billing).
var (
	readLinkQuickBudget = 27 * time.Second
	readLinkWaitBudget  = 4 * time.Minute
	// The CLI watchdog fires slightly after the budget so in-flight steps
	// normally finish or time out on their own first.
	readLinkWatchdogGrace = 2 * time.Second

	firecrawlQuickScrapeTimeout = 25 * time.Second
	firecrawlWaitScrapeTimeout  = 120 * time.Second
	// Firecrawl rejects timeouts under 1s, and very short attempts mostly fail
	// as generic engine errors, so skip attempts that cannot get this much.
	firecrawlMinScrapeTimeout = 5 * time.Second
	// Allowance after Firecrawl's own timeout for its response to arrive, so
	// Firecrawl rather than webctx normally ends a slow attempt.
	firecrawlResponseGrace = 3 * time.Second
	// Browser session creation, the 20s sandbox command, and teardown.
	firecrawlBrowserMinBudget = 30 * time.Second
	firecrawlBrowserMaxBudget = 120 * time.Second
)

// readLinkPendingExitCode is EX_TEMPFAIL: the page was not readable within the
// quick budget, but a --wait retry may succeed.
const readLinkPendingExitCode = 75

var errReadBudgetExhausted = errors.New("not enough time left in the read budget for another Firecrawl attempt")

type readBudget struct {
	deadline time.Time
	wait     bool
}

func newReadBudget(wait bool) *readBudget {
	total := readLinkQuickBudget
	if wait {
		total = readLinkWaitBudget
	}
	return &readBudget{deadline: time.Now().Add(total), wait: wait}
}

func (b *readBudget) total() time.Duration {
	if b.wait {
		return readLinkWaitBudget
	}
	return readLinkQuickBudget
}

// scrapeTimeout is Firecrawl's server-side timeout for the next attempt. ok is
// false when too little budget remains for a useful attempt.
func (b *readBudget) scrapeTimeout() (time.Duration, bool) {
	limit := firecrawlQuickScrapeTimeout
	if b.wait {
		limit = firecrawlWaitScrapeTimeout
	}
	if left := time.Until(b.deadline) - firecrawlResponseGrace; left < limit {
		limit = left
	}
	limit = limit.Truncate(time.Millisecond)
	return limit, limit >= firecrawlMinScrapeTimeout
}

// firecrawlScrape runs one scrape attempt within the budget. A nil budget keeps
// the legacy request without an explicit Firecrawl timeout.
func (b *readBudget) firecrawlScrape(rawURL, apiKey, proxy string, domainTools bool) (scrapedPage, error) {
	if b == nil {
		return firecrawlScrape(rawURL, apiKey, proxy, domainTools)
	}
	timeout, ok := b.scrapeTimeout()
	if !ok {
		return scrapedPage{}, errReadBudgetExhausted
	}
	ctx, cancel := context.WithDeadline(context.Background(), b.deadline)
	defer cancel()
	started := time.Now()
	page, err := firecrawlScrapeContext(ctx, rawURL, apiKey, proxy, domainTools, timeout)
	if err != nil && firecrawlAttemptTimedOut(err, started, timeout) {
		return page, &firecrawlTimeoutError{after: time.Since(started), err: err}
	}
	return page, err
}

// browserFallback runs browser extraction only when the budget can fit it.
func (b *readBudget) browserFallback(rawURL, apiKey string, lastError error) (string, string, error) {
	if b == nil {
		return firecrawlBrowserFallback(rawURL, apiKey)
	}
	if time.Until(b.deadline) < firecrawlBrowserMinBudget {
		if b.wait {
			return "", "", fmt.Errorf("not enough time left in the %s budget for browser extraction", formatBudget(b.total()))
		}
		reason := "Firecrawl's quick scrape did not return usable content, and the slower browser-extraction fallback needs more time"
		if lastError != nil {
			reason += " (last error: " + truncateError(lastError, 200) + ")"
		}
		return "", "", &readLinkPendingError{reason: reason, budget: b.total()}
	}
	deadline := time.Now().Add(firecrawlBrowserMaxBudget)
	if b.deadline.Before(deadline) {
		deadline = b.deadline
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	return firecrawlBrowserFallbackContext(ctx, rawURL, apiKey)
}

// pending converts a timed-out or out-of-budget attempt into a quick-mode
// pending result. Wait mode and legacy callers keep escalating instead.
func (b *readBudget) pending(err error) error {
	if b == nil || b.wait || err == nil {
		return nil
	}
	var timeout *firecrawlTimeoutError
	switch {
	case errors.As(err, &timeout):
		return &readLinkPendingError{reason: fmt.Sprintf("Firecrawl's scrape timed out after %s", formatBudget(timeout.after)), budget: b.total()}
	case errors.Is(err, errReadBudgetExhausted):
		return &readLinkPendingError{reason: "there was not enough time left for another Firecrawl attempt", budget: b.total()}
	}
	return nil
}

type firecrawlTimeoutError struct {
	after time.Duration
	err   error
}

func (e *firecrawlTimeoutError) Error() string {
	return fmt.Sprintf("Firecrawl scrape timed out after %s: %v", formatBudget(e.after), e.err)
}

func (e *firecrawlTimeoutError) Unwrap() error { return e.err }

// firecrawlAttemptTimedOut recognizes both client deadlines and Firecrawl's own
// timeout, which it reports as a generic "All scraping engines failed" 500.
// Account-pool redaction drops error types, so messages and elapsed time are
// checked as well.
func firecrawlAttemptTimedOut(err error, started time.Time, serverTimeout time.Duration) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "context deadline exceeded") || strings.Contains(message, "timed out") || strings.Contains(message, "timeout") {
		return true
	}
	return serverTimeout > 0 && time.Since(started) >= serverTimeout-time.Second
}

// readLinkPendingError means the quick budget ran out before usable content
// arrived. Nothing keeps running afterwards, so the only follow-ups are a
// --wait retry or skipping the page.
type readLinkPendingError struct {
	reason string
	budget time.Duration
}

func (e *readLinkPendingError) Error() string {
	return fmt.Sprintf("page was not ready within the %s read-link budget: %s", formatBudget(e.budget), e.reason)
}

// formatReadLinkPending tells the agent exactly how to retry with --wait.
func formatReadLinkPending(rawURL, question string, pending *readLinkPendingError) string {
	command := "webctx read-link " + quoteShellURL(rawURL)
	if strings.TrimSpace(question) != "" {
		command += " --question " + quoteShellURL(question)
	}
	command += " --wait"
	return fmt.Sprintf(`read-link: %s was not ready within %s: %s.
Nothing was left running, so there is nothing to cancel.

Next steps:
1. Retry with a longer budget (up to %s). Run it as a long-running or background command so it does not block you:
   %s
2. Or skip this page and continue with other sources.`,
		rawURL, formatBudget(pending.budget), pending.reason, formatBudget(readLinkWaitBudget), command)
}

// runReadLinkCLI applies one budget to the whole command, including related
// sources read for --question, and enforces it with a watchdog so slow
// non-Firecrawl paths cannot block the caller either.
func runReadLinkCLI(rawURL, question string, wait bool) (string, error) {
	budget := newReadBudget(wait)
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := readLinkFocused(rawURL, question, func(u string) (string, error) {
			return readLinkWithin(u, true, budget)
		})
		done <- result{text, err}
	}()
	watchdog := time.NewTimer(time.Until(budget.deadline) + readLinkWatchdogGrace)
	defer watchdog.Stop()
	select {
	case r := <-done:
		return r.text, r.err
	case <-watchdog.C:
		if wait {
			return "", fmt.Errorf("Error reading web page: did not finish within the %s --wait budget", formatBudget(readLinkWaitBudget))
		}
		return "", &readLinkPendingError{reason: "the read was still in progress", budget: budget.total()}
	}
}

func formatBudget(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		minutes := int(d / time.Minute)
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	if d >= time.Second {
		return fmt.Sprintf("%ds", int(d.Round(time.Second)/time.Second))
	}
	return d.Round(time.Millisecond).String()
}

func truncateError(err error, maxRunes int) string {
	runes := []rune(err.Error())
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}
