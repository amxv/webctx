package app

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var observedSearchRate sync.Map

// Brave's policy includes both the 1-second burst limit and monthly quota.
// Only the 1-second window belongs in the local admission scheduler.
func braveSearchQPS(headers http.Header) int {
	for _, item := range strings.Split(headers.Get("X-RateLimit-Policy"), ",") {
		parts := strings.Split(strings.TrimSpace(item), ";")
		if len(parts) < 2 {
			continue
		}
		window := false
		for _, part := range parts[1:] {
			if strings.TrimSpace(part) == "w=1" {
				window = true
			}
		}
		if window {
			if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && n > 0 && n <= 10000 {
				return n
			}
		}
	}
	return 0
}

func observeBraveSearchQPS(headers http.Header, key string) {
	qps := braveSearchQPS(headers)
	if qps == 0 {
		return
	}
	balancer := currentSearchKeyBalancer()
	cacheKey := balancer.dir + ":brave:" + searchKeyID(key)
	if previous, ok := observedSearchRate.Load(cacheKey); ok && previous == qps {
		return
	}
	if balancer.observeSearchQPS(context.Background(), "brave", key, qps) == nil {
		observedSearchRate.Store(cacheKey, qps)
	}
}

// Brave reports a reset for each window, including monthly quota. If the
// one-second window is full, honor its reset; if the month is also empty,
// honor the longer reset for this account (not for other configured keys).
func braveSearchRetryAfter(headers http.Header) time.Duration {
	remaining := strings.Split(headers.Get("X-RateLimit-Remaining"), ",")
	resets := strings.Split(headers.Get("X-RateLimit-Reset"), ",")
	var longest time.Duration
	for i := 0; i < len(remaining) && i < len(resets); i++ {
		left, errLeft := strconv.ParseInt(strings.TrimSpace(remaining[i]), 10, 64)
		seconds, errReset := strconv.ParseFloat(strings.TrimSpace(resets[i]), 64)
		if errLeft != nil || errReset != nil || left > 0 || seconds <= 0 {
			continue
		}
		d := time.Duration(math.Min(seconds, 86400) * float64(time.Second))
		if d > longest {
			longest = d
		}
	}
	return longest
}
