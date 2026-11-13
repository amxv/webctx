package app

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A shared, file-locked queue gives independent webctx processes run by the
// same OS user one Firecrawl admission budget PER ACCOUNT KEY. Only admission uses file locks:
// no global mutex is held while the network call runs. OS locks are released
// automatically when a process exits, including on crashes.
//
// Request limits and browser capacity follow WEBCTX_FIRECRAWL_PLAN.
// Distinct endpoint groups have separate bounded sliding windows. Scrapes and maps
// within one account share its configured in-flight browser slots. Browser sessions hold
// a slot for the lifetime of their browser session.
type firecrawlQueue struct {
	dir    string
	window time.Duration
	slots  int
}

// Tests replace this package variable inside TestMain. Production uses
// the provider's one-minute rolling window, regardless of plan.
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
	_, plan := activeFirecrawlPlan()
	return &firecrawlQueue{dir: dir, window: firecrawlQueueWindow, slots: plan.browserSlots}
}

// The user configures credentials belonging to separate Firecrawl teams.
// Tokens, upstream cooldowns and browser sessions are therefore isolated by
// credential identity. Hashes avoid persisting credentials or leaking them in
// paths, while separate CLI processes with the same credential share a pool.
func (q *firecrawlQueue) forKey(key string) *firecrawlQueue {
	return &firecrawlQueue{
		dir:    filepath.Join(q.dir, "accounts", searchKeyID(key)),
		window: q.window,
		slots:  q.slots,
	}
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
	_, plan := activeFirecrawlPlan()
	switch {
	case method == "DELETE" && strings.HasPrefix(path, "/v2/interact/"):
		// Never throttle browser teardown: it releases scarce remote slots.
		return "", 0, false
	case method == "POST" && path == "/v2/scrape":
		return "scrape", plan.scrape, true
	case method == "POST" && path == "/v2/map":
		return "map", plan.mapSite, true
	case method == "POST" && (path == "/v2/crawl" || path == "/v2/batch/scrape"):
		return "crawl", plan.crawl, false
	case method == "GET" && (strings.HasPrefix(path, "/v2/crawl/") || strings.HasPrefix(path, "/v2/batch/scrape/")):
		return "crawl-status", plan.crawlStatus, false
	case method == "DELETE" && (strings.HasPrefix(path, "/v2/crawl/") || strings.HasPrefix(path, "/v2/batch/scrape/")):
		return "", 0, false
	case method == "POST" && path == "/v2/search":
		return "search", plan.search, false
	case method == "POST" && (path == "/v2/agent" || path == "/v2/extract"):
		return "agent", plan.agent, false
	case method == "GET" && (strings.HasPrefix(path, "/v2/agent/") || strings.HasPrefix(path, "/v2/extract/")):
		return "agent-status", plan.agentStatus, false
	case method == "POST" && path == "/v2/interact":
		return "interact", plan.interact, false
	case method == "POST" && strings.HasPrefix(path, "/v2/interact/"):
		return "interact-execute", plan.interactExec, false
	default:
		// Undocumented endpoints have no published per-plan limit; keep
		// their conservative shared allowance rather than inventing one.
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
		permit, delay, err := q.tryAcquire(group, maxRequests, needsSlot)
		if err != nil {
			return nil, err
		}
		if permit != nil {
			return permit, nil
		}
		if err := waitFirecrawl(ctx, delay); err != nil {
			return nil, err
		}
	}
}

// Non-blocking admission lets a caller try another independently provisioned
// account before waiting for a busy key. Never hold a browser slot while
// waiting on quota, and never hold a rate lock during HTTP execution.
func (q *firecrawlQueue) tryAcquire(group string, maxRequests int, needsSlot bool) (*firecrawlPermit, time.Duration, error) {
	if maxRequests == 0 {
		return &firecrawlPermit{}, 0, nil
	}
	if err := q.setup(); err != nil {
		return nil, 0, err
	}
	permit := &firecrawlPermit{}
	if needsSlot {
		slot, err := q.tryBrowserSlot()
		if err != nil {
			return nil, 0, err
		}
		if slot == nil {
			return nil, 65 * time.Millisecond, nil
		}
		permit.slot = slot
	}
	delay, err := q.reserveRateToken(group, maxRequests)
	if err != nil || delay > 0 {
		permit.release()
		return nil, delay, err
	}
	return permit, 0, nil
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
	count := q.slots
	if count <= 0 {
		count = 2
	}
	for index := 0; index < count; index++ {
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

// Keep just 64 time buckets in one lock-protected state file per endpoint,
// regardless of whether the tier permits 10 or 500,000 RPM. This is a
// conservative rolling-window limiter (at most one bucket of over-throttling)
// and avoids scanning/opening thousands of files for high-volume paid plans.
// The on-disk counts survive process exit and plan changes.
const firecrawlRateBuckets = 64
const firecrawlRateMagic = "WFCQ0001"
const firecrawlRateStateSize = 16 + firecrawlRateBuckets*16

func (q *firecrawlQueue) reserveRateToken(group string, maxRequests int) (time.Duration, error) {
	if groupPause, err := q.rateCooldown(group, 0); err != nil || groupPause > 0 {
		return groupPause, err
	}
	if maxRequests <= 0 {
		return 0, nil
	}
	if err := q.setup(); err != nil {
		return 0, err
	}
	window := q.window
	if window <= 0 {
		window = time.Minute
	}
	width := window.Nanoseconds() / 60
	if width < 1 {
		width = 1
	}
	name := filepath.Join(q.dir, "rate-"+group+".state")
	var file *os.File
	// A momentarily held state-file lock is not an exhausted quota. Retry
	// briefly before returning a wait hint to callers selecting other keys.
	for attempt := 0; attempt < 100; attempt++ {
		var acquired bool
		var err error
		file, acquired, err = tryOpenQueueLock(name)
		if err != nil {
			return 0, err
		}
		if acquired {
			break
		}
		if attempt == 99 {
			return 20 * time.Millisecond, nil
		}
		time.Sleep(time.Millisecond)
	}
	defer func() { unlockQueueFile(file); _ = file.Close() }()
	var data [firecrawlRateStateSize]byte
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if stat.Size() != 0 {
		if stat.Size() != int64(len(data)) {
			return 0, fmt.Errorf("invalid Firecrawl rate state size for %s", group)
		}
		if _, err := file.ReadAt(data[:], 0); err != nil {
			return 0, err
		}
		if string(data[:8]) != firecrawlRateMagic || int64(binary.LittleEndian.Uint64(data[8:16])) != width {
			return 0, fmt.Errorf("incompatible Firecrawl rate state for %s", group)
		}
	} else {
		copy(data[:8], firecrawlRateMagic)
		binary.LittleEndian.PutUint64(data[8:16], uint64(width))
	}
	now := time.Now()
	tick := now.UnixNano() / width
	oldest := now.Add(-window).UnixNano() / width
	var total uint64
	var earliest int64
	for i := 0; i < firecrawlRateBuckets; i++ {
		offset := 16 + i*16
		epoch := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
		if epoch < oldest || epoch > tick {
			continue
		}
		count := binary.LittleEndian.Uint64(data[offset+8 : offset+16])
		total += count
		if count > 0 && (earliest == 0 || epoch < earliest) {
			earliest = epoch
		}
	}
	if total >= uint64(maxRequests) {
		// A bucket may contain requests just before the window boundary.
		// Only release the bucket after its entire interval expires.
		next := time.Unix(0, (earliest+1)*width+window.Nanoseconds())
		wait := time.Until(next) + time.Millisecond
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		return wait, nil
	}
	offset := 16 + int(tick%firecrawlRateBuckets)*16
	previous := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	count := uint64(0)
	if previous == tick {
		count = binary.LittleEndian.Uint64(data[offset+8 : offset+16])
	}
	binary.LittleEndian.PutUint64(data[offset:offset+8], uint64(tick))
	binary.LittleEndian.PutUint64(data[offset+8:offset+16], count+1)
	if _, err := file.WriteAt(data[:], 0); err != nil {
		return 0, err
	}
	return 0, file.Sync()
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
