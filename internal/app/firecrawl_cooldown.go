package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Share server-reported 429 cooldowns across callers using the SAME Firecrawl
// account on this host. Each endpoint also keeps its own window.
// File locking and the timestamp update are atomic across CLI processes.
func (q *firecrawlQueue) rateCooldown(group string, setFor time.Duration) (time.Duration, error) {
	if group == "" {
		return 0, nil
	}
	if err := q.setup(); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var file *os.File
	for {
		locked, acquired, err := tryOpenQueueLock(filepath.Join(q.dir, "cooldown-"+group+".lock"))
		if err != nil {
			return 0, err
		}
		if acquired {
			file = locked
			break
		}
		if err := waitFirecrawl(ctx, 10*time.Millisecond); err != nil {
			return 0, err
		}
	}
	defer func() { unlockQueueFile(file); _ = file.Close() }()
	stamp, err := io.ReadAll(io.LimitReader(file, 48))
	if err != nil {
		return 0, err
	}
	var until int64
	if text := strings.TrimSpace(string(stamp)); text != "" {
		until, err = strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid Firecrawl %s cooldown: %w", group, err)
		}
	}
	if setFor > 0 {
		newUntil := time.Now().Add(setFor).UnixMilli()
		if newUntil > until {
			until = newUntil
			if err = file.Truncate(0); err == nil {
				_, err = file.WriteAt([]byte(strconv.FormatInt(until, 10)), 0)
			}
			if err != nil {
				return 0, err
			}
		}
	}
	remaining := time.Until(time.UnixMilli(until))
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}

func firecrawl429Cooldown(headers http.Header) time.Duration {
	if after := searchRetryAfter(headers.Get("Retry-After")); after > 0 {
		return after
	}
	if raw := headers.Get("RateLimit-Reset"); raw != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
			if n > 1e9 {
				return minDuration(time.Until(time.Unix(n, 0)), 24*time.Hour)
			}
			if n > 0 {
				return minDuration(time.Duration(n)*time.Second, 24*time.Hour)
			}
		}
	}
	// Without a reset header, the remote 429 might mean a full browser queue
	// rather than a minute quota. Retry soon while local windows still protect
	// the published rate cap.
	return 3 * time.Second
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
