package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// API contracts are public documentation, not calls to the documented API.
// When a question requires exact REST payloads, inspecting the site's linked
// OpenAPI spec is more faithful than extrapolating HTTP requests from SDK code.
type openAPIContext struct {
	URL     string
	Content string
}

func asksForAPIContracts(question string) bool {
	question = strings.ToLower(question)
	return strings.Contains(question, "rest") || strings.Contains(question, "openapi") ||
		(strings.Contains(question, "api") && (strings.Contains(question, "endpoint") || strings.Contains(question, "request") || strings.Contains(question, "schema") || strings.Contains(question, "contract"))) ||
		(strings.Contains(question, "exact") && (strings.Contains(question, "curl") || strings.Contains(question, "request example")))
}

func discoverOpenAPIContract(rawURL, question string) openAPIContext {
	if !asksForAPIContracts(question) || directSourceLanguage(rawURL) != "" {
		return openAPIContext{}
	}
	u, err := url.Parse(rawURL)
	if err != nil || validateSourceURL(rawURL) != nil || strings.EqualFold(u.Hostname(), "github.com") {
		return openAPIContext{}
	}
	indexURL := u.Scheme + "://" + u.Host + "/llms.txt"
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	index, err := fetchPublicText(ctx, indexURL, 300000)
	if err != nil {
		return openAPIContext{}
	}
	var specURLs []sourceCandidate
	seen := make(map[string]bool)
	for _, match := range mdLinkPattern.FindAllStringSubmatch(index, 300) {
		dest, err := url.Parse(strings.TrimSpace(match[2]))
		if err != nil {
			continue
		}
		resolved := u.ResolveReference(dest)
		name := strings.ToLower(resolved.Path)
		if !strings.EqualFold(resolved.Hostname(), u.Hostname()) || resolved.Port() != u.Port() ||
			!strings.HasSuffix(name, ".json") || !strings.Contains(name, "openapi") ||
			validateSourceURL(resolved.String()) != nil || sensitiveURL(resolved) {
			continue
		}
		sourceURL := canonicalSourceURL(resolved)
		if seen[sourceURL] {
			continue
		}
		seen[sourceURL] = true
		score := 0
		if strings.Contains(name, "v2-openapi") {
			score += 10
		}
		if strings.HasPrefix(name, "/api-reference/") {
			score += 5
		}
		if name == "/openapi.json" {
			score += 5
		}
		score -= len(name) / 20 // Prefer the canonical path over translations.
		specURLs = append(specURLs, sourceCandidate{url: sourceURL, score: score})
	}
	sort.SliceStable(specURLs, func(i, j int) bool { return specURLs[i].score > specURLs[j].score })
	for i, item := range specURLs {
		if i >= 2 { // Don't recursively fetch every language/version.
			break
		}
		body, err := fetchPublicText(ctx, item.url, maxDirectSourceBytes)
		if err != nil {
			continue
		}
		if content := summarizeOpenAPIContract(body, question); content != "" {
			return openAPIContext{URL: item.url, Content: content}
		}
	}
	return openAPIContext{}
}

type openAPIOperation struct {
	Method   string
	Path     string
	Summary  string
	Score    int
	Request  map[string]any
	Response map[string]any
}

func summarizeOpenAPIContract(source, question string) string {
	var spec struct {
		OpenAPI string `json:"openapi"`
		Swagger string `json:"swagger"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if json.Unmarshal([]byte(source), &spec) != nil || (spec.OpenAPI == "" && spec.Swagger == "") || len(spec.Paths) == 0 {
		return ""
	}
	terms := queryTerms(question)
	terms = append(terms, relatedAPITerms(question)...)
	var operations []openAPIOperation
	for path, methods := range spec.Paths {
		// A broad question about Alexandria must not bring in unrelated API
		// routes such as research papers or team activity merely because the
		// path contains "search" or a summary happens to contain "API".
		if strings.Contains(strings.ToLower(question), "alexandria") &&
			strings.Contains(strings.Trim(path, "/"), "/") &&
			!strings.Contains(strings.ToLower(path), "alexandria") {
			continue
		}
		for method, rawDetail := range methods {
			switch strings.ToUpper(method) {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				continue
			}
			detail := mapValue(rawDetail)
			summary := stringValue(detail["summary"])
			req := mapValue(detail["requestBody"])
			resp := mapValue(detail["responses"])
			score := termScore(path+" "+summary, terms)*4 + termScore(stringValue(detail["description"]), terms)
			if strings.Contains(strings.ToLower(question), "alexandria") &&
				termScore(strings.Trim(path, "/"), []string{"search", "scrape", "find", "tools", "alexandria"}) == 0 {
				continue
			}
			if examples := requestExamples(req); len(examples) > 0 {
				for name, example := range examples {
					score += termScore(name+" "+stringValue(example["summary"]), terms) * 3
				}
			}
			if score > 0 {
				operations = append(operations, openAPIOperation{
					Method: strings.ToUpper(method), Path: path, Summary: summary, Score: score, Request: req, Response: resp,
				})
			}
		}
	}
	sort.SliceStable(operations, func(i, j int) bool {
		if operations[i].Score == operations[j].Score {
			return operations[i].Path < operations[j].Path
		}
		return operations[i].Score > operations[j].Score
	})
	if len(operations) == 0 {
		return ""
	}
	apiRoot := ""
	for _, server := range spec.Servers {
		if validateSourceURL(server.URL) == nil {
			apiRoot = strings.TrimRight(server.URL, "/")
			break
		}
	}
	parts := []string{"OpenAPI-defined HTTP operations and example request bodies (not executed):"}
	for i, operation := range operations {
		if i >= 4 {
			break
		}
		endpoint := operation.Path
		if apiRoot != "" {
			endpoint = apiRoot + operation.Path
		}
		parts = append(parts, "", fmt.Sprintf("### %s %s", operation.Method, endpoint))
		if operation.Summary != "" {
			parts = append(parts, operation.Summary)
		}
		if examples := requestExamples(operation.Request); len(examples) > 0 {
			names := make([]string, 0, len(examples))
			for name := range examples {
				names = append(names, name)
			}
			sort.SliceStable(names, func(i, j int) bool {
				a := termScore(names[i]+" "+stringValue(examples[names[i]]["summary"]), terms)
				b := termScore(names[j]+" "+stringValue(examples[names[j]]["summary"]), terms)
				if a == b {
					return names[i] < names[j]
				}
				return a > b
			})
			emitted := 0
			for _, name := range names {
				value := examples[name]["value"]
				body, err := json.MarshalIndent(value, "", "  ")
				if err != nil || len(body) > 9000 || len(body) == 0 {
					continue
				}
				if strings.Contains(strings.ToLower(question), "alexandria") &&
					!strings.Contains(strings.ToLower(name+" "+string(body)), "alexandria") &&
					!strings.Contains(strings.ToLower(name+" "+string(body)), "findtools") &&
					!strings.Contains(strings.ToLower(name+" "+string(body)), "find-tools") {
					continue
				}
				if emitted >= 2 {
					break
				}
				emitted++
				parts = append(parts, "", "**Published request example:** "+stringValue(examples[name]["summary"]))
				if apiRoot != "" && !strings.Contains(endpoint, "{") {
					// Keep the published JSON body verbatim; the API key must expand in the shell.
					parts = append(parts, fmt.Sprintf("```bash\ncurl -X %s %q \\\n  -H \"Authorization: Bearer $API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  --data-binary @- <<'JSON_REQUEST'\n%s\nJSON_REQUEST\n```", operation.Method, endpoint, string(body)))
				} else {
					parts = append(parts, "```json\n"+string(body)+"\n```")
				}
			}
		}
	}
	for _, name := range []string{"FindToolsOptions", "AlexandriaCall", "FindToolsData"} {
		if termScore(question, []string{"alexandria", "tools", "inspect", "schema", "contracts"}) == 0 {
			break
		}
		contract, ok := spec.Components.Schemas[name]
		if !ok {
			continue
		}
		parts = append(parts, "", "### "+name)
		if desc := stringValue(contract["description"]); desc != "" {
			parts = append(parts, desc)
		}
		if props, ok := contract["properties"].(map[string]any); ok {
			keys := make([]string, 0, len(props))
			for key := range props {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				item := mapValue(props[key])
				typ := stringValue(item["type"])
				if typ == "" {
					typ = "schema"
				}
				description := stringValue(item["description"])
				if len(description) > 240 {
					description = description[:240] + "…"
				}
				parts = append(parts, fmt.Sprintf("- `%s` (%s): %s", key, typ, description))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func mapValue(value any) map[string]any {
	v, _ := value.(map[string]any)
	return v
}

func requestExamples(request map[string]any) map[string]map[string]any {
	content := mapValue(request["content"])
	jsonContent := mapValue(content["application/json"])
	examples := mapValue(jsonContent["examples"])
	out := make(map[string]map[string]any, len(examples))
	for key, example := range examples {
		if item := mapValue(example); item != nil && item["value"] != nil {
			out[key] = item
		}
	}
	return out
}

func relatedAPITerms(question string) []string {
	question = strings.ToLower(question)
	var terms []string
	if strings.Contains(question, "discover") || strings.Contains(question, "locat") {
		terms = append(terms, "search", "find", "list")
	}
	if strings.Contains(question, "inspect") || strings.Contains(question, "contract") {
		terms = append(terms, "find", "tools", "schema")
	}
	if strings.Contains(question, "execut") || strings.Contains(question, "run ") {
		terms = append(terms, "scrape", "call", "execute")
	}
	return terms
}
