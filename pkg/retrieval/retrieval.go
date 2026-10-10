// Package retrieval exposes WebCTX's shared retrieval engine to non-CLI
// transports. The CLI remains backward-compatible while Origo MCP can call
// these functions without importing Go internal packages from Vercel's
// generated handler package.
package retrieval

import "github.com/amxv/webctx/internal/app"

func ReadLink(url string) (string, error) { return app.ReadLink(url) }

// ReadLinkFocused follows related documentation when the caller has a question.
// A URL-only call retains the original complete-document behavior.
func ReadLinkFocused(url, question string) (string, error) {
	return app.ReadLinkFocused(url, question)
}

func MapSite(url string) (string, error) { return app.MapSite(url) }
