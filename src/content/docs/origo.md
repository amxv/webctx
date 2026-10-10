---
title: Origo MCP server
description: Connect ChatGPT and other MCP clients to WebCTX's read-link and site-mapping engine without exposing search.
order: 12
category: Guides
summary: Origo is the hosted MCP transport over WebCTX's shared retrieval engine.
---

**Origo** is WebCTX's hosted MCP interface, not a separate retrieval implementation. It uses the same Go retrieval engine as the CLI and exposes only two tools: `read_link` and `map_site`.

`read_link` now accepts an optional `question`. When supplied, it automatically follows up to two relevant same-host documentation references, selects focused API/code excerpts, and provides precise source URLs. The URL-only call keeps the previous full-page behavior. `map_site` remains unchanged; there are no additional tools to discover or learn.

The CLI keeps `search`, `read-link`, and `map-site`. Origo deliberately excludes search, so it complements your agent's existing search capabilities without replacing them.

## Connect an MCP client

```text
https://api.origo.ashray.xyz/mcp?key=<your-private-key>
```

Origo uses the standard MCP Streamable HTTP protocol in stateless mode, so it can run as a Go Function on Vercel without a persistent session store. A long random `ORIGO_API_KEY` is stored in the **Origo API project**, never in this public repository or the docs deployment.

Treat the full URL as a credential. Query-string keys can be exposed in logs, clipboard history, analytics, or pasted links. Do not share it or visit the authenticated endpoint as an ordinary webpage.

## What `read_link` does

For REST/API questions, Origo can also inspect the site's published OpenAPI document linked from llms.txt and return exact operations, request-schema fields, and published request bodies. Code and cURL examples are kept whole, not shortened into invalid fragments. This is read-only documentation retrieval; Origo does not execute an API just because it is documented by a source page.

Pass an absolute HTTP(S) URL. Origo uses the same retrieval ladder as `webctx read-link`:

1. **GitHub-native** structured or raw sources, including precise source lines, Markdown heading selectors, issues, pull requests, changes, and Actions.
2. **Raw structured documents**, such as JSON/OpenAPI, YAML, XML, and other supported public text sources.
3. **Native Markdown**, when a direct `.md` representation exists.
4. **Firecrawl Scrape**, with clean main-content Markdown, free Alexandria domain-tool matching where available, and an automatic proxy that tries the basic route before enhanced proxying.
5. **Explicit enhanced proxy**, if Firecrawl technically succeeds but returns a blocked or empty page.
6. **Firecrawl Browser Sandbox**, if scraping still fails, with a short-lived, read-only agent-browser extraction.

Only the retrieval adapters are internal implementation details. There are no additional model-facing MCP tools.

Alexandria tool discovery does not authorize paid data-provider calls or acceptance of third-party terms. Origo never executes those calls on a user's behalf. Source excerpts and embedded JSON are provided as evidence, not as a generated or independently verified answer.

Firecrawl Scrape uses **Firecrawl's built-in 30-minute cache** (`maxAge: 1800000` milliseconds). No application database, custom KV namespace, Redis service, or separate cache layer is used. Native GitHub and direct Markdown reads remain direct source reads.

Source URL errors, blocked-page responses, and native GitHub permission failures are surfaced as errors rather than treated as authoritative empty Markdown. Browser sessions are best-effort and close after extraction; the browser has no tool to click arbitrary links or submit forms through MCP.

## What `map_site` does

Pass a site URL to discover its reachable pages using Firecrawl's Map API. Origo shares WebCTX's existing mapping settings, including sitemap inclusion, subdomain discovery, query-parameter normalization, and the bounded URL inventory.

## Development and deployment

Two independent Vercel projects build from the same `amxv/webctx` repository:

| Product | Vercel project | Deployment | Trigger |
| --- | --- | --- | --- |
| WebCTX docs | `webctx-docs` | `webctx.ashray.xyz` | Docs and site changes on `main` |
| Origo MCP | `origo-api` | `api.origo.ashray.xyz` | Retrieval and MCP changes on `main` |

`vercel.mjs` is conditional on the project identity. `scripts/should-build.mjs` filters relevant paths independently. The CLI's tag-driven GitHub Releases and npm publishing are unchanged.

Origo code lives in `api/mcp.go`, `pkg/origo`, and `pkg/retrieval`. The shared retrieval engine remains in `internal/app`.

Run the usual checks before pushing:

```bash
make check
bun run docs:check
bun run docs:build
```

For native CLI installation and examples, follow the [WebCTX quickstart](/docs/quickstart). For details of direct-source optimizations, see [How URL reading works](/docs/architecture).
