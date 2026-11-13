package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFirecrawlAllPublishedPlansAndEndpoints(t *testing.T) {
	t.Setenv(firecrawlPlanEnv, "free")
	for _, tc := range []struct {
		name          string
		scrape, crawl int
		status        int
		browser, exec int
		slots         int
	}{
		{"free", 10, 2, 500, 2, 10, 2},
		{"hobby", 100, 20, 5000, 20, 100, 5},
		{"standard", 500, 100, 25000, 100, 500, 25},
		{"growth", 5000, 1000, 250000, 1000, 5000, 50},
		{"scale", 10000, 2000, 500000, 1500, 7500, 100},
	} {
		if err := os.Setenv(firecrawlPlanEnv, strings.ToUpper(tc.name)); err != nil {
			t.Fatal(err)
		}
		name, limits := activeFirecrawlPlan()
		if name != tc.name || limits.browserSlots != tc.slots || getFirecrawlQueue().slots != tc.slots {
			t.Fatalf("%q: unexpected plan/browsers: %q %+v", tc.name, name, limits)
		}
		for _, endpoint := range []struct {
			method, path, group string
			rpm                 int
			browser             bool
		}{
			{"POST", "/v2/scrape", "scrape", tc.scrape, true},
			{"POST", "/v2/map", "map", tc.scrape, true},
			{"POST", "/v2/search", "search", tc.scrape, false},
			{"POST", "/v2/crawl", "crawl", tc.crawl, false},
			{"POST", "/v2/batch/scrape", "crawl", tc.crawl, false},
			{"GET", "/v2/crawl/job", "crawl-status", tc.status, false},
			{"GET", "/v2/batch/scrape/job", "crawl-status", tc.status, false},
			{"POST", "/v2/agent", "agent", tc.crawl, false},
			{"POST", "/v2/extract", "agent", tc.crawl, false},
			{"GET", "/v2/agent/job", "agent-status", tc.status, false},
			{"GET", "/v2/extract/job", "agent-status", tc.status, false},
			{"POST", "/v2/interact", "interact", tc.browser, false},
			{"POST", "/v2/interact/session/execute", "interact-execute", tc.exec, false},
			{"DELETE", "/v2/interact/session", "", 0, false},
			{"DELETE", "/v2/crawl/job", "", 0, false},
			{"DELETE", "/v2/batch/scrape/job", "", 0, false},
		} {
			group, rpm, slot := firecrawlRequestLimits(endpoint.method, endpoint.path)
			if group != endpoint.group || rpm != endpoint.rpm || slot != endpoint.browser {
				t.Errorf("%s %s %s: got %s/%d/%t, want %s/%d/%t", tc.name, endpoint.method, endpoint.path,
					group, rpm, slot, endpoint.group, endpoint.rpm, endpoint.browser)
			}
		}
	}
}

func TestFirecrawlUnknownPlanNeverExceedsFree(t *testing.T) {
	t.Setenv(firecrawlPlanEnv, "enterprise")
	name, _ := activeFirecrawlPlan()
	if name != "free" {
		t.Fatalf("unpublished plan should not grant extra capacity: %s", name)
	}
	if _, rpm, _ := firecrawlRequestLimits("POST", "/v2/scrape"); rpm != 10 {
		t.Fatal("unrecognized plan bypassed Free rate limits")
	}
	t.Setenv(firecrawlPlanEnv, "")
	if name, _ := activeFirecrawlPlan(); name != "free" {
		t.Fatal("missing plan should default to Free")
	}
}

func TestFirecrawlPlanVisibleInCLIStatus(t *testing.T) {
	t.Setenv(firecrawlPlanEnv, "hobby")
	t.Setenv("WEBCTX_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	var status strings.Builder
	if err := printProviderStatus(&status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.String(), "plan: hobby (scrape/map/search 100 RPM; 5 concurrent browsers)") {
		t.Fatalf("status should display the effective tier without revealing credentials: %s", status.String())
	}
}

func TestFirecrawlHighRateBucketsRemainCompact(t *testing.T) {
	q := &firecrawlQueue{dir: t.TempDir(), window: time.Minute, slots: 100}
	for i := 0; i < 300; i++ {
		wait, err := q.reserveRateToken("scrape", 10000)
		if err != nil || wait != 0 {
			t.Fatalf("high tier request %d: wait=%v err=%v", i, wait, err)
		}
	}
	file := filepath.Join(q.dir, "rate-scrape.state")
	stat, err := os.Stat(file)
	if err != nil || stat.Size() != firecrawlRateStateSize {
		t.Fatalf("scalable limiter should use one small file, stat=%v err=%v", stat, err)
	}
	// A later downgrade must not erase any recent requests.
	if wait, err := q.reserveRateToken("scrape", 10); err != nil || wait <= 0 {
		t.Fatalf("downgrading must retain the previous window: wait=%v err=%v", wait, err)
	}
}

func TestFirecrawlPaidPlanBrowserConcurrency(t *testing.T) {
	q := &firecrawlQueue{dir: t.TempDir(), window: time.Second, slots: 5}
	permits := make([]*firecrawlPermit, 0, 5)
	defer func() {
		for _, permit := range permits {
			permit.release()
		}
	}()
	for i := 0; i < 5; i++ {
		permit, err := q.browserSession(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		permits = append(permits, permit)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if blocked, err := q.browserSession(ctx); !errors.Is(err, context.DeadlineExceeded) {
		if blocked != nil {
			blocked.release()
		}
		t.Fatalf("sixth session bypassed Hobby capacity: %v", err)
	}
	permits[0].release()
	permit, err := q.browserSession(context.Background())
	if err != nil {
		t.Fatalf("slot was not reusable: %v", err)
	}
	permit.release()
}
