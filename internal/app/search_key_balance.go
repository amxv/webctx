package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Search providers independently rotate their configured API keys. A small
// per-provider state file and OS file lock coordinate the starting key across
// CLI invocations, including multiple agents running at the same time.
// The files contain hashes of key identifiers, NEVER secret values.
type searchKeyBalancer struct {
	dir string
	now func() time.Time
}

type searchKeyBalanceState struct {
	LastSelected string           `json:"last_selected,omitempty"`
	Cooldowns    map[string]int64 `json:"cooldowns,omitempty"`
}

func currentSearchKeyBalancer() searchKeyBalancer {
	dir := strings.TrimSpace(os.Getenv("WEBCTX_SEARCH_KEY_BALANCER_DIR"))
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = filepath.Join(os.TempDir(), "webctx-user-"+strconv.Itoa(os.Getuid()))
		}
		dir = filepath.Join(base, "webctx", "search-keys")
	}
	return searchKeyBalancer{dir: dir, now: time.Now}
}

func searchKeyID(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

// reserve selects a healthy starting credential and persists the selection
// before returning. The other healthy keys form the fallback order. The
// process holds no file lock while contacting a provider.
func (b searchKeyBalancer) reserve(ctx context.Context, provider string, keys []string) ([]int, error) {
	if len(keys) == 0 {
		return nil, errors.New("no keys configured")
	}
	if len(keys) == 1 {
		return []int{0}, nil
	}
	ids := make([]string, len(keys))
	for i, key := range keys {
		ids[i] = searchKeyID(key)
	}
	var order []int
	err := b.withState(ctx, provider, func(state *searchKeyBalanceState) (bool, error) {
		now := b.now().UnixMilli()
		start := 0
		for i, id := range ids {
			if id == state.LastSelected {
				start = (i + 1) % len(keys)
				break
			}
		}
		var soonest int64
		for step := 0; step < len(keys); step++ {
			index := (start + step) % len(keys)
			if until := state.Cooldowns[ids[index]]; until > now {
				if soonest == 0 || until < soonest {
					soonest = until
				}
				continue
			}
			order = append(order, index)
		}
		if len(order) == 0 {
			return false, fmt.Errorf("%s: all %d configured search API keys are temporarily unavailable; retry in %s", provider, len(keys), (time.Duration(soonest-now) * time.Millisecond).Round(time.Second))
		}
		state.LastSelected = ids[order[0]]
		return true, nil
	})
	return order, err
}

// available checks fresh shared cooldown state before a fallback attempt. A
// different process may have just discovered that this key is rate-limited.
func (b searchKeyBalancer) available(ctx context.Context, provider, key string) (bool, error) {
	available := false
	err := b.withState(ctx, provider, func(state *searchKeyBalanceState) (bool, error) {
		available = state.Cooldowns[searchKeyID(key)] <= b.now().UnixMilli()
		return false, nil
	})
	return available, err
}

func (b searchKeyBalancer) coolDown(ctx context.Context, provider, key string, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	until := b.now().Add(duration).UnixMilli()
	return b.withState(ctx, provider, func(state *searchKeyBalanceState) (bool, error) {
		id := searchKeyID(key)
		if state.Cooldowns == nil {
			state.Cooldowns = make(map[string]int64)
		}
		if until > state.Cooldowns[id] {
			state.Cooldowns[id] = until
			return true, nil
		}
		return false, nil
	})
}

// withState serializes a tiny read/modify/atomic-replace transaction. The OS
// lock is separate from the JSON state file for Windows rename compatibility.
func (b searchKeyBalancer) withState(ctx context.Context, provider string, update func(*searchKeyBalanceState) (bool, error)) error {
	if !validSearchProvider(provider) {
		return fmt.Errorf("unknown search provider %q", provider)
	}
	if err := os.MkdirAll(b.dir, 0700); err != nil {
		return fmt.Errorf("create search key rotation directory: %w", err)
	}
	var lock *os.File
	lockPath := filepath.Join(b.dir, provider+".lock")
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, acquired, err := tryOpenQueueLock(lockPath)
		if err != nil {
			return fmt.Errorf("lock search key rotation state: %w", err)
		}
		if acquired {
			lock = file
			break
		}
		if err := waitFirecrawl(ctx, 15*time.Millisecond); err != nil {
			return err
		}
	}
	defer func() { unlockQueueFile(lock); _ = lock.Close() }()

	statePath := filepath.Join(b.dir, provider+".json")
	raw, err := os.Open(statePath)
	state := searchKeyBalanceState{}
	if err == nil {
		contents, readErr := io.ReadAll(io.LimitReader(raw, 1024*1024+1))
		_ = raw.Close()
		if readErr != nil {
			return fmt.Errorf("read search key rotation state: %w", readErr)
		}
		if len(contents) > 1024*1024 || json.Unmarshal(contents, &state) != nil {
			return fmt.Errorf("search key rotation state for %s is invalid", provider)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("open search key rotation state: %w", err)
	}
	changed := false
	for id, until := range state.Cooldowns {
		if until <= b.now().UnixMilli() {
			delete(state.Cooldowns, id)
			changed = true
		}
	}
	updated, err := update(&state)
	if err != nil {
		return err
	}
	if !changed && !updated {
		return nil
	}
	contents, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode search key rotation state: %w", err)
	}
	file, err := os.CreateTemp(b.dir, provider+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), statePath); err != nil {
		return fmt.Errorf("save search key rotation state: %w", err)
	}
	return nil
}

// Cooldowns avoid repeatedly hitting exhausted/invalid credentials on every
// search. A retryable failure still immediately tries another usable key.
// Retry-After from providers overrides the generic short rate-limit window.
func searchKeyFailureCooldown(err error) time.Duration {
	var status *httpStatusError
	if errors.As(err, &status) {
		switch code := status.Code; {
		case code == 401 || code == 403 || (code == 422 && strings.Contains(status.Message, "SUBSCRIPTION_TOKEN_INVALID")):
			return 30 * time.Minute
		case code == 402 || code == 432:
			return 15 * time.Minute
		case code == 429:
			if status.RetryAfter > 0 {
				return status.RetryAfter
			}
			return time.Minute
		case code == 408 || code >= 500:
			return 15 * time.Second
		}
	}
	return 15 * time.Second
}
