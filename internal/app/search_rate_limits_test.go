package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchFreeRateDefaultsAndOverrides(t *testing.T) {
	for _, test := range []struct {
		name   string
		limit  int
		window time.Duration
	}{
		{"tavily", 100, time.Minute},
		{"exa", 10, time.Second},
		{"brave", 50, time.Second},
	} {
		rule := searchRateForProvider(test.name)
		if rule.limit != test.limit || rule.window != test.window {
			t.Errorf("%s default quota: %+v", test.name, rule)
		}
	}
	t.Setenv("WEBCTX_TAVILY_RPM", "1000")
	t.Setenv("WEBCTX_EXA_QPS", "25")
	t.Setenv("WEBCTX_BRAVE_QPS", "oops")
	if searchRateForProvider("tavily").limit != 1000 || searchRateForProvider("exa").limit != 25 || searchRateForProvider("brave").limit != 50 {
		t.Fatal("paid-plan overrides or invalid-value fallback failed")
	}
}

func TestSearchQuotaIndependentPerAccountKey(t *testing.T) {
	clock := time.Now()
	b := searchKeyBalancer{dir: t.TempDir(), now: func() time.Time { return clock }}
	other := searchKeyBalancer{dir: b.dir, now: b.now}
	rule := searchRateRule{limit: 3, window: time.Second}
	keys := []string{"first-secret", "second-secret", "third-secret"}
	for i := 0; i < 9; i++ {
		if selected, wait, err := b.tryAdmit(context.Background(), "exa", keys, nil, rule); err != nil || selected != i%3 || wait != 0 {
			t.Fatalf("admission %d: key=%d wait=%s err=%v", i, selected, wait, err)
		}
	}
	if selected, wait, err := other.tryAdmit(context.Background(), "exa", keys, nil, rule); err != nil || selected >= 0 || wait < 900*time.Millisecond {
		t.Fatalf("another process bypassed all per-account caps: key=%d wait=%s err=%v", selected, wait, err)
	}
	data, err := os.ReadFile(filepath.Join(b.dir, "exa.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range keys {
		if strings.Contains(string(data), secret) {
			t.Fatal("search scheduler persisted a raw API key")
		}
	}
	clock = clock.Add(time.Second + time.Millisecond)
	if index, wait, err := other.tryAdmit(context.Background(), "exa", keys, nil, rule); err != nil || index != 0 || wait > 0 {
		t.Fatalf("account capacity did not recover after window: %d %s %v", index, wait, err)
	}
}

func TestSearchTavilyUsesHealthyKeySpecificCapacity(t *testing.T) {
	clock := time.Now()
	b := searchKeyBalancer{dir: t.TempDir(), now: func() time.Time { return clock }}
	keys := []string{"tavily-A", "tavily-B", "tavily-C"}
	rule := searchRateRule{limit: 2, window: time.Minute}
	var counts [3]int
	for i := 0; i < 6; i++ {
		index, wait, err := b.tryAdmit(context.Background(), "tavily", keys, nil, rule)
		if err != nil || wait != 0 || index < 0 {
			t.Fatalf("per-key admission %d: %d %s %v", i, index, wait, err)
		}
		counts[index]++
	}
	if counts != [3]int{2, 2, 2} {
		t.Fatalf("per-key quota or key balance wrong: %v", counts)
	}
	if index, wait, err := b.tryAdmit(context.Background(), "tavily", keys, nil, rule); err != nil || index >= 0 || wait < 59*time.Second {
		t.Fatalf("did not enforce all three key windows: %d %s %v", index, wait, err)
	}
	clock = clock.Add(time.Minute + time.Millisecond)
	if index, _, err := b.tryAdmit(context.Background(), "tavily", keys, nil, rule); err != nil || index < 0 {
		t.Fatalf("expired Tavily quota did not refill: %d %v", index, err)
	}
}

func TestSearchAdmissionAtomicUnderAgentBurst(t *testing.T) {
	b := searchKeyBalancer{dir: t.TempDir(), now: time.Now}
	const agents = 32
	rule := searchRateRule{limit: 7, window: 10 * time.Second}
	keys := []string{"key-A", "key-B"}
	var admitted atomic.Int64
	var blocked atomic.Int64
	var wg sync.WaitGroup
	issues := make(chan error, agents)
	for i := 0; i < agents; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			index, wait, err := b.tryAdmit(context.Background(), "exa", keys, nil, rule)
			if err != nil {
				issues <- err
			} else if index >= 0 {
				admitted.Add(1)
			} else if wait > 0 {
				blocked.Add(1)
			} else {
				issues <- errors.New("request neither admitted nor blocked")
			}
		}()
	}
	wg.Wait()
	close(issues)
	for err := range issues {
		t.Fatal(err)
	}
	if admitted.Load() != 14 || blocked.Load() != agents-14 {
		t.Fatalf("quota race: %d admitted, %d blocked", admitted.Load(), blocked.Load())
	}
}

func TestSearchAdmitDeadlineDoesNotHoldUpFederatedResults(t *testing.T) {
	b := searchKeyBalancer{dir: t.TempDir(), now: time.Now}
	rule := searchRateForProvider("exa")
	keys := []string{"one"}
	for i := 0; i < rule.limit; i++ {
		if _, _, err := b.tryAdmit(context.Background(), "exa", keys, nil, rule); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	_, err := b.admit(context.Background(), "exa", keys, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "temporarily at capacity") || time.Since(start) > 300*time.Millisecond {
		t.Fatalf("exhausted provider blocked other searches: %v", err)
	}
}

func TestSearchObservedPlanQPSOverridesDefault(t *testing.T) {
	b := searchKeyBalancer{dir: t.TempDir(), now: time.Now}
	if err := b.observeSearchQPS(context.Background(), "brave", "one", 2); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if index, _, err := b.tryAdmit(context.Background(), "brave", []string{"one", "two"}, nil, searchRateForProvider("brave")); err != nil || index < 0 {
			t.Fatalf("custom limit request %d: %d %v", i, index, err)
		}
	}
	var state searchKeyBalanceState
	data, readErr := os.ReadFile(filepath.Join(b.dir, "brave.json"))
	if readErr != nil || json.Unmarshal(data, &state) != nil || len(state.Admissions[searchKeyID("one")]) != 2 {
		t.Fatalf("custom plan cap not applied solely to its own key: %v %+v", readErr, state)
	}
}

func TestSearchLimiterSubprocessHelper(t *testing.T) {
	if os.Getenv("WEBCTX_SEARCH_LIMITER_TEST_HELPER") != "1" {
		return
	}
	b := currentSearchKeyBalancer()
	index, _, err := b.tryAdmit(context.Background(), "exa", []string{"a", "b"}, nil, searchRateRule{limit: 4, window: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if index >= 0 {
		fmt.Println("WEBCTX_RATE_ADMITTED")
	} else {
		fmt.Println("WEBCTX_RATE_BLOCKED")
	}
}

func TestSearchLimiterQuotaAcrossSubprocesses(t *testing.T) {
	dir := t.TempDir()
	const agents = 12
	var wg sync.WaitGroup
	results := make(chan string, agents)
	for i := 0; i < agents; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSearchLimiterSubprocessHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "WEBCTX_SEARCH_LIMITER_TEST_HELPER=1", "WEBCTX_SEARCH_KEY_BALANCER_DIR="+dir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				results <- fmt.Sprintf("ERROR %v %s", err, output)
			} else if strings.Contains(string(output), "WEBCTX_RATE_ADMITTED") {
				results <- "admitted"
			} else if strings.Contains(string(output), "WEBCTX_RATE_BLOCKED") {
				results <- "blocked"
			} else {
				results <- fmt.Sprintf("ERROR unexpected %s", output)
			}
		}()
	}
	wg.Wait()
	close(results)
	counts := map[string]int{}
	for result := range results {
		counts[result]++
	}
	if counts["admitted"] != 8 || counts["blocked"] != agents-8 {
		t.Fatalf("cross-process admission was not atomic: %v", counts)
	}
	data, err := os.ReadFile(filepath.Join(dir, "exa.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state searchKeyBalanceState
	if err := json.Unmarshal(data, &state); err != nil || len(state.Admissions[searchKeyID("a")]) != 4 || len(state.Admissions[searchKeyID("b")]) != 4 {
		t.Fatalf("bad on-disk rate state: %v %+v", err, state)
	}
}
