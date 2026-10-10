package app

import (
	"context"
	"crypto/rand"
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
	"time"
)

const dataAPIBase = "https://api.firecrawl.dev/v2"
const defaultDataCreditBudget = 100
const maxDataResponseBytes int64 = 12 << 20

type DataError struct {
	Code           string         `json:"code"`
	Message        string         `json:"message"`
	RequiresAction map[string]any `json:"requires_action,omitempty"`
	Details        any            `json:"details,omitempty"`
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
	if err != nil || value < 1 || value > 100000 {
		return defaultDataCreditBudget
	}
	return value
}

func paidDataEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WEBCTX_ALEXANDRIA_PAID"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

var safeRequestID = regexp.MustCompile("^[A-Za-z0-9_-]{8,128}$")

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
	res, err := http.DefaultClient.Do(req)
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
		out := &DataError{Code: code, Message: message}
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
		return nil, nil, dataError("invalid_upstream_response", "The catalogue returned no browsable entries.")
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
