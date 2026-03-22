package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type MarkdownResult struct {
	URL      string
	Title    string
	Markdown string
}

const keychainServiceName = "webctx"

var (
	credentialEnvKeys = []string{"BRAVE_API_KEY", "TAVILY_API_KEY", "EXA_API_KEY", "FIRECRAWL_API_KEY"}
	getwdFunc         = os.Getwd
	executableFunc    = os.Executable
	keychainLookup    = lookupKeychainSecret
)

type githubURLInfo struct {
	Owner  string
	Repo   string
	Branch string
	Path   string
	IsFile bool
}

func parseGitHubURL(raw string) *githubURLInfo {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() != "github.com" {
		return nil
	}
	parts := strings.FieldsFunc(strings.TrimPrefix(parsed.Path, "/"), func(r rune) bool { return r == '/' })
	if len(parts) < 2 {
		return nil
	}
	info := &githubURLInfo{Owner: parts[0], Repo: parts[1], IsFile: true}
	if len(parts) == 2 {
		return info
	}
	switch parts[2] {
	case "tree":
		info.IsFile = false
		if len(parts) > 3 {
			info.Branch = parts[3]
		}
		if len(parts) > 4 {
			info.Path = strings.Join(parts[4:], "/")
		}
		return info
	case "blob":
		if len(parts) > 3 {
			info.Branch = parts[3]
		}
		if len(parts) > 4 {
			info.Path = strings.Join(parts[4:], "/")
		}
		return info
	default:
		return nil
	}
}

func convertToRawGitHubURL(info *githubURLInfo) string {
	if info.Path == "" {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/README.md", info.Owner, info.Repo)
	}
	branch := info.Branch
	if branch == "" {
		branch = "HEAD"
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", info.Owner, info.Repo, branch, info.Path)
}

func fetchGitHubRawContent(raw string) (*MarkdownResult, error) {
	info := parseGitHubURL(raw)
	if info == nil || !info.IsFile {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	content, status, err := fetchText(ctx, convertToRawGitHubURL(info))
	if err != nil || status < 200 || status >= 300 {
		if info.Path == "" && status == http.StatusNotFound {
			for _, alt := range []string{"readme.md", "Readme.md", "README"} {
				altURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/%s", info.Owner, info.Repo, alt)
				content, status, err = fetchText(ctx, altURL)
				if err == nil && status >= 200 && status < 300 {
					title := firstHeadingOrFallback(content, info.Owner+"/"+info.Repo)
					return &MarkdownResult{URL: raw, Title: title, Markdown: content}, nil
				}
			}
		}
		return nil, nil
	}
	title := firstHeadingOrFallback(content, fallbackGitHubTitle(info))
	return &MarkdownResult{URL: raw, Title: title, Markdown: content}, nil
}

func checkMarkdownAvailable(raw string) (bool, error) {
	mdURL := raw
	if !strings.HasSuffix(strings.ToLower(mdURL), ".md") {
		mdURL += ".md"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, mdURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	contentLength := resp.ContentLength
	return (strings.Contains(contentType, "markdown") || strings.Contains(contentType, "text/plain")) && contentLength > 50, nil
}

func fetchMarkdownContent(raw string) (*MarkdownResult, error) {
	mdURL := raw
	if !strings.HasSuffix(strings.ToLower(mdURL), ".md") {
		mdURL += ".md"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	content, status, err := fetchText(ctx, mdURL)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("Failed to fetch markdown: %d", status)
	}
	title := firstHeadingOrFallback(content, filepath.Base(raw))
	return &MarkdownResult{URL: raw, Title: title, Markdown: content}, nil
}

func fetchText(ctx context.Context, rawURL string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}

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

func fallbackGitHubTitle(info *githubURLInfo) string {
	if info.Path != "" {
		parts := strings.Split(info.Path, "/")
		return parts[len(parts)-1]
	}
	return info.Owner + "/" + info.Repo
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
