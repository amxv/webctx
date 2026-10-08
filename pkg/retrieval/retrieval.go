// Package retrieval exposes WebCTX's shared retrieval engine to non-CLI
// transports. The CLI remains backward-compatible while Origo MCP can call
// these functions without importing Go internal packages from Vercel's
// generated handler package.
package retrieval

import "github.com/amxv/webctx/internal/app"

func ReadLink(url string) (string, error) { return app.ReadLink(url) }

func MapSite(url string) (string, error) { return app.MapSite(url) }
