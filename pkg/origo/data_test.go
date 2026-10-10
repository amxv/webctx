package origo

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testDataRoundTripper func(*http.Request) (*http.Response, error)

func (f testDataRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func toolResponse(t *testing.T, name string, args any, id int) map[string]any {
	t.Helper()
	command, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := request(t, "secret", string(command))
	if response.Code != 200 {
		t.Fatalf("MCP status %d: %s", response.Code, response.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("MCP JSON decode: %v", err)
	}
	if decoded["error"] != nil {
		t.Fatalf("MCP protocol error: %s", response.Body.String())
	}
	result, ok := decoded["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %s", response.Body.String())
	}
	return result
}

func TestDataToolErrorsAreActionableStructuredMCPResults(t *testing.T) {
	t.Setenv("ORIGO_API_KEY", "secret")
	for _, tc := range []struct {
		name  string
		input map[string]any
		code  string
	}{
		{"research", map[string]any{"view": "invalid"}, "invalid_view"},
		{"inspect", map[string]any{"id": "not-a-valid-operation/"}, "invalid_operation"},
		{"execute", map[string]any{"calls": []any{}}, "invalid_calls"},
	} {
		response := toolResponse(t, tc.name, tc.input, 11)
		if response["isError"] != true {
			t.Fatalf("%s should return isError: %#v", tc.name, response)
		}
		output, ok := response["structuredContent"].(map[string]any)
		if !ok {
			t.Fatalf("%s did not preserve structuredContent: %#v", tc.name, response)
		}
		errorResult, ok := output["error"].(map[string]any)
		if !ok || errorResult["code"] != tc.code {
			t.Fatalf("%s error should be %s: %#v", tc.name, tc.code, response)
		}
	}
}

func TestDataToolResearchInspectExecuteOverHTTP(t *testing.T) {
	t.Setenv("ORIGO_API_KEY", "secret")
	t.Setenv("FIRECRAWL_API_KEY", "test")
	t.Setenv("WEBCTX_ALEXANDRIA_PAID", "true")
	original := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = original })
	var paidCount int
	http.DefaultClient = &http.Client{Transport: testDataRoundTripper(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var args map[string]any
		if json.Unmarshal(body, &args) != nil {
			t.Fatalf("bad upstream request: %s", body)
		}
		var data map[string]any
		switch {
		case req.URL.Path == "/v2/search":
			data = map[string]any{"success": true, "data": map[string]any{"tools": []any{
				map[string]any{"id": "example/data/list", "provider": "example", "capability": "data/list", "name": "List example data", "creditsCost": 2, "description": "Demo read-only data"},
			}}}
		case req.URL.Path == "/v2/scrape" && args["alexandria"] != nil:
			switch args["alexandria"].(type) {
			case map[string]any:
				data = map[string]any{"success": true, "data": map[string]any{"alexandria": []any{
					map[string]any{"provider": "firecrawl", "capability": "find-tools", "data": map[string]any{
						"items": []any{map[string]any{
							"id": "example/data/list", "provider": "example", "capability": "data/list", "name": "List example data",
							"creditsCost": 2, "perRecord": false, "options": []any{map[string]any{"name": "query", "type": "string", "required": true}},
							"response": map[string]any{"key": "data"},
						}},
					}},
				}}}
			case []any:
				paidCount++
				if req.Header.Get("x-request-id") == "" {
					t.Fatal("paid call missing idempotency header")
				}
				data = map[string]any{"success": true, "data": map[string]any{"creditsCost": 2, "alexandria": []any{
					map[string]any{"provider": "example", "capability": "data/list", "creditsCost": 2, "alexandriaId": "receipt-1", "data": map[string]any{"data": []any{map[string]any{"answer": 42}}}},
				}}}
			}
		}
		if data == nil {
			t.Fatalf("unhandled upstream operation %s", req.URL)
		}
		b, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header), Request: req}, nil
	})}
	discovery := toolResponse(t, "research", map[string]any{"query": "example data"}, 12)
	if discovery["isError"] == true || discovery["structuredContent"] == nil {
		t.Fatalf("research failed: %#v", discovery)
	}
	contract := toolResponse(t, "inspect", map[string]any{"id": "example/data/list"}, 13)
	structured := contract["structuredContent"].(map[string]any)
	if structured["id"] != "example/data/list" || structured["next"] == nil {
		t.Fatalf("inspect failed: %#v", structured)
	}
	result := toolResponse(t, "execute", map[string]any{"calls": []any{
		map[string]any{"id": "example/data/list", "inputs": map[string]any{"query": "forty two"}},
	}, "max_credits": 3}, 14)
	final := result["structuredContent"].(map[string]any)
	if result["isError"] == true || final["credits_used"] != float64(2) || paidCount != 1 {
		t.Fatalf("paid query failed: %#v calls=%d", final, paidCount)
	}
}
