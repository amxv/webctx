package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func exampleOpenAPISpec(t *testing.T) string {
	t.Helper()
	source := map[string]any{
		"openapi": "3.0.0",
		"servers": []any{map[string]any{"url": "https://api.example.com/v2"}},
		"paths": map[string]any{
			"/search": map[string]any{
				"parameters": []any{map[string]any{"name": "unused"}}, // Legal non-operation path entry.
				"post":       map[string]any{"summary": "Search the Alexandria catalogue"},
			},
			"/scrape": map[string]any{
				"post": map[string]any{
					"summary": "Execute a provider capability or find tool contracts",
					"requestBody": map[string]any{
						"content": map[string]any{
							"application/json": map[string]any{
								"examples": map[string]any{
									"findTools": map[string]any{
										"summary": "Find Tools (free)",
										"value": map[string]any{
											"alexandria": map[string]any{
												"provider":   "firecrawl",
												"capability": "find-tools",
												"options": map[string]any{
													"providers": []string{"particle"},
													"expand":    []string{"options", "response"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]any{"schemas": map[string]any{
			"FindToolsOptions": map[string]any{
				"description": "Browse providers and inspect tool contracts.",
				"properties": map[string]any{
					"expand":    map[string]any{"type": "array", "description": "Contract sections to return"},
					"providers": map[string]any{"type": "array", "description": "Provider IDs"},
				},
			},
		}},
	}
	bytes, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func TestOpenAPIContractReturnsExactInspectionRESTExample(t *testing.T) {
	result := summarizeOpenAPIContract(exampleOpenAPISpec(t),
		"What are the Alexandria REST API endpoints for discovering, inspecting, and executing tools? Include exact request examples.")
	for _, want := range []string{
		"POST https://api.example.com/v2/search",
		"POST https://api.example.com/v2/scrape",
		"\"provider\": \"firecrawl\"",
		"\"capability\": \"find-tools\"",
		"\"expand\": [",
		"FindToolsOptions",
		"<<'JSON_REQUEST'",
		"-H \"Authorization: Bearer $API_KEY\"",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("OpenAPI extraction lost %q:\n%s", want, result)
		}
	}
	if count := strings.Count(result, "JSON_REQUEST\n"+strings.Repeat(string(rune(96)), 3)); count != 1 {
		t.Fatalf("expected one complete REST example, got %d:\n%s", count, result)
	}
}

func TestQuestionFocusedReadPullsOfficialOpenAPISpec(t *testing.T) {
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	spec := exampleOpenAPISpec(t)
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/llms.txt":
			return testHTTPResponse(req, http.StatusOK,
				"# Docs\n\n## OpenAPI Specs\n\n- [v2-openapi](/api-reference/v2-openapi.json)\n", nil), nil
		case "/api-reference/v2-openapi.json":
			return testHTTPResponse(req, http.StatusOK, spec, nil), nil
		default:
			return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
		}
	})}
	primary := "# Alexandria\n\n## Discovery\n\ncurl https://api.example.com/v2/search\n" +
		"\n## Execution\n\ncurl https://api.example.com/v2/scrape\n"
	reader := func(url string) (string, error) { return primary, nil }
	result, err := readLinkFocused("https://docs.example.com/features/alexandria",
		"What are the Alexandria REST API endpoints for discovering, inspecting, and executing tools? Include exact request examples.",
		reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## REST API contract",
		"**Source:** https://docs.example.com/api-reference/v2-openapi.json",
		"\"capability\": \"find-tools\"",
		"curl https://api.example.com/v2/search",
		"curl https://api.example.com/v2/scrape",
		"**Coverage:** 2 source document(s)",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("focused read missing %q:\n%s", want, result)
		}
	}
}

func TestOpenAPIDiscoveryNeverCrossesToUnrelatedHost(t *testing.T) {
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	var calledOutsideIndex bool
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/llms.txt" {
			calledOutsideIndex = true
		}
		return testHTTPResponse(req, http.StatusOK,
			"- [External OpenAPI](https://evil.example.com/openapi.json)\n"+
				"- [Local OpenAPI](http://127.0.0.1/openapi.json)\n", nil), nil
	})}
	if found := discoverOpenAPIContract("https://docs.example.com/reference",
		"Find exact REST API endpoint request examples"); found.Content != "" || calledOutsideIndex {
		t.Fatalf("unsafe OpenAPI discovery: %+v, off-index=%v", found, calledOutsideIndex)
	}
}
