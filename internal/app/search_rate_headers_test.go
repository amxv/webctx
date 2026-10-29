package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBravePlanPolicyAndResetHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Policy", "2;w=1, 1000;w=2592000")
	h.Set("X-RateLimit-Remaining", "0, 8")
	h.Set("X-RateLimit-Reset", "0.75, 1420300")
	if qps := braveSearchQPS(h); qps != 2 {
		t.Fatalf("unexpected Brave QPS: %d", qps)
	}
	if retry := braveSearchRetryAfter(h); retry != 750*time.Millisecond {
		t.Fatalf("ignored sliding-window reset: %s", retry)
	}
	h.Set("X-RateLimit-Remaining", "0, 0")
	if retry := braveSearchRetryAfter(h); retry != 24*time.Hour {
		t.Fatalf("ignored exhausted monthly quota: %s", retry)
	}
	h.Set("X-RateLimit-Policy", "1000;w=2592000")
	if braveSearchQPS(h) != 0 {
		t.Fatal("incorrectly interpreted monthly quota as QPS")
	}
}

func TestBraveObservedLowerPlanQuotaIsShared(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", dir)
	header := http.Header{}
	header.Set("X-RateLimit-Policy", "1;w=1, 1000;w=2592000")
	observeBraveSearchQPS(header, "A")
	state := currentSearchKeyBalancer()
	index, _, err := state.tryAdmit(context.Background(), "brave", []string{"A", "B"}, nil, searchRateForProvider("brave"))
	if err != nil || index < 0 {
		t.Fatalf("first observed-plan request: %d %v", index, err)
	}
	index, wait, err := state.tryAdmit(context.Background(), "brave", []string{"A", "B"}, nil, searchRateForProvider("brave"))
	if err != nil || index != 1 || wait != 0 {
		t.Fatalf("separate Brave account must have independent capacity: %d %s %v", index, wait, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "brave.json")); err != nil {
		t.Fatal(err)
	}
}
