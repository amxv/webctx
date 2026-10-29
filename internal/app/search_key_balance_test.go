package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSearchKeysRoundRobinAcrossAllProviders(t *testing.T) {
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	for _, provider := range []string{"brave", "tavily", "exa"} {
		keys := []string{provider + "-private-first", provider + "-private-second", provider + "-private-third"}
		prefix := strings.ToUpper(provider) + "_API_KEY"
		t.Setenv(prefix+"S", strings.Join(keys[:2], ","))
		t.Setenv(prefix, keys[2])
		var got []string
		for i := 0; i < 9; i++ {
			_, err := useSearchKeys(context.Background(), provider, func(key string) ([]byte, error) {
				got = append(got, key)
				return []byte("ok"), nil
			})
			if err != nil {
				t.Fatalf("%s round robin %d: %v", provider, i, err)
			}
		}
		want := []string{keys[0], keys[1], keys[2], keys[0], keys[1], keys[2], keys[0], keys[1], keys[2]}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s round robin: got %v, want %v", provider, got, want)
		}
		state, err := os.ReadFile(filepath.Join(os.Getenv("WEBCTX_SEARCH_KEY_BALANCER_DIR"), provider+".json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range keys {
			if strings.Contains(string(state), secret) {
				t.Fatalf("secret was persisted in %s balancer state", provider)
			}
		}
	}
}

func TestSearchKeyBalancerSharedCooldownAndRecovery(t *testing.T) {
	now := time.Now()
	b := searchKeyBalancer{dir: t.TempDir(), now: func() time.Time { return now }}
	keys := []string{"private-A", "private-B", "private-C"}
	ctx := context.Background()
	first, err := b.reserve(ctx, "tavily", keys)
	if err != nil || !reflect.DeepEqual(first, []int{0, 1, 2}) {
		t.Fatalf("initial selection: %v %v", first, err)
	}
	if err := b.coolDown(ctx, "tavily", keys[0], 60*time.Second); err != nil {
		t.Fatal(err)
	}
	second, err := b.reserve(ctx, "tavily", keys)
	if err != nil || !reflect.DeepEqual(second, []int{1, 2}) {
		t.Fatalf("should avoid first temporarily unavailable key: %v %v", second, err)
	}
	// A second balancer (representing another CLI invocation) sees the same
	// cooldown without any shared in-memory state.
	other := searchKeyBalancer{dir: b.dir, now: b.now}
	if ready, err := other.available(ctx, "tavily", keys[0]); err != nil || ready {
		t.Fatalf("independent process sees unavailable key as available: ready=%t err=%v", ready, err)
	}
	if err := other.coolDown(ctx, "tavily", keys[1], 60*time.Second); err != nil {
		t.Fatal(err)
	}
	third, err := b.reserve(ctx, "tavily", keys)
	if err != nil || !reflect.DeepEqual(third, []int{2}) {
		t.Fatalf("should use the only healthy key: %v %v", third, err)
	}
	if err := other.coolDown(ctx, "tavily", keys[2], 60*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := b.reserve(ctx, "tavily", keys); err == nil || !strings.Contains(err.Error(), "temporarily unavailable") {
		t.Fatalf("should not continuously hammer rate-limited keys: %v", err)
	}
	now = now.Add(61 * time.Second)
	ready, err := b.reserve(ctx, "tavily", keys)
	if err != nil || len(ready) != 3 || ready[0] != 0 {
		t.Fatalf("cooldowns did not expire into original rotation: %v %v", ready, err)
	}
}

func TestSearchKeysFallbackSkipsUnavailableCredentials(t *testing.T) {
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("TAVILY_API_KEYS", "private-A,private-B")
	t.Setenv("TAVILY_API_KEY", "private-C")
	var attempts []string
	try := func(key string) ([]byte, error) {
		attempts = append(attempts, key)
		switch key {
		case "private-A":
			return nil, &httpStatusError{Code: 429, RetryAfter: time.Minute, Message: "rate limited"}
		case "private-B":
			return nil, &httpStatusError{Code: 432, Message: "quota exhausted"}
		default:
			return []byte("result"), nil
		}
	}
	result, err := useSearchKeys(context.Background(), "tavily", try)
	if err != nil || string(result) != "result" || !reflect.DeepEqual(attempts, []string{"private-A", "private-B", "private-C"}) {
		t.Fatalf("initial failover did not reach the third key: result=%s attempts=%v err=%v", result, attempts, err)
	}
	attempts = nil
	result, err = useSearchKeys(context.Background(), "tavily", try)
	if err != nil || string(result) != "result" || !reflect.DeepEqual(attempts, []string{"private-C"}) {
		t.Fatalf("subsequent request retried cooled keys: result=%s attempts=%v err=%v", result, attempts, err)
	}
}

func TestSeparateExaAndBraveAccountsCanFailOverAfter429(t *testing.T) {
	for _, provider := range []string{"exa", "brave"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
			prefix := strings.ToUpper(provider) + "_API_KEY"
			t.Setenv(prefix+"S", "bad-account-key,good-account-key")
			t.Setenv(prefix, "")
			attempts := []string{}
			call := func(key string) ([]byte, error) {
				attempts = append(attempts, key)
				if key == "bad-account-key" {
					return nil, &httpStatusError{Code: 429, RetryAfter: 30 * time.Second, Message: "rate limited"}
				}
				return []byte("ok"), nil
			}
			if response, err := useSearchKeys(context.Background(), provider, call); err != nil || string(response) != "ok" {
				t.Fatalf("separate account failover: %q %v", response, err)
			}
			if !reflect.DeepEqual(attempts, []string{"bad-account-key", "good-account-key"}) {
				t.Fatalf("did not retry healthy second account: %v", attempts)
			}
			attempts = nil
			if response, err := useSearchKeys(context.Background(), provider, call); err != nil || string(response) != "ok" || !reflect.DeepEqual(attempts, []string{"good-account-key"}) {
				t.Fatalf("shared cooldown poisoned healthy account: %q %v %v", response, err, attempts)
			}
			data, err := os.ReadFile(filepath.Join(os.Getenv("WEBCTX_SEARCH_KEY_BALANCER_DIR"), provider+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var state searchKeyBalanceState
			if json.Unmarshal(data, &state) != nil || state.Cooldowns[searchKeyID("bad-account-key")] == 0 || state.Cooldowns[searchKeyID("good-account-key")] != 0 {
				t.Fatalf("incorrect per-account provider cooldown: %s", data)
			}
		})
	}
}

func TestTavilyHTTPFailoverAlsoCoolsDownKeysForLaterCalls(t *testing.T) {
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", t.TempDir())
	t.Setenv("TAVILY_API_KEYS", "test-key-A,test-key-B")
	t.Setenv("TAVILY_API_KEY", "test-key-C")
	previous := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
	var attempted []string
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal(payload, &body); err != nil {
			return nil, err
		}
		key, _ := body["api_key"].(string)
		attempted = append(attempted, key)
		switch key {
		case "test-key-A":
			return testHTTPResponse(req, 429, `{"error":"rate limited"}`, map[string]string{"Retry-After": "8"}), nil
		case "test-key-B":
			return testHTTPResponse(req, 432, `{"error":"quota exceeded"}`, nil), nil
		default:
			return testHTTPResponse(req, 200, `{"results":[{"url":"https://go.dev/doc/","title":"Go Documentation","content":"Official reference."}]}`, nil), nil
		}
	})
	first, err := searchWithTavily(context.Background(), "Go API", nil, false)
	if err != nil || len(first.Docs) != 1 || !reflect.DeepEqual(attempted, []string{"test-key-A", "test-key-B", "test-key-C"}) {
		t.Fatalf("fallback must reach third credential: %v results=%d attempts=%v", err, len(first.Docs), attempted)
	}
	state, err := os.ReadFile(filepath.Join(os.Getenv("WEBCTX_SEARCH_KEY_BALANCER_DIR"), "tavily.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "test-key") {
		t.Fatal("raw API keys leaked into shared rotation state")
	}
	var parsed searchKeyBalanceState
	if err := json.Unmarshal(state, &parsed); err != nil {
		t.Fatal(err)
	}
	if until := parsed.Cooldowns[searchKeyID("test-key-A")]; until < time.Now().Add(5*time.Second).UnixMilli() {
		t.Errorf("Retry-After header was not reflected in the shared cooldown")
	}
	attempted = nil
	second, err := searchWithTavily(context.Background(), "Go API", nil, false)
	if err != nil || len(second.Docs) != 1 || !reflect.DeepEqual(attempted, []string{"test-key-C"}) {
		t.Fatalf("subsequent Tavily search ignored shared cooldowns: err=%v attempts=%v", err, attempted)
	}
}

func TestSearchKeysSingleKeySharesAdmissionState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "single-key-admissions")
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", dir)
	t.Setenv("BRAVE_API_KEYS", "")
	t.Setenv("BRAVE_API_KEY", "single-private-key")
	_, err := useSearchKeys(context.Background(), "brave", func(_ string) ([]byte, error) { return []byte("ok"), nil })
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "brave.json"))
	if err != nil || !strings.Contains(string(data), "admissions") || strings.Contains(string(data), "single-private-key") {
		t.Fatalf("single-key search must reserve shared capacity without storing secrets: %s %v", data, err)
	}
}

func TestSearchKeyBalancerConcurrentRoundRobin(t *testing.T) {
	balancer := searchKeyBalancer{dir: t.TempDir(), now: time.Now}
	keys := []string{"first", "second", "third"}
	counts := [3]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	issues := make(chan error, 90)
	for i := 0; i < 90; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order, err := balancer.reserve(context.Background(), "exa", keys)
			if err != nil {
				issues <- err
				return
			}
			mu.Lock()
			counts[order[0]]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	close(issues)
	for err := range issues {
		t.Fatal(err)
	}
	if counts != [3]int{30, 30, 30} {
		t.Fatalf("concurrent reservations were not balanced: %v", counts)
	}
}

func TestSearchBalancerSubprocessHelper(t *testing.T) {
	if os.Getenv("WEBCTX_SEARCH_BALANCER_TEST_HELPER") != "1" {
		return
	}
	keys := []string{"test-first", "test-second", "test-third"}
	result, err := currentSearchKeyBalancer().reserve(context.Background(), "tavily", keys)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("WEBCTX_SELECTED_SLOT=%d\n", result[0])
}

func TestSearchBalancerRoundRobinAcrossSubprocesses(t *testing.T) {
	dir := t.TempDir()
	const count = 15
	results := make(chan int, count)
	errors := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSearchBalancerSubprocessHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "WEBCTX_SEARCH_BALANCER_TEST_HELPER=1", "WEBCTX_SEARCH_KEY_BALANCER_DIR="+dir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				errors <- fmt.Errorf("subprocess: %w", err)
				return
			}
			for _, line := range strings.Split(string(output), "\n") {
				if strings.HasPrefix(line, "WEBCTX_SELECTED_SLOT=") {
					slot, err := strconv.Atoi(strings.TrimPrefix(line, "WEBCTX_SELECTED_SLOT="))
					if err == nil {
						results <- slot
						return
					}
				}
			}
			errors <- fmt.Errorf("subprocess did not report a selected slot")
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	counts := [3]int{}
	for slot := range results {
		if slot < 0 || slot >= 3 {
			t.Fatalf("invalid selected key index: %d", slot)
		}
		counts[slot]++
	}
	if counts != [3]int{5, 5, 5} {
		t.Fatalf("separate processes failed to share round-robin cursor: %v", counts)
	}
}

func TestSearchKeyBalancerStateSurvivesKeyListChanges(t *testing.T) {
	b := searchKeyBalancer{dir: t.TempDir(), now: time.Now}
	ctx := context.Background()
	_, err := b.reserve(ctx, "brave", []string{"key-A", "key-B"})
	if err != nil {
		t.Fatal(err)
	}
	order, err := b.reserve(ctx, "brave", []string{"key-A", "key-new", "key-B"})
	if err != nil || order[0] != 1 {
		t.Fatalf("new credentials should join the existing rotation after the last selected key: %v %v", order, err)
	}
	order, err = b.reserve(ctx, "brave", []string{"key-B", "key-A"})
	if err != nil || len(order) != 2 {
		t.Fatalf("removing a key broke rotation: %v %v", order, err)
	}
	state, err := os.ReadFile(filepath.Join(b.dir, "brave.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed searchKeyBalanceState
	if json.Unmarshal(state, &parsed) != nil || parsed.LastSelected == "" {
		t.Fatalf("invalid persisted rotation state")
	}
}

func TestSearchRetryAfterHeaderAndErrorClassification(t *testing.T) {
	for _, test := range []struct {
		raw      string
		expected time.Duration
	}{
		{"", 0}, {"7", 7 * time.Second}, {"999999", 24 * time.Hour}, {"invalid", 0}, {"-1", 0},
	} {
		if got := searchRetryAfter(test.raw); got != test.expected {
			t.Errorf("Retry-After %q: got %s, want %s", test.raw, got, test.expected)
		}
	}
	if got := searchKeyFailureCooldown(&httpStatusError{Code: http.StatusTooManyRequests, RetryAfter: 7 * time.Second}); got != 7*time.Second {
		t.Errorf("Retry-After was not respected: %v", got)
	}
	if got := searchKeyFailureCooldown(&httpStatusError{Code: http.StatusServiceUnavailable}); got >= time.Minute {
		t.Errorf("transient server errors should have a short cooldown, got %v", got)
	}
}
