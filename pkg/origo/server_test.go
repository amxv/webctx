package origo

import (
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
