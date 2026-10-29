package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Keep stubbed Firecrawl requests from modifying the real user's shared queue
// during tests, while exercising actual file locks and rate accounting.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "webctx-firecrawl-tests-")
	if err != nil {
		os.Exit(2)
	}
	if os.Getenv("WEBCTX_FIRECRAWL_TEST_HELPER") != "1" && os.Getenv("WEBCTX_FIRECRAWL_ACCOUNTS_TEST_HELPER") != "1" {
		_ = os.Setenv("WEBCTX_FIRECRAWL_QUEUE_DIR", filepath.Join(dir, "rate-queue"))
	}
	if os.Getenv("WEBCTX_SEARCH_BALANCER_TEST_HELPER") != "1" && os.Getenv("WEBCTX_SEARCH_LIMITER_TEST_HELPER") != "1" {
		_ = os.Setenv("WEBCTX_SEARCH_KEY_BALANCER_DIR", filepath.Join(dir, "search-key-rotation"))
	}
	firecrawlQueueWindow = 100 * time.Millisecond
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
