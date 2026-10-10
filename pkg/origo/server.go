// Package origo provides the independently deployed MCP transport for the
// shared WebCTX retrieval engine. Search is intentionally not exposed.
package origo

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/amxv/webctx/pkg/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type linkInput struct {
	URL string `json:"url" jsonschema:"Absolute HTTP(S) URL to read as source-grounded Markdown"`
}

type readLinkInput struct {
	URL      string `json:"url" jsonschema:"Absolute HTTP(S) URL to read as source-grounded Markdown"`
	Question string `json:"question,omitempty" jsonschema:"Optional question or task. When set, automatically follows relevant source links and returns focused, cited context."`
}

func makeServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "origo", Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "read_link",
		Title:       "Read link",
		Description: "Read a URL as source-grounded Markdown. Optionally provide a question to automatically explore related docs and return relevant API contracts, code examples and precise source URLs. No search query required; no paid Alexandria tool execution.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input readLinkInput) (*mcp.CallToolResult, any, error) {
		result, err := retrieval.ReadLinkFocused(input.URL, input.Question)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result}}}, nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "map_site",
		Title:       "Map site",
		Description: "Discover URLs on a site using Firecrawl's sitemap-based map API. Does not perform web search.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input linkInput) (*mcp.CallToolResult, any, error) {
		result, err := retrieval.MapSite(input.URL)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result}}}, nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "research",
		Title:       "Find data sources",
		Description: "Discover real-world information sources and their available operations. Search semantically, browse all sources, groups or operations, filter by websites/source IDs/categories, and paginate. Free; returns IDs and copy-ready inspect calls. Use read_link for regular documentation.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input retrieval.ResearchInput) (*mcp.CallToolResult, any, error) {
		result, err := retrieval.Research(input)
		return dataMCPResult(result, err)
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "inspect",
		Title:       "Inspect a data source or operation",
		Description: "Explore operations in a source, or inspect one operation's EXACT input types, required fields, constraints, output contract, pricing, and examples. Returns the next execute call with the exact input keys to fill. Free.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input retrieval.InspectInput) (*mcp.CallToolResult, any, error) {
		result, err := retrieval.Inspect(input)
		return dataMCPResult(result, err)
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "execute",
		Title:       "Query a data source (uses credits)",
		Description: "Execute 1-10 inspected structured-data operations using their exact source IDs and inputs. Charges Firecrawl Alexandria credits after server-side schema and credit-budget checks. Supports pagination/continuations, stable request_id for safe retries, and original provider JSON. No third-party terms are accepted automatically.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input retrieval.ExecuteInput) (*mcp.CallToolResult, any, error) {
		result, err := retrieval.Execute(input)
		return dataMCPResult(result, err)
	})
	return s
}

func dataMCPResult(result map[string]any, err error) (*mcp.CallToolResult, any, error) {
	failed := err != nil
	if failed {
		// Use the shared error serializer to preserve codes, corrective details
		// and provider-term acceptance URLs as structured tool output.
		result = retrieval.DataErrorResult(err)
	}
	bytes, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(bytes)}},
		StructuredContent: result,
		IsError:           failed,
	}, nil, nil
}

var transport = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
	return makeServer()
}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Robots-Tag", "noindex")
		secret := strings.TrimSpace(os.Getenv("ORIGO_API_KEY"))
		if secret == "" {
			http.Error(w, "MCP endpoint not configured", http.StatusServiceUnavailable)
			return
		}
		keys := r.URL.Query()["key"]
		if len(keys) != 1 || !validKey(secret, keys[0]) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		transport.ServeHTTP(w, r)
	})
}

func validKey(expected, supplied string) bool {
	if supplied == "" {
		return false
	}
	want := sha256.Sum256([]byte(expected))
	got := sha256.Sum256([]byte(supplied))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}
