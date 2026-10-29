package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A shared, file-locked queue gives independent webctx processes run by the
// same OS user one Firecrawl admission budget. Only admission uses file locks:
// no global mutex is held while the network call runs. OS locks are released
// automatically when a process exits, including on crashes.
//
// Firecrawl Free (2026): /scrape, /map and /search each allow 10 requests per
// minute, /interact allows 2, and there are 2 concurrent browsers. Distinct
// endpoint groups have separate sliding windows. All scrapes and maps share
// the same two in-flight browser slots. Session-based browser fallbacks hold
// a slot for the lifetime of their browser session.
type firecrawlQueue struct {
	dir    string
	window time.Duration
}

// Tests replace this package variable inside TestMain. Production always uses
// the Firecrawl Free plan's one-minute rolling window.
var firecrawlQueueWindow = time.Minute

type firecrawlPermit struct {
	slot *os.File
}

func (p *firecrawlPermit) release() {
	if p != nil && p.slot != nil {
		unlockQueueFile(p.slot)
		_ = p.slot.Close()
		p.slot = nil
	}
}

func getFirecrawlQueue() *firecrawlQueue {
	dir := strings.TrimSpace(os.Getenv("WEBCTX_FIRECRAWL_QUEUE_DIR"))
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = filepath.Join(os.TempDir(), "webctx-user-"+strconv.Itoa(os.Getuid()))
		}
		// Vercel functions cannot rely on a writable user home. They still get
		// best-effort per-instance coordination in /tmp; the provider is the
		// ultimate team-wide enforcement mechanism across serverless workers.
		if os.Getenv("VERCEL") != "" || os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
			base = filepath.Join(os.TempDir(), "webctx")
		}
		dir = filepath.Join(base, "webctx", "firecrawl-queue")
	}
	return &firecrawlQueue{dir: dir, window: firecrawlQueueWindow}
}

func (q *firecrawlQueue) setup() error {
	if q.dir == "" {
		return fmt.Errorf("Firecrawl queue directory is not configured")
	}
	if err := os.MkdirAll(q.dir, 0700); err != nil {
		return fmt.Errorf("create shared Firecrawl queue: %w", err)
	}
	return nil
}

func firecrawlRequestLimits(method, path string) (group string, requestsPerMinute int, browserSlot bool) {
	switch {
	case method == "DELETE" && strings.HasPrefix(path, "/v2/interact/"):
		// Never throttle browser teardown: it releases scarce remote slots.
		return "", 0, false
	case method == "POST" && path == "/v2/scrape":
		return "scrape", 10, true
	case method == "POST" && path == "/v2/map":
		return "map", 10, true
	case method == "POST" && path == "/v2/search":
		return "search", 10, false
	case method == "POST" && path == "/v2/interact":
		return "interact", 2, false
	case method == "POST" && strings.HasPrefix(path, "/v2/interact/"):
		return "interact-execute", 10, false
	default:
		// Unknown Firecrawl endpoints conservatively share a low allowance.
		return "other", 2, false
	}
}

func (q *firecrawlQueue) acquire(ctx context.Context, group string, maxRequests int, needsSlot bool) (*firecrawlPermit, error) {
	if maxRequests == 0 {
		return &firecrawlPermit{}, nil
	}
	if err := q.setup(); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		permit := &firecrawlPermit{}
		if needsSlot {
			slot, err := q.tryBrowserSlot()
			if err != nil {
				return nil, err
			}
			if slot == nil {
				if err := waitForFirecrawlSlot(ctx); err != nil {
					return nil, err
				}
				continue
			}
			permit.slot = slot
		}
		delay, err := q.reserveRateToken(group, maxRequests)
		if err != nil {
			permit.release()
			return nil, err
		}
		if delay == 0 {
			return permit, nil
		}
		// Never occupy a browser slot while waiting for the minute quota to
		// refill. Another endpoint may use that slot in the meantime.
		permit.release()
		if err := waitFirecrawl(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func (q *firecrawlQueue) browserSession(ctx context.Context) (*firecrawlPermit, error) {
	if err := q.setup(); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		slot, err := q.tryBrowserSlot()
		if err != nil {
			return nil, err
		}
		if slot != nil {
			return &firecrawlPermit{slot: slot}, nil
		}
		if err := waitForFirecrawlSlot(ctx); err != nil {
			return nil, err
		}
	}
}

func (q *firecrawlQueue) tryBrowserSlot() (*os.File, error) {
	for index := 0; index < 2; index++ {
		file, locked, err := tryOpenQueueLock(filepath.Join(q.dir, fmt.Sprintf("browser-%d.lock", index)))
		if err != nil {
			return nil, err
		}
		if locked {
			return file, nil
		}
	}
	return nil, nil
}

// Ten token files, one per minute allowance, enforce a strict rolling 60s
// window across every process on the machine. A token's last-use time is
// persistent; crashes cannot reset a burst of requests. The per-token lock is
// held only for the few milliseconds needed to claim an available token.
func (q *firecrawlQueue) reserveRateToken(group string, maxRequests int) (time.Duration, error) {
	now := time.Now()
	window := q.window
	if window <= 0 {
		window = time.Minute
	}
	var earliest time.Time
	locked := false
	for index := 0; index < maxRequests; index++ {
		name := filepath.Join(q.dir, fmt.Sprintf("rate-%s-%02d.lock", group, index))
		file, acquired, err := tryOpenQueueLock(name)
		if err != nil {
			return 0, err
		}
		if !acquired {
			locked = true
			continue
		}
		stamp, err := io.ReadAll(io.LimitReader(file, 48))
		if err != nil {
			unlockQueueFile(file)
			_ = file.Close()
			return 0, err
		}
		last := time.Time{}
		if len(stamp) > 0 {
			ms, err := strconv.ParseInt(strings.TrimSpace(string(stamp)), 10, 64)
			if err != nil {
				unlockQueueFile(file)
				_ = file.Close()
				return 0, fmt.Errorf("corrupt Firecrawl queue token %s: %w", name, err)
			}
			last = time.UnixMilli(ms)
		}
		next := last.Add(window)
		if now.Before(next) {
			if earliest.IsZero() || next.Before(earliest) {
				earliest = next
			}
			unlockQueueFile(file)
			_ = file.Close()
			continue
		}
		if err := file.Truncate(0); err == nil {
			_, err = file.WriteAt([]byte(strconv.FormatInt(now.UnixMilli(), 10)), 0)
		}
		if err == nil {
			err = file.Sync()
		}
		unlockQueueFile(file)
		_ = file.Close()
		return 0, err
	}
	if locked {
		// A locked token might become available as soon as another caller
		// finishes claiming it. Check again instead of sleeping a full minute.
		return 30 * time.Millisecond, nil
	}
	if earliest.IsZero() {
		return 100 * time.Millisecond, nil
	}
	delay := time.Until(earliest) + 10*time.Millisecond
	if delay < 10*time.Millisecond {
		return 10 * time.Millisecond, nil
	}
	return delay, nil
}

func tryOpenQueueLock(path string) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false, err
	}
	acquired, err := tryLockQueueFile(file)
	if err != nil || !acquired {
		_ = file.Close()
		return nil, false, err
	}
	return file, true, nil
}

func waitForFirecrawlSlot(ctx context.Context) error {
	return waitFirecrawl(ctx, 65*time.Millisecond)
}

func waitFirecrawl(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
