package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// The published free-plan search quotas (October 2026). Tavily documents its
// development limit per API-key environment. Exa's 10 QPS is shared by the
// team and Brave's 50 QPS by the plan, but configured credentials belong to
// DIFFERENT teams/plans. Each key therefore has its own independent bucket.
// These are sliding windows, not a fixed delay between individual requests:
// healthy agents can burst into unused capacity without extra latency.
type searchRateRule struct {
	limit  int
	window time.Duration
}

func searchRateForProvider(provider string) searchRateRule {
	switch provider {
	case "tavily":
		return searchRateRule{limit: searchRateOverride("WEBCTX_TAVILY_RPM", 100), window: time.Minute}
	case "exa":
		return searchRateRule{limit: searchRateOverride("WEBCTX_EXA_QPS", 10), window: time.Second}
	case "brave":
		return searchRateRule{limit: searchRateOverride("WEBCTX_BRAVE_QPS", 50), window: time.Second}
	default:
		return searchRateRule{}
	}
}

func searchRateOverride(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value < 1 || value > 10000 {
		return fallback
	}
	return value
}

// A short, bounded admission wait keeps one exhausted provider from holding
// up otherwise successful federated searches. Agents needing every provider
// can explicitly increase this budget (up to the search's 40s deadline).
func searchAdmissionBudget() time.Duration {
	ms, err := strconv.Atoi(strings.TrimSpace(os.Getenv("WEBCTX_SEARCH_QUEUE_WAIT_MS")))
	if err == nil && ms >= 0 && ms <= 30000 {
		return time.Duration(ms) * time.Millisecond
	}
	return 1500 * time.Millisecond
}

// tryAdmit atomically chooses one eligible key AND reserves its rate-window
// capacity. A separate WebCTX process cannot claim the same remaining slot.
// Returning a wait duration does not reserve any capacity or rotate a key.
func (b searchKeyBalancer) tryAdmit(ctx context.Context, provider string, keys []string, tried map[string]bool, rule searchRateRule) (int, time.Duration, error) {
	if len(keys) == 0 || rule.limit <= 0 || rule.window <= 0 {
		return -1, 0, fmt.Errorf("%s: no rate-limited search credentials available", provider)
	}
	ids := make([]string, len(keys))
	for i, key := range keys {
		ids[i] = searchKeyID(key)
	}
	selected := -1
	var soonest int64
	var candidateCount int
	err := b.withState(ctx, provider, func(state *searchKeyBalanceState) (bool, error) {
		now := b.now().UnixMilli()
		window := rule.window.Milliseconds()
		changed := false
		// Keep state bounded even as credentials are added and removed.
		for scope, admissions := range state.Admissions {
			cutoff := now - window
			first := 0
			for first < len(admissions) && admissions[first] <= cutoff {
				first++
			}
			if first > 0 {
				if first == len(admissions) {
					delete(state.Admissions, scope)
				} else {
					state.Admissions[scope] = append([]int64(nil), admissions[first:]...)
				}
				changed = true
			}
		}
		start := 0
		for i, id := range ids {
			if id == state.LastSelected {
				start = (i + 1) % len(ids)
				break
			}
		}
		// Never read legacy shared-account state: keys are separate accounts.
		for step := 0; step < len(keys); step++ {
			index := (start + step) % len(keys)
			id := ids[index]
			if tried[id] {
				continue
			}
			candidateCount++
			readyAt := state.Cooldowns[id]
			if readyAt > now {
				if soonest == 0 || readyAt < soonest {
					soonest = readyAt
				}
				continue
			}
			scope := id
			limit := rule.limit
			if provider == "brave" && state.KeyQPS[id] > 0 {
				limit = state.KeyQPS[id]
			}
			admissions := state.Admissions[scope]
			if len(admissions) >= limit {
				next := admissions[len(admissions)-limit] + window
				if soonest == 0 || next < soonest {
					soonest = next
				}
				continue
			}
			selected = index
			state.LastSelected = id
			if state.Admissions == nil {
				state.Admissions = make(map[string][]int64)
			}
			state.Admissions[scope] = append(admissions, now)
			return true, nil
		}
		return changed, nil
	})
	if err != nil {
		return -1, 0, err
	}
	if selected >= 0 {
		return selected, 0, nil
	}
	if candidateCount == 0 {
		return -1, 0, fmt.Errorf("%s: no untried search API keys remain", provider)
	}
	if soonest == 0 {
		return -1, 0, fmt.Errorf("%s: no search capacity available", provider)
	}
	return -1, time.Duration(max(1, soonest-b.now().UnixMilli())) * time.Millisecond, nil
}

func (b searchKeyBalancer) admit(ctx context.Context, provider string, keys []string, tried map[string]bool, budget time.Duration) (int, error) {
	rule := searchRateForProvider(provider)
	deadline := time.Now().Add(budget)
	for {
		if err := ctx.Err(); err != nil {
			return -1, err
		}
		index, wait, err := b.tryAdmit(ctx, provider, keys, tried, rule)
		if err != nil {
			return -1, err
		}
		if index >= 0 {
			return index, nil
		}
		if wait > time.Until(deadline) {
			return -1, fmt.Errorf("%s: all configured search keys are temporarily at capacity; retry in %s", provider, wait.Round(time.Millisecond))
		}
		if err := waitFirecrawl(ctx, wait+time.Millisecond); err != nil {
			return -1, err
		}
	}
}

// Brave publishes the effective per-plan 1-second cap in every response.
// Learn custom (or older) plan limits instead of blindly assuming 50 QPS.
func (b searchKeyBalancer) observeSearchQPS(ctx context.Context, provider, key string, qps int) error {
	if qps < 1 || qps > 10000 {
		return nil
	}
	id := searchKeyID(key)
	return b.withState(ctx, provider, func(state *searchKeyBalanceState) (bool, error) {
		if state.KeyQPS[id] == qps {
			return false, nil
		}
		if state.KeyQPS == nil {
			state.KeyQPS = make(map[string]int)
		}
		state.KeyQPS[id] = qps
		return true, nil
	})
}
