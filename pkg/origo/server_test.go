package origo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func request(t *testing.T, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/mcp"
	if key != "" {
		path += "?key=" + key
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, req)
	return response
}

func TestDataSchemasExposeExactEnumsAndNonNullableCalls(t *testing.T) {
	t.Setenv("ORIGO_API_KEY", "secret")
	result := request(t, "secret", `{"jsonrpc":"2.0","id":12,"method":"tools/list","params":{}}`)
	if result.Code != 200 {
		t.Fatalf("tools/list failed: %s", result.Body.String())
	}
	var decoded struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
				Annotations map[string]any `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	schemas := map[string]map[string]any{}
	for _, tool := range decoded.Result.Tools {
		if tool.Annotations["readOnlyHint"] != true {
			t.Fatalf("%s was not marked read-only", tool.Name)
		}
		if _, exists := tool.Annotations["openWorldHint"]; exists {
			t.Fatalf("%s should omit openWorldHint", tool.Name)
		}
		schemas[tool.Name] = tool.InputSchema
	}
	if len(schemas) != 5 {
		t.Fatalf("expected five tools, got %d", len(schemas))
	}
	props := func(name string) map[string]any { return schemas[name]["properties"].(map[string]any) }
	mode := props("research")["mode"].(map[string]any)
	view := props("research")["view"].(map[string]any)
	if mode["type"] != "string" || len(mode["enum"].([]any)) != 2 ||
		view["type"] != "string" || len(view["enum"].([]any)) != 3 {
		t.Fatalf("research discovery schemas are not typed: mode=%#v view=%#v", mode, view)
	}
	incl := props("research")["include"].(map[string]any)
	if incl["type"] != "array" || len(incl["items"].(map[string]any)["enum"].([]any)) != 3 {
		t.Fatalf("include does not constrain allowed options: %#v", incl)
	}
	calls := props("execute")["calls"].(map[string]any)
	if _, present := props("execute")["max_credits"]; present {
		t.Fatal("execute still exposes credit-budget control")
	}
	if calls["type"] != "array" || calls["minItems"] != float64(1) || calls["maxItems"] != float64(10) {
		t.Fatalf("execute calls must be non-null and 1–10 items: %#v", calls)
	}
}

func TestServerRequiresConfiguredKey(t *testing.T) {
	t.Setenv("ORIGO_API_KEY", "")
	response := request(t, "somekey", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("without a configured key: expected 503, got %d", response.Code)
	}
}

func TestServerAuthentication(t *testing.T) {
	t.Setenv("ORIGO_API_KEY", "secret")
	for _, key := range []string{"", "wrong"} {
		response := request(t, key, `{}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("key %q: expected 401, got %d", key, response.Code)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("no-store header missing")
		}
	}
	response := request(t, "secret", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"origo"`) {
		t.Fatalf("initialize failed: %d %s", response.Code, response.Body.String())
	}
}

func TestFiveToolsWithProgressiveDataInterface(t *testing.T) {
	t.Setenv("ORIGO_API_KEY", "secret")
	response := request(t, "secret", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("tools/list failed: %d %s", response.Code, response.Body.String())
	}
	text := response.Body.String()
	if !strings.Contains(text, `"name":"read_link"`) || !strings.Contains(text, `"name":"map_site"`) ||
		!strings.Contains(text, `"name":"research"`) || !strings.Contains(text, `"name":"inspect"`) ||
		!strings.Contains(text, `"name":"execute"`) || strings.Contains(text, `"name":"search"`) {
		t.Fatalf("unexpected tools: %s", text)
	}
	if count := strings.Count(text, `"name":`); count != 5 {
		t.Fatalf("expected exactly 5 tools, got %d: %s", count, text)
	}
	if count := strings.Count(text, `"readOnlyHint":true`); count != 5 {
		t.Fatalf("expected all five MCP tools to declare readOnlyHint=true, got %d: %s", count, text)
	}
	if strings.Contains(text, `"readOnlyHint":false`) || strings.Contains(text, `"openWorldHint"`) || strings.Contains(text, `"destructiveHint"`) {
		t.Fatalf("unexpected non-read-only or open-world/destructive annotations: %s", text)
	}
}

func TestQuestionIsOptionalOnReadLinkOnly(t *testing.T) {
	t.Setenv("ORIGO_API_KEY", "secret")
	response := request(t, "secret", `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("list tools: %s", response.Body.String())
	}
	text := response.Body.String()
	if !strings.Contains(text, `"question"`) || !strings.Contains(text, `"required":["url"]`) {
		t.Fatalf("question should be optional; URL remains required: %s", text)
	}
}
