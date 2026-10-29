package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Only stateless Firecrawl operations can change accounts on retry. Crawl job
// polling and interact session commands MUST remain on their original account;
// charged Alexandria executions are deliberately handled outside this path.
func pooledFirecrawlEndpoint(req *http.Request) bool {
	if req.URL.Hostname() != "api.firecrawl.dev" || req.Method != http.MethodPost {
		return false
	}
	switch req.URL.Path {
	case "/v2/scrape", "/v2/map", "/v2/search", "/v2/crawl":
		return true
	}
	return false
}

func firecrawlRequestCredential(req *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
}

// Selects a key with available account-specific quota and (where required)
// a browser slot. No quota is wasted on accounts already at capacity. The
// short balancer lock coordinates key ordering across independent processes;
// per-key rate/slot file locks provide the actual capacity guarantee.
func firecrawlAccountPermit(ctx context.Context, keys []string, tried map[string]bool, group string, limit int, browser bool) (string, *firecrawlPermit, error) {
	balancer := currentSearchKeyBalancer()
	root := getFirecrawlQueue()
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		order, err := balancer.reserve(ctx, "firecrawl", keys)
		if err != nil {
			return "", nil, err
		}
		var wait time.Duration
		var eligible bool
		for _, index := range order {
			key := keys[index]
			if tried[searchKeyID(key)] {
				continue
			}
			eligible = true
			// A different agent could have cooled this account down since
			// reserve() returned its order.
			ready, err := balancer.available(ctx, "firecrawl", key)
			if err != nil {
				return "", nil, err
			}
			if !ready {
				continue
			}
			permit, delay, err := root.forKey(key).tryAcquire(group, limit, browser)
			if err != nil {
				return "", nil, err
			}
			if permit != nil {
				return key, permit, nil
			}
			if wait == 0 || delay < wait {
				wait = delay
			}
		}
		if !eligible {
			return "", nil, errors.New("firecrawl: all configured accounts have been attempted")
		}
		if wait <= 0 {
			wait = 50 * time.Millisecond
		}
		if err := waitFirecrawl(ctx, wait+time.Millisecond); err != nil {
			return "", nil, err
		}
	}
}

// Stateless page reads may safely try a different independently provisioned
// account on an explicit rejection. Do not retry invalid input or ambiguous
// transport failures that might have already incurred a provider charge.
func doPooledFirecrawlRequest(ctx context.Context, original *http.Request, keys []string) ([]byte, error) {
	group, limit, browser := firecrawlRequestLimits(original.Method, original.URL.Path)
	tried := make(map[string]bool, len(keys))
	var lastErr error
	for len(tried) < len(keys) {
		key, permit, err := firecrawlAccountPermit(ctx, keys, tried, group, limit, browser)
		if err != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("firecrawl: no more accounts with capacity: %w (last upstream error: %v)", err, lastErr)
			}
			return nil, fmt.Errorf("firecrawl: account admission: %w", err)
		}
		tried[searchKeyID(key)] = true
		req := original.Clone(ctx)
		if original.GetBody != nil {
			req.Body, err = original.GetBody()
			if err != nil {
				permit.release()
				return nil, err
			}
		}
		req.Header.Set("Authorization", "Bearer "+key)
		body, requestErr := executeRawRequest(req, permit)
		if requestErr == nil {
			return body, nil
		}
		lastErr = redactSearchError(requestErr, keys)
		var status *httpStatusError
		if !errors.As(requestErr, &status) || !firecrawlRetryableStatus(status.Code, original) {
			return nil, lastErr
		}
		if status.Code != 429 {
			// HTTP transport already records 429 on the specific key+endpoint
			// queue. Do not disable map/crawl because scrape ran out of RPM.
			if err := currentSearchKeyBalancer().coolDown(ctx, "firecrawl", key, firecrawlAccountCooldown(status)); err != nil {
				return nil, fmt.Errorf("firecrawl: record failed account state: %w", err)
			}
		}
	}
	return nil, fmt.Errorf("firecrawl: all %d configured accounts failed: %w", len(keys), lastErr)
}

func firecrawlRetryableStatus(status int, req *http.Request) bool {
	if status == http.StatusForbidden && req.URL.Path == "/v2/scrape" {
		// Domain-tools permission failures must preserve the existing same-
		// account retry with domainTools=false before considering other accounts.
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err == nil {
				defer body.Close()
				var request struct {
					DomainTools bool `json:"domainTools"`
				}
				if json.NewDecoder(body).Decode(&request) == nil && request.DomainTools {
					return false
				}
			}
		}
	}
	// Never retry uncertain 5xx/transport failures across accounts: Firecrawl
	// may have accepted a billed scrape or queued a crawl despite that error.
	return status == 401 || status == 402 || status == 403 || status == 429 || status == 432
}

func firecrawlAccountCooldown(status *httpStatusError) time.Duration {
	if status.Code == 403 {
		// Some sites or API capabilities return 403 even with valid keys.
		// Don't ban a healthy account for 30 minutes over one such request.
		return 15 * time.Second
	}
	return searchKeyFailureCooldown(status)
}

func firecrawlBrowserAccount(ctx context.Context) (string, *firecrawlPermit, error) {
	return firecrawlBrowserAccountExcluding(ctx, nil)
}

func firecrawlBrowserAccountExcluding(ctx context.Context, tried map[string]bool) (string, *firecrawlPermit, error) {
	keys := firecrawlKeys()
	if len(keys) == 0 {
		return "", nil, errors.New("missing FIRECRAWL_API_KEY or FIRECRAWL_API_KEYS")
	}
	balancer := currentSearchKeyBalancer()
	root := getFirecrawlQueue()
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		order, err := balancer.reserve(ctx, "firecrawl", keys)
		if err != nil {
			return "", nil, err
		}
		var wait time.Duration
		var eligible bool
		for _, index := range order {
			key := keys[index]
			if tried[searchKeyID(key)] {
				continue
			}
			eligible = true
			ready, err := balancer.available(ctx, "firecrawl", key)
			if err != nil {
				return "", nil, err
			}
			if !ready {
				continue
			}
			permit, delay, err := root.forKey(key).tryAcquire("interact", 2, true)
			if err != nil {
				return "", nil, err
			}
			if permit != nil {
				return key, permit, nil
			}
			if wait == 0 || delay < wait {
				wait = delay
			}
		}
		if !eligible {
			return "", nil, errors.New("firecrawl: all browser accounts have been attempted")
		}
		if wait == 0 {
			wait = 65 * time.Millisecond
		}
		if err := waitFirecrawl(ctx, wait+time.Millisecond); err != nil {
			return "", nil, err
		}
	}
}

// An upstream rejection during browser creation can be tried on another
// independently provisioned account. Once the session exists, all commands
// and teardown are strictly pinned to that same account.
func startFirecrawlBrowser(ctx context.Context) (string, map[string]any, *firecrawlPermit, error) {
	tried := make(map[string]bool)
	var lastErr error
	for len(tried) < len(firecrawlKeys()) {
		key, permit, err := firecrawlBrowserAccountExcluding(ctx, tried)
		if err != nil {
			return "", nil, nil, err
		}
		tried[searchKeyID(key)] = true
		session, err := firecrawlJSONWithPermit(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/interact", key,
			map[string]any{"ttl": 60, "activityTtl": 30}, &firecrawlPermit{})
		if err == nil {
			return key, session, permit, nil
		}
		permit.release()
		lastErr = redactSearchError(err, firecrawlKeys())
		var status *httpStatusError
		if !errors.As(err, &status) || (status.Code != 401 && status.Code != 402 && status.Code != 403 && status.Code != 429 && status.Code != 432) {
			return "", nil, nil, lastErr
		}
		if status.Code != 429 {
			if err := currentSearchKeyBalancer().coolDown(ctx, "firecrawl", key, firecrawlAccountCooldown(status)); err != nil {
				return "", nil, nil, err
			}
		}
	}
	return "", nil, nil, fmt.Errorf("firecrawl: no browser accounts succeeded: %w", lastErr)
}
