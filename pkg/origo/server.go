// Package origo provides the independently deployed MCP transport for the
// shared WebCTX retrieval engine. Search is intentionally not exposed.
package origo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/amxv/webctx/pkg/retrieval"
	"github.com/google/jsonschema-go/jsonschema"
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
			code := "read_failed"
			if strings.Contains(err.Error(), "absolute HTTP(S)") || strings.Contains(err.Error(), "readable URL") {
				code = "invalid_url"
			}
			return dataMCPResult(nil, &retrieval.DataError{Code: code, Message: err.Error()})
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result}}}, nil, nil
	})
	s.AddTool(&mcp.Tool{
		Name:        "map_site",
		Title:       "Map site",
		Description: "Discover website pages in bounded, paginated groups (30 by default). Filter by topic, path prefix and language (en by default, all for translations). Returns structured titles, descriptions, counts, and copy-ready next-page arguments.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: mapSiteSchema(),
	}, dataHandler(retrieval.MapSitePage))
	s.AddTool(&mcp.Tool{
		Name:        "research",
		Title:       "Find data sources",
		Description: "Discover real-world information sources and their available operations. Search semantically, browse all sources, groups or operations, filter by websites/source IDs/categories, and paginate. Free; returns IDs and copy-ready inspect calls. Use read_link for regular documentation.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: researchDataSchema(),
	}, dataHandler(retrieval.Research))
	s.AddTool(&mcp.Tool{
		Name:        "inspect",
		Title:       "Inspect a data source or operation",
		Description: "Explore operations in a source, or inspect one operation's EXACT input types, required fields, constraints, output contract, pricing, and examples. Returns the next execute call with the exact input keys to fill. Free.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: inspectDataSchema(),
	}, dataHandler(retrieval.Inspect))
	s.AddTool(&mcp.Tool{
		Name:        "execute",
		Title:       "Query a data source (uses credits)",
		Description: "Execute 1-10 inspected structured-data operations using exact operation IDs and inputs. Alexandria credits are charged automatically; the server enforces its configured 200-credit per-request ceiling. Actual credits are reported. Supports pagination, safe request_id retries and provider-native JSON. Does not accept third-party terms.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: executeDataSchema(),
	}, dataHandler(retrieval.Execute))
	return s
}

// Keep client-facing types narrow and self-describing. The SDK's default
// reflection makes optional slices nullable and string enums unconstrained;
// the catalogue operations support neither ambiguity.
func researchDataSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[retrieval.ResearchInput](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["mode"].Enum = []any{"ranked", "catalogue"}
	schema.Properties["view"].Enum = []any{"sources", "groups", "operations"}
	for _, key := range []string{"mode", "view"} {
		schema.Properties[key].Type = "string"
		schema.Properties[key].Types = nil
	}
	for _, key := range []string{"urls", "sources", "categories", "groups", "operations", "include"} {
		p := schema.Properties[key]
		p.Type = "array"
		p.Types = nil
	}
	schema.Properties["include"].Items.Enum = []any{"inputs", "output", "examples"}
	return schema
}

func inspectDataSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[retrieval.InspectInput](nil)
	if err != nil {
		panic(err)
	}
	p := schema.Properties["include"]
	p.Type = "array"
	p.Types = nil
	p.Items.Enum = []any{"inputs", "output", "examples"}
	return schema
}

func executeDataSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[retrieval.ExecuteInput](nil)
	if err != nil {
		panic(err)
	}
	countMin, countMax := 1, 10
	p := schema.Properties["calls"]
	p.Type = "array"
	p.Types = nil
	p.MinItems = &countMin
	p.MaxItems = &countMax
	return schema
}

func mapSiteSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[retrieval.MapInput](nil)
	if err != nil {
		panic(err)
	}
	return schema
}

// Manual validation keeps data errors in successful MCP tool responses rather
// than surfacing them as client-side INVALID_ARGUMENT tool exceptions.
func dataHandler[In any](fn func(In) (map[string]any, error)) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input In
		args := req.Params.Arguments
		if len(args) == 0 {
			args = []byte("{}")
		}
		decoder := json.NewDecoder(bytes.NewReader(args))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			response, _, e := dataMCPResult(nil, &retrieval.DataError{Code: "invalid_arguments", Message: err.Error()})
			return response, e
		}
		result, err := fn(input)
		response, _, e := dataMCPResult(result, err)
		return response, e
	}
}

func dataMCPResult(result map[string]any, err error) (*mcp.CallToolResult, any, error) {
	failed := err != nil
	if failed {
		// Use the shared error serializer to preserve codes, corrective details
		// and provider-term acceptance URLs as structured tool output.
		result = retrieval.DataErrorResult(err)
		result["ok"] = false
	} else {
		result["ok"] = true
	}
	bytes, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(bytes)}},
		StructuredContent: result,
		// Keep validation/provider failures in the structured result. Clients
		// commonly convert isError=true to opaque INVALID_ARGUMENT exceptions.
		IsError: false,
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
