---
title: Origo technical reference
description: MCP protocol, authentication, retrieval paths, data billing, and deployment.
order: 26
category: Reference
summary: How the hosted MCP interface is secured and operated.
---

**Origo** exposes the WebCTX retrieval engine through stateless MCP Streamable HTTP. It provides five tools: `read_link`, `map_site`, `research`, `inspect`, and `execute`. Federated web `search` is CLI-only.

## Endpoint and access

```text
https://api.origo.ashray.xyz/mcp?key=<your-private-key>
```

An Origo deployment must have its own `ORIGO_API_KEY`. Treat the full URL as a credential: query parameters can end up in logs, analytics, clipboard histories, and copied links. Do not embed this URL in public documentation, screenshots, or version control.

Origo runs as a stateless Go function, so it does not require a persistent MCP session store. A client needs an access key to use the hosted endpoint.

## Tool behavior

| MCP tool | CLI counterpart | Notes |
| --- | --- | --- |
| `read_link` | `read-link` | Optional `question` finds relevant passages and related sources. |
| `map_site` | `map-site` | Bounded URL inventory, filters, and continuation. |
| `research` | `research` | Finds data source operations; no paid execution. |
| `inspect` | `inspect` | Returns input/output contract and price. |
| `execute` | `execute` | May spend Alexandria credits. |

The tools advertise the MCP `readOnlyHint`, but **`execute` can still trigger paid provider calls**. Inspect the contract and price before using it.

## URL retrieval strategy

`read_link` tries increasingly general ways to represent a source:

1. GitHub's structured/raw data for recognized repository, file, issue, PR, commit, and CI URLs.
2. Direct text for JSON, YAML, OpenAPI, and similar source documents.
3. Published Markdown representations of normal documentation pages when available.
4. Firecrawl Scrape with main-content extraction and the provider's own 30-minute cache.
5. A more capable Firecrawl proxy or a short-lived Browser Sandbox session if extraction fails.

Question mode can follow relevant same-host documentation links and consult published `llms.txt` or OpenAPI materials. Native GitHub permissions and rate limits are not silently replaced with scraped versions of protected data. See [How URL reading works](/docs/architecture).

## Data execution and billing

`research` and `inspect` provide free discovery and schema lookup; `execute` runs paid Alexandria operations. The hosted server enforces a default **200-credit upper bound per request**. It validates input fields before submission, preserves provider-native results, and reports estimated and provider-reported usage separately.

Third-party provider terms can require human acceptance. The response supplies the terms URL; Origo does not accept it automatically.

For uncertain paid retries, reuse the original `request_id` and unchanged inputs. Identical replays on the same warm worker can use a 15-minute in-memory cache and avoid another upstream execution. Replays across separate workers are not guaranteed by the local cache; Firecrawl's upstream idempotency remains authoritative.

Origo has no custom Redis, KV, or persistent cache service. Firecrawl's 30-minute scrape cache is controlled by Firecrawl, not by the MCP server.

## Deployment

Two Vercel projects use the same GitHub repository but deploy independently:

| Surface | Project | Domain | Trigger |
| --- | --- | --- | --- |
| Documentation | `webctx-docs` | `webctx.ashray.xyz` | Relevant docs/site changes on `main` |
| Origo API | `origo-api` | `api.origo.ashray.xyz` | Retrieval/MCP changes on `main` |

The Origo entrypoint is `api/mcp.go`; MCP implementation lives in `pkg/origo` and the shared retrieval adapter in `pkg/retrieval`. The underlying engine is in `internal/app`. CLI release tags independently trigger GitHub binary builds and npm publication.

For contributor workflows, see [CONTRIBUTORS.md](https://github.com/amxv/webctx/blob/main/CONTRIBUTORS.md).
