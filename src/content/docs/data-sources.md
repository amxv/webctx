---
title: Research structured data
description: Discover data sources, inspect their exact API contracts, and execute queries without learning a new provider's syntax.
order: 14
category: Guides
summary: Research, inspect, and execute a huge catalogue of typed data operations directly from Webctx or Origo.
---

Webctx and Origo expose three progressively more powerful tools for Firecrawl Alexandria data. Normal documentation still uses `read-link` or `read_link`; structured-data research follows **research → inspect → execute**. Discovery and contract inspection are free. Only execution spends provider credits.

## 1. Research what is available

```bash
webctx research "podcast conversations about AI agents" --limit 5
```

The response lists human-readable operations, a stable ID (e.g. `particle/podcasts/episodes/search`), description, price, and a `next` object with the exact arguments for `inspect`.

To browse rather than search:

```bash
webctx research --view sources --limit 5
webctx research --mode catalogue --view operations --sources particle --limit 10 --offset 0
webctx research --mode catalogue --view operations --query "podcast conversations" --include inputs,output,examples --limit 5
webctx research --view groups --limit 5
```

`research` supports `query`, `urls`, `sources`, `categories`, `groups`, `operations`, `view`, `include`, `limit`, `offset`, `mode`, and `raw`. Views are `sources`, `groups`, and `operations`. Include controls are `inputs`, `output`, and `examples`. Set `mode=catalogue` to browse the full, pageable catalogue instead of using relevance-ranked discovery.

The `next` response contains the next tool name and arguments. Catalogue pagination retains the upstream continuation request as `native_request` for exact traceability.

## 2. Inspect an exact operation

```bash
webctx inspect particle/podcasts/episodes/search
```

Origo will return the upstream field names, types, required constraints, allowed values, defaults, response structure, examples where available, price, and an execution template:

```json
{
  "next": {
    "tool": "execute",
    "arguments": {
      "calls": [{
        "id": "particle/podcasts/episodes/search",
        "inputs": {
          "semantic_search": null,
          "limit": 25
        }
      }]
    }
  },
  "fill_before_execution": ["semantic_search"]
}
```

*Illustrative abbreviated contract.* Blank fields must be filled before the call; Origo never invents provider-specific query values. Use `inspect particle` to browse that source's operations instead of inspecting an individual contract.

## 3. Execute using real inputs

```bash
webctx execute particle/podcasts/episodes/search \
  --inputs '{"semantic_search":"AI agents","limit":2}'
```

Batch multiple data operations through one call (up to ten):

```bash
webctx execute --calls '[
  {"id":"fred-stlouisfed-org/economic-data/series_observations","inputs":{"series_id":"CPIAUCSL","limit":2}},
  {"id":"fred-stlouisfed-org/economic-data/series_observations","inputs":{"series_id":"UNRATE","limit":2}}
]'
```

For large JSON arguments use `--calls @requests.json` or `--inputs @inputs.json`.

The response contains untouched provider-native `data`, the operation ID, the data receipt ID, credits charged, and the `request_id`. If the provider supplies a continuation cursor, next-page offset, or an executable continuation, Origo returns a copy-ready `next` execute call. Call again using the next arguments; do not reuse the previous page's `request_id`.

## Credits, retries, and terms

Paid Alexandria queries execute automatically. Origo validates inputs against the inspected schema and checks estimated credits **before** submitting them. The hosted server enforces a **200-credit per-request limit in its Vercel environment**, with no user-facing budget toggle or parameter.

Responses show `estimated_credits` before execution and provider-reported `credits_used` afterward, including per-operation charges. Estimates are not audited billing totals; Firecrawl team/account limits remain authoritative. Per-record tools require a known upper bound, such as a `limit`.

Every paid request has an idempotency `request_id`. If the network fails and you need to retry the *same* request, reuse that ID. A different page or different query requires a new ID. The system does not automatically retry a potentially charged request.

If a provider requires third-party data terms, Origo returns the upstream acceptance URL and stops. It does not accept the terms or create an account on your behalf.

When a source publishes no examples, `inspect` generates a clearly labelled, schema-validated example when enough concrete inputs are available. It does not claim that generated examples came from the provider or were executed. FRED CPI observations include a note distinguishing price-index levels from year-over-year inflation percentages.

Use `--raw` or `"raw": true` to inspect the complete upstream Firecrawl response instead of the agent-friendly projection. This makes advanced provider features and their exact contracts available without requiring additional MCP tools.

## Origo MCP tools

The hosted MCP server at `api.origo.ashray.xyz` uses the same Go data engine as the CLI. Its five tools are `read_link`, `map_site`, `research`, `inspect`, and `execute`. All five advertise the MCP `readOnlyHint`; `execute` can nevertheless incur paid Alexandria charges. Unlike the CLI's federated web `search`, MCP `research` browses structured data sources.
