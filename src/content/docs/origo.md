---
title: Origo MCP server
description: Connect ChatGPT and other MCP clients to the shared web/document reader and progressively discover, inspect, and execute structured data sources.
order: 12
category: Guides
summary: Origo is the hosted MCP transport over WebCTX's shared retrieval engine.
---

**Origo** is WebCTX's hosted MCP interface, not a separate retrieval implementation. It uses the same Go retrieval engine as the CLI and exposes **five** tools: `read_link`, `map_site`, `research`, `inspect`, and `execute`.

`read_link` accepts an optional `question` for source-grounded, question-focused exploration. URL-only calls and `map_site` retain their original behavior. The new data tools progressively reveal the relevant provider contracts and execute them when desired.

The CLI includes `search`, `read-link`, `map-site`, `research`, `inspect`, and `execute`. Origo deliberately excludes *federated web* `search` while providing the same three structured-data commands as the CLI.

## Structured-data tools

`research` finds structured sources and operations using natural-language questions, websites, source/category/group filters, and browse/pagination controls. Its results contain exact IDs and next-step arguments for `inspect`.

`inspect` retrieves a source's operations or one operation's complete upstream input and output contracts, validation rules, examples and price. Its response includes a ready-shaped `execute` call and identifies any required inputs the agent needs to fill.

`execute` makes one to ten provider-native data queries with the selected IDs and input fields. It uses paid Firecrawl Alexandria credits; the server validates inputs and enforces a 200-credit ceiling configured in Vercel, with no user-facing budget controls. Results preserve original JSON, estimated and actual charges, provenance and next-page continuations where supported.

See [Research structured data](/docs/data-sources) for the complete API and CLI workflow.

## Connect an MCP client

```text
https://api.origo.ashray.xyz/mcp?key=<your-private-key>
```

Origo uses the standard MCP Streamable HTTP protocol in stateless mode, so it can run as a Go Function on Vercel without a persistent session store. A long random `ORIGO_API_KEY` is stored in the **Origo API project**, never in this public repository or the docs deployment.

Treat the full URL as a credential. Query-string keys can be exposed in logs, clipboard history, analytics, or pasted links. Do not share it or visit the authenticated endpoint as an ordinary webpage.

## What `read_link` does

For REST/API questions, Origo can also inspect the site's published OpenAPI document linked from llms.txt and return exact operations, request-schema fields, and published request bodies. Code and cURL examples are kept whole, not shortened into invalid fragments. This is read-only documentation retrieval; Origo does not execute an API just because it is documented by a source page.

Origo and the CLI share the same improved native Markdown reader and challenge-screening logic. A canonical documentation URL can resolve to official directory index.md or page .md files without requiring the agent to rewrite it. Question-focused retrieval expands relevant links, rejects CAPTCHA boilerplate, and reports request fields whose support remains incomplete. In development, WEBCTX_DEBUG=1 traces retrieval stages and durations without exposing URL queries or keys.

Pass an absolute HTTP(S) URL. Origo uses the same retrieval ladder as `webctx read-link`:

1. **GitHub-native** structured or raw sources, including precise source lines, Markdown heading selectors, issues, pull requests, changes, and Actions.
2. **Raw structured documents**, such as JSON/OpenAPI, YAML, XML, and other supported public text sources.
3. **Native Markdown**, when a direct `.md` representation exists.
4. **Firecrawl Scrape**, with clean main-content Markdown, free Alexandria domain-tool matching where available, and an automatic proxy that tries the basic route before enhanced proxying.
5. **Explicit enhanced proxy**, if Firecrawl technically succeeds but returns a blocked or empty page.
6. **Firecrawl Browser Sandbox**, if scraping still fails, with a short-lived, read-only agent-browser extraction.

Provider discovery, validation and paid execution share the same backend between CLI and MCP; agents do not need provider-specific MCP connections.

`read_link` performs free Alexandria discovery only. `execute` deliberately invokes paid provider operations using the caller's chosen inputs. The server checks the estimated charge against its configured limit and shows credits used. Provider terms must still be accepted separately by a human. Source excerpts and embedded JSON are provided as evidence, not as a generated or independently verified answer.

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
