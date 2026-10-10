package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const dataAPIBase = "https://api.firecrawl.dev/v2"

// The server, not a client-provided tool parameter, controls this preflight ceiling.
// A production override is configured on the Origo Vercel project.
const defaultDataCreditBudget = 200
const maxDataResponseBytes int64 = 12 << 20

type DataError struct {
	Code              string         `json:"code"`
	Message           string         `json:"message"`
	RequiresAction    map[string]any `json:"requires_action,omitempty"`
	Details           any            `json:"details,omitempty"`
	RetryAfterSeconds int            `json:"retry_after_seconds,omitempty"`
	RetryAt           string         `json:"retry_at,omitempty"`
	Retryable         bool           `json:"retryable,omitempty"`
}

func (e *DataError) Error() string { return e.Code + ": " + e.Message }
func dataError(code, message string) error {
	return &DataError{Code: code, Message: message}
}

func dataAPIKey() (string, error) {
	key := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
	if key == "" {
		return "", dataError("missing_credentials", "Set FIRECRAWL_API_KEY to use data source discovery and queries.")
	}
	return key, nil
}

func dataCreditCap() int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("WEBCTX_ALEXANDRIA_MAX_CREDITS")))
	if err != nil || value < 1 || value > defaultDataCreditBudget {
		return defaultDataCreditBudget
	}
	return value
}

var safeRequestID = regexp.MustCompile("^[A-Za-z0-9_-]{8,128}$")

var freeDataCooldown struct {
	sync.Mutex
	until time.Time
}

type dataReadRetryKey struct{}

var upstreamDataGate = make(chan struct{}, 3)

type catalogueCacheEntry struct {
	payload  map[string]any
	response map[string]any
	expires  time.Time
}

var catalogueCache = struct {
	sync.Mutex
	entries map[string]catalogueCacheEntry
}{entries: map[string]catalogueCacheEntry{}}
var catalogueFlight singleflight.Group

func cacheableCatalogueKey(options map[string]any) string {
	key, err := dataAPIKey()
	if err != nil || strings.HasPrefix(key, "test") {
		return ""
	}
	snapshot, err := json.Marshal(options)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(append(append([]byte(key), 0), snapshot...))
	return hex.EncodeToString(hash[:])
}

// Coordinate upstream read traffic per warm worker. Vercel has multiple
// workers, so this is a local best-effort throttle, not a global quota.
func dataReadCooldown(ctx context.Context) error {
	freeDataCooldown.Lock()
	until := freeDataCooldown.until
	freeDataCooldown.Unlock()
	if remaining := time.Until(until); remaining > 0 {
		if remaining < 4*time.Second {
			timer := time.NewTimer(remaining)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return dataError("request_cancelled", "The provider read was cancelled while throttled.")
			}
		}
		return rateLimitError(int(remaining.Seconds())+1, "The provider's request allowance is temporarily exhausted.", "local_cooldown")
	}
	return nil
}

func rateLimitError(seconds int, message, source string) *DataError {
	if seconds < 1 {
		seconds = 60
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return &DataError{
		Code: "rate_limited", Message: message, Retryable: true, RetryAfterSeconds: seconds,
		RetryAt: time.Now().Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339),
		Details: map[string]any{"retry_after_source": source, "guidance": "Retry this read after the indicated delay. For paid calls, preserve the original request_id and exact inputs."},
	}
}

func readRetryDelay(headers http.Header) (int, string) {
	if raw := strings.TrimSpace(headers.Get("Retry-After")); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil && n > 0 {
			return n, "retry-after-header"
		}
		if t, e := http.ParseTime(raw); e == nil {
			seconds := int(time.Until(t).Seconds()) + 1
			if seconds > 0 {
				return seconds, "retry-after-header"
			}
		}
	}
	if raw := strings.TrimSpace(headers.Get("RateLimit-Reset")); raw != "" {
		if n, e := strconv.ParseInt(raw, 10, 64); e == nil {
			if n > 1e9 {
				n = int64(time.Until(time.Unix(n, 0)).Seconds()) + 1
			}
			if n > 0 && n < 3601 {
				return int(n), "ratelimit-reset-header"
			}
		}
	}
	return 60, "estimated_minute_window"
}

func newRequestID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "webctx-" + hex.EncodeToString(bytes[:]), nil
}

func doDataAPI(ctx context.Context, endpoint string, payload any, requestID string) (map[string]any, error) {
	key, err := dataAPIKey()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, dataError("invalid_request", "Could not encode request as JSON: "+err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dataAPIBase+endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		req.Header.Set("x-request-id", requestID)
	}
	if requestID == "" {
		if err := dataReadCooldown(ctx); err != nil {
			return nil, err
		}
	}
	select {
	case upstreamDataGate <- struct{}{}:
	case <-ctx.Done():
		return nil, dataError("request_cancelled", "Timed out waiting for an upstream request slot.")
	}
	res, err := http.DefaultClient.Do(req)
	<-upstreamDataGate
	if err != nil {
		return nil, &DataError{Code: "network_error", Message: "The provider request did not complete. For a paid call, retry only with the same request_id to prevent duplicate charges."}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxDataResponseBytes+1))
	if err != nil {
		return nil, dataError("read_error", "Could not read provider response.")
	}
	if int64(len(raw)) > maxDataResponseBytes {
		return nil, dataError("response_too_large", "Provider response exceeded 12 MiB; use a smaller limit or pagination.")
	}
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil {
		if res.StatusCode == http.StatusTooManyRequests {
			seconds, source := readRetryDelay(res.Header)
			return nil, rateLimitError(seconds, "The upstream data provider is rate limiting requests.", source)
		}
		return nil, dataError("invalid_upstream_response", fmt.Sprintf("Provider returned non-JSON HTTP %d.", res.StatusCode))
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || decoded["success"] == false {
		code := stringValue(decoded["code"])
		if code == "" {
			code = fmt.Sprintf("http_%d", res.StatusCode)
		}
		message := stringValue(decoded["error"])
		if message == "" {
			message = stringValue(decoded["message"])
		}
		if message == "" {
			message = "The provider rejected the request."
		}
		if len(message) > 800 {
			message = message[:800]
		}
		if res.StatusCode == http.StatusTooManyRequests || strings.EqualFold(code, "rate_limit_exceeded") || strings.EqualFold(code, "rate_limited") {
			seconds, source := readRetryDelay(res.Header)
			if requestID == "" {
				freeDataCooldown.Lock()
				next := time.Now().Add(time.Duration(seconds) * time.Second)
				if next.After(freeDataCooldown.until) {
					freeDataCooldown.until = next
				}
				freeDataCooldown.Unlock()
			}
			// A short retry is safe for free discovery. Never automatically
			// repeat a paid execution: its actual charging state may be unknown.
			attempt, _ := ctx.Value(dataReadRetryKey{}).(int)
			if requestID == "" && seconds <= 2 && attempt == 0 {
				timer := time.NewTimer(time.Duration(seconds) * time.Second)
				defer timer.Stop()
				select {
				case <-timer.C:
					return doDataAPI(context.WithValue(ctx, dataReadRetryKey{}, 1), endpoint, payload, requestID)
				case <-ctx.Done():
					return nil, rateLimitError(seconds, "The provider's allowance has not recovered yet.", source)
				}
			}
			return nil, rateLimitError(seconds, "The upstream data provider is rate limiting requests.", source)
		}
		out := &DataError{Code: code, Message: message}
		if strings.EqualFold(code, "duplicate_request") {
			out.Message = "This request_id was already used with different inputs. Reuse the exact original request body to replay it, or omit request_id to start a new query."
			out.Details = map[string]any{"guidance": "Do not retry the conflicting payload with the same request_id.", "recovery": "Restore the exact original calls for replay, or generate a fresh request_id for intentionally new calls."}
		}
		if action, ok := decoded["requiresAction"].(map[string]any); ok {
			if target := stringValue(action["url"]); target != "" {
				if u, e := url.Parse(target); e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
					out.RequiresAction = map[string]any{"url": target}
				}
			}
		}
		return nil, out
	}
	return decoded, nil
}

func nativeDataCall(ctx context.Context, provider, capability string, options map[string]any, requestID string) (map[string]any, error) {
	request := map[string]any{"alexandria": map[string]any{
		"provider": provider, "capability": capability, "options": options,
	}}
	return doDataAPI(ctx, "/scrape", request, requestID)
}

func extractNativeData(response map[string]any) (map[string]any, error) {
	data, ok := response["data"].(map[string]any)
	if !ok {
		return nil, dataError("invalid_upstream_response", "The response has no data object.")
	}
	items, ok := data["alexandria"].([]any)
	if !ok || len(items) == 0 {
		return nil, dataError("invalid_upstream_response", "The response contains no Alexandria results.")
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		return nil, dataError("invalid_upstream_response", "The source returned an invalid result.")
	}
	return item, nil
}

func freeDataBrowse(ctx context.Context, options map[string]any) (map[string]any, map[string]any, error) {
	cacheKey := cacheableCatalogueKey(options)
	if cacheKey != "" {
		catalogueCache.Lock()
		cached, ok := catalogueCache.entries[cacheKey]
		catalogueCache.Unlock()
		if ok && time.Now().Before(cached.expires) {
			return cached.payload, cached.response, nil
		}
		type cacheResult struct {
			payload  map[string]any
			response map[string]any
		}
		result, err, _ := catalogueFlight.Do(cacheKey, func() (any, error) {
			data, raw, e := uncachedFreeDataBrowse(ctx, options)
			if e != nil {
				return nil, e
			}
			catalogueCache.Lock()
			if len(catalogueCache.entries) > 256 {
				catalogueCache.entries = map[string]catalogueCacheEntry{}
			}
			catalogueCache.entries[cacheKey] = catalogueCacheEntry{
				payload: data, response: raw, expires: time.Now().Add(2 * time.Minute),
			}
			catalogueCache.Unlock()
			return cacheResult{payload: data, response: raw}, nil
		})
		if err != nil {
			return nil, nil, err
		}
		got := result.(cacheResult)
		return got.payload, got.response, nil
	}
	return uncachedFreeDataBrowse(ctx, options)
}

func uncachedFreeDataBrowse(ctx context.Context, options map[string]any) (map[string]any, map[string]any, error) {
	response, err := nativeDataCall(ctx, "firecrawl", "find-tools", options, "")
	if err != nil {
		return nil, nil, err
	}
	item, err := extractNativeData(response)
	if err != nil {
		return nil, nil, err
	}
	details, ok := item["data"].(map[string]any)
	if !ok {
		if item["data"] == nil && item["error"] == nil {
			// Some Alexandria catalogues signal an unknown ID by returning
			// a successful find-tools call with data=null.
			return map[string]any{"items": []any{}, "total": 0}, response, nil
		}
		return nil, nil, dataError("invalid_upstream_response", "The catalogue returned an unreadable response. Retry discovery before selecting an operation.")
	}
	return details, response, nil
}

func defaultDataContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 45*time.Second)
}

func dataRawOrError(err error) map[string]any {
	var typed *DataError
	if errors.As(err, &typed) {
		return map[string]any{"error": typed}
	}
	return map[string]any{"error": map[string]any{"code": "internal_error", "message": err.Error()}}
}

// DataErrorResult preserves actionable failures in both CLI and MCP JSON.
func DataErrorResult(err error) map[string]any { return dataRawOrError(err) }

func listField(value any) []any {
	switch v := value.(type) {
	case []any:
		return v
	case []string:
		out := make([]any, 0, len(v))
		for _, s := range v {
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func stringField(value any) string      { s, _ := value.(string); return s }
func mapField(value any) map[string]any { m, _ := value.(map[string]any); return m }
func numberField(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, e := n.Float64()
		return f, e == nil
	default:
		return 0, false
	}
}
