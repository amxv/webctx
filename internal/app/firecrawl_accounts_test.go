package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFirecrawlKeysAreSeparateAccounts(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEYS", "firecrawl-team-a,firecrawl-team-b,firecrawl-team-a")
	t.Setenv("FIRECRAWL_API_KEY", "firecrawl-team-primary")
	if got, want := firecrawlKeys(), []string{"firecrawl-team-a", "firecrawl-team-b", "firecrawl-team-primary"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Firecrawl credential list: %v", got)
	}
	if firecrawlDefaultKey() != "firecrawl-team-primary" {
		t.Fatal("paid Alexandria requests must keep the explicitly chosen primary account")
	}
	t.Setenv("FIRECRAWL_API_KEY", "")
	if firecrawlDefaultKey() != "firecrawl-team-a" {
		t.Fatal("plural-only Firecrawl credentials should work without a singular key")
	}
}

func TestFirecrawlKeychainManagementAndStatus(t *testing.T) {
	t.Setenv("WEBCTX_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("FIRECRAWL_API_KEYS", "test-account-A,test-account-B")
	t.Setenv("FIRECRAWL_API_KEY", "")
	previousLookup, previousStore := keychainLookup, keychainStore
	t.Cleanup(func() { keychainLookup, keychainStore = previousLookup, previousStore })
	store := map[string]string{}
	keychainLookup = func(account string) (string, error) { return store[account], nil }
	keychainStore = func(account, value string) error { store[account] = value; return nil }
	if count, err := addManagedSearchKey("firecrawl", strings.NewReader("managed-secret\n")); err != nil || count != 1 {
		t.Fatalf("Firecrawl Keychain add: %d %v", count, err)
	}
	if managed, err := readManagedKeys("firecrawl"); err != nil || !reflect.DeepEqual(managed, []string{"managed-secret"}) {
		t.Fatalf("Firecrawl Keychain entry: %v %v", managed, err)
	}
	var status strings.Builder
	if err := printProviderStatus(&status); err != nil || !strings.Contains(status.String(), "key configured (2 separate accounts)") || strings.Contains(status.String(), "test-account-A") {
		t.Fatalf("Firecrawl status leaked a key or missed account count: %v %q", err, status.String())
	}
	if count, err := removeManagedSearchKey("firecrawl", "1"); err != nil || count != 0 {
		t.Fatalf("Firecrawl Keychain remove: %d %v", count, err)
	}
}

func TestFirecrawlRateAndBrowserSlotsIndependentPerAccount(t *testing.T) {
	root := &firecrawlQueue{dir: t.TempDir(), window: time.Second}
	a, b := root.forKey("secret-A"), root.forKey("secret-B")
	if delay, err := a.reserveRateToken("scrape", 1); err != nil || delay != 0 {
		t.Fatalf("account A first request: %s %v", delay, err)
	}
	if delay, err := root.forKey("secret-A").reserveRateToken("scrape", 1); err != nil || delay < 900*time.Millisecond {
		t.Fatalf("same account bypassed its quota across queue instances: %s %v", delay, err)
	}
	if delay, err := b.reserveRateToken("scrape", 1); err != nil || delay != 0 {
		t.Fatalf("second account should have full quota: %s %v", delay, err)
	}
	if _, err := a.rateCooldown("scrape", time.Minute); err != nil {
		t.Fatal(err)
	}
	if delay, err := b.reserveRateToken("map", 1); err != nil || delay != 0 {
		t.Fatalf("other account or endpoint poisoned by cooldown: %s %v", delay, err)
	}
	if err := a.setup(); err != nil {
		t.Fatal(err)
	}
	first, err := a.tryBrowserSlot()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unlockQueueFile(first); _ = first.Close() }()
	second, err := a.tryBrowserSlot()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unlockQueueFile(second); _ = second.Close() }()
	if third, err := root.forKey("secret-A").tryBrowserSlot(); err != nil || third != nil {
		t.Fatalf("third browser on same account acquired a slot: %v %v", third, err)
	}
	if err := b.setup(); err != nil {
		t.Fatal(err)
	}
	separate, err := b.tryBrowserSlot()
	if err != nil || separate == nil {
		t.Fatalf("separate Firecrawl account cannot use its own browser quota: %v", err)
	}
	unlockQueueFile(separate)
	_ = separate.Close()
	if _, err := os.Stat(filepath.Join(root.dir, "accounts", "secret-A")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unhashed key is part of Firecrawl queue path: %v", err)
	}
}

func TestFirecrawlPooledRequestFailsOverAndKeepsKeyCooldownLocal(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEYS", "test-account-bad,test-account-good")
	t.Setenv("FIRECRAWL_API_KEY", "")
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("WEBCTX_FIRECRAWL_QUEUE_DIR", t.TempDir())
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var attempts []string
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		key := firecrawlRequestCredential(req)
		attempts = append(attempts, key)
		if key == "test-account-bad" {
			return testHTTPResponse(req, 429, `{"error":"rate limited"}`, map[string]string{"Retry-After": "30"}), nil
		}
		return testHTTPResponse(req, 200, `{"success":true,"links":[]}`, nil), nil
	})
	for i := 0; i < 2; i++ {
		body, err := doJSONRequest(context.Background(), http.MethodPost, "https://api.firecrawl.dev/v2/map", map[string]string{
			"Authorization": "Bearer " + firecrawlDefaultKey(),
		}, map[string]any{"url": "https://example.com"})
		if err != nil || !strings.Contains(string(body), `"success":true`) {
			t.Fatalf("Firecrawl map failover attempt %d: %s %v", i, body, err)
		}
	}
	if !reflect.DeepEqual(attempts, []string{"test-account-bad", "test-account-good", "test-account-good"}) {
		t.Fatalf("per-account 429 must not block healthy accounts: %v", attempts)
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("WEBCTX_SEARCH_KEY_BALANCER_DIR"), "firecrawl.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state searchKeyBalanceState
	if json.Unmarshal(data, &state) != nil || state.Cooldowns[searchKeyID("test-account-bad")] != 0 {
		t.Fatalf("Firecrawl endpoint 429 improperly disabled the whole account: %s", data)
	}
	root := getFirecrawlQueue()
	if wait, err := root.forKey("test-account-bad").rateCooldown("map", 0); err != nil || wait < 20*time.Second {
		t.Fatalf("bad account's map cooldown was not retained: %s %v", wait, err)
	}
	if wait, err := root.forKey("test-account-bad").rateCooldown("scrape", 0); err != nil || wait > 0 {
		t.Fatalf("one account's map rate limit blocked scrape: %s %v", wait, err)
	}
	if wait, err := root.forKey("test-account-good").rateCooldown("map", 0); err != nil || wait > 0 {
		t.Fatalf("a rejected account throttled a different account: %s %v", wait, err)
	}
	if strings.Contains(string(data), "test-account") {
		t.Fatal("Firecrawl shared state contains API secrets")
	}
}

func TestFirecrawlBrowserAccountSelectsAvailableAccount(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEYS", "test-browser-A,test-browser-B")
	t.Setenv("FIRECRAWL_API_KEY", "")
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("WEBCTX_FIRECRAWL_QUEUE_DIR", t.TempDir())
	q := getFirecrawlQueue().forKey("test-browser-A")
	if err := q.setup(); err != nil {
		t.Fatal(err)
	}
	first, err := q.tryBrowserSlot()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unlockQueueFile(first); _ = first.Close() }()
	second, err := q.tryBrowserSlot()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unlockQueueFile(second); _ = second.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	key, permit, err := firecrawlBrowserAccount(ctx)
	if err != nil || key != "test-browser-B" || permit == nil {
		t.Fatalf("browser account did not use idle second key: %q %v", key, err)
	}
	permit.release()
}

func TestFirecrawlBrowserSessionCreationFailsOverAndPinsCommands(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEYS", "test-browser-rate-limited,test-browser-healthy")
	t.Setenv("FIRECRAWL_API_KEY", "")
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("WEBCTX_FIRECRAWL_QUEUE_DIR", t.TempDir())
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var calls []string
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		key := firecrawlRequestCredential(req)
		calls = append(calls, key+" "+req.Method+" "+req.URL.Path)
		switch {
		case req.URL.Path == "/v2/interact" && key == "test-browser-rate-limited":
			return testHTTPResponse(req, 429, `{"error":"rate limited"}`, map[string]string{"Retry-After": "30"}), nil
		case req.URL.Path == "/v2/interact" && key == "test-browser-healthy":
			return testHTTPResponse(req, 200, `{"success":true,"id":"browser-session-abc"}`, nil), nil
		case req.URL.Path == "/v2/interact/browser-session-abc/execute" && key == "test-browser-healthy":
			return testHTTPResponse(req, 200, `{"success":true,"stdout":"\"Original documentation text\"","exitCode":0}`, nil), nil
		case req.URL.Path == "/v2/interact/browser-session-abc" && key == "test-browser-healthy":
			return testHTTPResponse(req, 200, `{"success":true}`, nil), nil
		default:
			t.Errorf("browser command went to the wrong account: %s %s %s", key, req.Method, req.URL.Path)
			return testHTTPResponse(req, 500, `{"error":"wrong account"}`, nil), nil
		}
	})
	title, text, err := firecrawlBrowserFallback("https://docs.example.com", "test-browser-rate-limited")
	if err != nil || title != "Browser extraction" || text != "Original documentation text" {
		t.Fatalf("browser session failover: %q %q %v", title, text, err)
	}
	if len(calls) != 4 || !strings.Contains(calls[0], "test-browser-rate-limited") {
		t.Fatalf("unexpected browser creation sequence: %v", calls)
	}
	for _, call := range calls[1:] {
		if !strings.Contains(call, "test-browser-healthy") {
			t.Fatalf("a browser session command changed accounts: %v", calls)
		}
	}
}

func TestFirecrawlPaidExecutionPinsOriginalAccount(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEYS", "secondary-account")
	t.Setenv("FIRECRAWL_API_KEY", "primary-account")
	t.Setenv("WEBCTX_FIRECRAWL_QUEUE_DIR", t.TempDir())
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var seen []string
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, firecrawlRequestCredential(req))
		return testHTTPResponse(req, 200, `{"success":true,"data":{}}`, nil), nil
	})
	for i := 0; i < 2; i++ {
		if _, err := doDataAPI(context.Background(), "/scrape", map[string]any{"alexandria": "test"}, "same-idempotency-key"); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(seen, []string{"primary-account", "primary-account"}) {
		t.Fatalf("paid requests switched Firecrawl billing accounts: %v", seen)
	}
}

func TestFirecrawlKeyQuotaSubprocessHelper(t *testing.T) {
	if os.Getenv("WEBCTX_FIRECRAWL_ACCOUNTS_TEST_HELPER") != "1" {
		return
	}
	q := getFirecrawlQueue().forKey(os.Getenv("WEBCTX_TEST_ACCOUNT"))
	q.window = 30 * time.Second
	delay, err := q.reserveRateToken("scrape", 3)
	if err != nil {
		t.Fatal(err)
	}
	if delay == 0 {
		fmt.Println("ACCOUNT_ADMITTED")
	} else {
		fmt.Println("ACCOUNT_BLOCKED")
	}
}

func TestFirecrawlPerAccountRateLimitsAcrossProcesses(t *testing.T) {
	const perAccount = 8
	const allowed = 3
	dir := t.TempDir()
	results := make(chan string, perAccount*2)
	var wg sync.WaitGroup
	for _, account := range []string{"account-A", "account-B"} {
		for i := 0; i < perAccount; i++ {
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFirecrawlKeyQuotaSubprocessHelper$", "-test.v")
				cmd.Env = append(os.Environ(),
					"WEBCTX_FIRECRAWL_ACCOUNTS_TEST_HELPER=1",
					"WEBCTX_FIRECRAWL_QUEUE_DIR="+dir,
					"WEBCTX_TEST_ACCOUNT="+key)
				output, err := cmd.CombinedOutput()
				if err != nil {
					results <- fmt.Sprintf("error:%s:%v:%s", key, err, output)
				} else if strings.Contains(string(output), "ACCOUNT_ADMITTED") {
					results <- key + ":admitted"
				} else if strings.Contains(string(output), "ACCOUNT_BLOCKED") {
					results <- key + ":blocked"
				} else {
					results <- fmt.Sprintf("error:%s:%s", key, output)
				}
			}(account)
		}
	}
	wg.Wait()
	close(results)
	counts := map[string]int{}
	for result := range results {
		counts[result]++
	}
	if counts["account-A:admitted"] != allowed || counts["account-B:admitted"] != allowed ||
		counts["account-A:blocked"] != perAccount-allowed || counts["account-B:blocked"] != perAccount-allowed {
		t.Fatalf("separate CLI processes did not share per-account rate limits: %v", counts)
	}
}

func TestFirecrawlDomainToolsPermissionDoesNotDisableAccount(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://api.firecrawl.dev/v2/scrape", strings.NewReader(`{"domainTools":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if firecrawlRetryableStatus(403, req) {
		t.Fatal("domainTools 403 should preserve same-account fallback")
	}
	req, _ = http.NewRequest(http.MethodPost, "https://api.firecrawl.dev/v2/scrape", strings.NewReader(`{"domainTools":false}`))
	if !firecrawlRetryableStatus(403, req) {
		t.Fatal("invalid key should be able to fall back after domainTools is disabled")
	}
	if !pooledFirecrawlEndpoint(req) {
		t.Fatal("scrape should participate in the independent-account pool")
	}
}
