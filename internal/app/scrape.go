package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type MarkdownResult struct {
	URL       string
	SourceURL string
	Title     string
	Markdown  string
}

const keychainServiceName = "webctx"

var (
	credentialEnvKeys = []string{"BRAVE_API_KEY", "TAVILY_API_KEY", "EXA_API_KEY", "FIRECRAWL_API_KEY", "GH_TOKEN", "GITHUB_TOKEN"}
	getwdFunc         = os.Getwd
	executableFunc    = os.Executable
	keychainLookup    = lookupKeychainSecret
)

func firstHeadingOrFallback(markdown, fallback string) string {
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	if strings.TrimSpace(fallback) == "" {
		return "Document"
	}
	return fallback
}

type tokenBucketRateLimiter struct {
	mu             sync.Mutex
	tokens         int
	maxTokens      int
	refillRate     int
	refillInterval time.Duration
	lastRefill     time.Time
}

func newFirecrawlRateLimiter() *tokenBucketRateLimiter {
	return &tokenBucketRateLimiter{tokens: 10, maxTokens: 10, refillRate: 1, refillInterval: 6 * time.Second, lastRefill: time.Now()}
}

func (r *tokenBucketRateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	if elapsed < r.refillInterval {
		return
	}
	intervals := int(elapsed / r.refillInterval)
	if intervals <= 0 {
		return
	}
	r.tokens += intervals * r.refillRate
	if r.tokens > r.maxTokens {
		r.tokens = r.maxTokens
	}
	r.lastRefill = r.lastRefill.Add(time.Duration(intervals) * r.refillInterval)
}

func (r *tokenBucketRateLimiter) acquire(ctx context.Context) error {
	for {
		r.mu.Lock()
		r.refill()
		if r.tokens > 0 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}
		wait := r.refillInterval - time.Since(r.lastRefill) + 100*time.Millisecond
		r.mu.Unlock()
		if wait < 100*time.Millisecond {
			wait = 100 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

type firecrawlQueue struct {
	rateLimiter *tokenBucketRateLimiter
	mu          sync.Mutex
}

var (
	queueOnce sync.Once
	queueInst *firecrawlQueue
)

func getFirecrawlQueue() *firecrawlQueue {
	queueOnce.Do(func() {
		queueInst = &firecrawlQueue{rateLimiter: newFirecrawlRateLimiter()}
	})
	return queueInst
}

func (q *firecrawlQueue) enqueue(_ string, requestFn func() (map[string]any, error)) (map[string]any, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := q.rateLimiter.acquire(ctx); err != nil {
		return nil, err
	}
	return requestFn()
}

func loadEnvLocal() {
	for _, candidate := range envLocalCandidates() {
		loadDotEnvFile(candidate)
	}
	loadKeychainEnv()
}

func envLocalCandidates() []string {
	candidates := []string{}
	if exe, err := executableFunc(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, ".env.local"), filepath.Join(filepath.Dir(exeDir), ".env.local"))
	}
	if cwd, err := getwdFunc(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".env.local"))
	}
	seen := map[string]struct{}{}
	unique := []string{}
	for _, c := range candidates {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		unique = append(unique, c)
	}
	return unique
}

func loadDotEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
			_ = os.Setenv(key, value)
		}
	}
}

func loadKeychainEnv() {
	for _, key := range credentialEnvKeys {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value, err := keychainLookup(key)
		if err != nil || strings.TrimSpace(value) == "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func lookupKeychainSecret(account string) (string, error) {
	if runtime.GOOS != "darwin" || strings.TrimSpace(account) == "" {
		return "", nil
	}

	out, err := exec.Command("security", "find-generic-password", "-s", keychainServiceName, "-a", account, "-w").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
