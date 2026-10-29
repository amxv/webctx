package app

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFirecrawlQueueTwoConcurrentSlots(t *testing.T) {
	q := &firecrawlQueue{dir: t.TempDir(), window: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a, err := q.acquire(ctx, "scrape", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.release()
	b, err := q.acquire(ctx, "scrape", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	defer b.release()
	third, thirdCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer thirdCancel()
	if permit, err := q.acquire(third, "scrape", 10, true); !errors.Is(err, context.DeadlineExceeded) {
		if permit != nil {
			permit.release()
		}
		t.Fatalf("third request entered two-slot browser pool: %v", err)
	}
	a.release()
	c, err := q.acquire(ctx, "scrape", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	c.release()
}

func TestFirecrawlQueueSharedSlidingWindow(t *testing.T) {
	q := &firecrawlQueue{dir: t.TempDir(), window: 250 * time.Millisecond}
	if err := q.setup(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		wait, err := q.reserveRateToken("scrape", 2)
		if err != nil || wait != 0 {
			t.Fatalf("request %d rejected: wait=%s err=%v", i, wait, err)
		}
	}
	wait, err := q.reserveRateToken("scrape", 2)
	if err != nil || wait <= 0 {
		t.Fatalf("a third request should wait for the rolling window: wait=%s err=%v", wait, err)
	}
	// A separate process/queue instance shares the durable rate state.
	other := &firecrawlQueue{dir: q.dir, window: q.window}
	wait, err = other.reserveRateToken("scrape", 2)
	if err != nil || wait <= 0 {
		t.Fatalf("independent queue bypassed the shared limit: wait=%s err=%v", wait, err)
	}
	// Different documented endpoints have independent per-minute allowances.
	wait, err = other.reserveRateToken("map", 2)
	if err != nil || wait != 0 {
		t.Fatalf("scrape should not exhaust /map: wait=%s err=%v", wait, err)
	}
	if err := waitFirecrawl(context.Background(), waitForWindow(q.window)); err != nil {
		t.Fatal(err)
	}
	wait, err = other.reserveRateToken("scrape", 2)
	if err != nil || wait != 0 {
		t.Fatalf("slot did not refresh after rolling window: wait=%s err=%v", wait, err)
	}
}

func waitForWindow(window time.Duration) time.Duration { return window + 25*time.Millisecond }

func TestFirecrawlRequestClassification(t *testing.T) {
	for _, tc := range []struct {
		method, path, group string
		limit               int
		slot                bool
	}{
		{"POST", "/v2/scrape", "scrape", 10, true},
		{"POST", "/v2/map", "map", 10, true},
		{"POST", "/v2/crawl", "crawl", 2, false},
		{"GET", "/v2/crawl/job-id", "crawl-status", 500, false},
		{"DELETE", "/v2/crawl/job-id", "", 0, false},
		{"POST", "/v2/search", "search", 10, false},
		{"POST", "/v2/interact", "interact", 2, false},
		{"POST", "/v2/interact/abc/execute", "interact-execute", 10, false},
		{"DELETE", "/v2/interact/abc", "", 0, false},
	} {
		group, limit, slot := firecrawlRequestLimits(tc.method, tc.path)
		if group != tc.group || limit != tc.limit || slot != tc.slot {
			t.Errorf("%s %s => %s %d %t, want %+v", tc.method, tc.path, group, limit, slot, tc)
		}
	}
}

func TestFirecrawlUpstream429CooldownSharedButEndpointIsolated(t *testing.T) {
	q := &firecrawlQueue{dir: t.TempDir(), window: time.Second}
	if _, err := q.rateCooldown("scrape", 150*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	other := &firecrawlQueue{dir: q.dir, window: q.window}
	if wait, err := other.reserveRateToken("scrape", 10); err != nil || wait < 50*time.Millisecond {
		t.Fatalf("shared scrape cooldown was bypassed: %s %v", wait, err)
	}
	if wait, err := other.reserveRateToken("map", 10); err != nil || wait != 0 {
		t.Fatalf("scrape cooldown blocked independent map: %s %v", wait, err)
	}
	if _, err := other.rateCooldown("scrape", 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if wait, err := q.reserveRateToken("scrape", 10); err != nil || wait < 50*time.Millisecond {
		t.Fatalf("shorter 429 overwrote longer cooldown: %s %v", wait, err)
	}
	if err := waitFirecrawl(context.Background(), 180*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if wait, err := other.reserveRateToken("scrape", 10); err != nil || wait != 0 {
		t.Fatalf("expired upstream cooldown didn't recover: %s %v", wait, err)
	}
}

// These subprocess helpers use the same test executable but independent OS
// processes. This verifies real flock/LockFileEx behavior, not just goroutines.
func TestFirecrawlSharedQueueHelper(t *testing.T) {
	if os.Getenv("WEBCTX_FIRECRAWL_TEST_HELPER") != "1" {
		return
	}
	q := &firecrawlQueue{dir: os.Getenv("WEBCTX_FIRECRAWL_QUEUE_DIR"), window: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	permit, err := q.browserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer permit.release()
	_, _ = os.Stdout.WriteString("WEBCTX_QUEUE_READY\n")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func TestFirecrawlQueueSharedAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	type child struct {
		command *exec.Cmd
		stdin   *os.File
	}
	var children []child
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			for _, c := range children {
				_, _ = c.stdin.WriteString("done\n")
				_ = c.stdin.Close()
				_ = c.command.Process.Kill()
				_ = c.command.Wait()
			}
		})
	}
	t.Cleanup(cleanup)
	for i := 0; i < 2; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestFirecrawlSharedQueueHelper$", "-test.v")
		cmd.Env = append(os.Environ(), "WEBCTX_FIRECRAWL_TEST_HELPER=1", "WEBCTX_FIRECRAWL_QUEUE_DIR="+dir)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		// The returned stdin is an io.WriteCloser (os.File for subprocess pipes).
		input, ok := stdin.(*os.File)
		if !ok {
			t.Fatal("expected OS pipe")
		}
		children = append(children, child{command: cmd, stdin: input})
		ready := make(chan bool, 1)
		go func() {
			sc := bufio.NewScanner(stdout)
			for sc.Scan() {
				if strings.Contains(sc.Text(), "WEBCTX_QUEUE_READY") {
					ready <- true
					return
				}
			}
			ready <- false
		}()
		select {
		case found := <-ready:
			if !found {
				t.Fatal("child failed to acquire shared browser slot")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("child did not initialize shared queue")
		}
	}
	q := &firecrawlQueue{dir: dir, window: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 140*time.Millisecond)
	defer cancel()
	permit, err := q.browserSession(ctx)
	if permit != nil {
		permit.release()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third process bypassed two held browser slots: %v", err)
	}
	cleanup()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	permit, err = q.browserSession(ctx2)
	if err != nil {
		t.Fatalf("crashed/closed subprocess did not release OS lock: %v", err)
	}
	permit.release()
}
