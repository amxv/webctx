---
title: Structured data reference
description: Firecrawl Alexandria discovery, contracts, execution, pricing, and retries.
order: 25
category: Reference
summary: Advanced parameters and charging semantics for research, inspect, and execute.
---

`research`, `inspect`, and `execute` provide progressive access to Firecrawl Alexandria's data operations. Discovery and schema inspection are free; **execution is paid**. Both the CLI and Origo MCP use the same underlying engine.

## Discovery modes and filters

```bash
webctx research "podcast conversations" --limit 5
webctx research --view sources --limit 5
webctx research --view groups --limit 5
webctx research --mode catalogue --view operations --sources particle --limit 10 --offset 0
webctx research --mode catalogue --view operations --query "podcast conversations" --include inputs,output,examples
```

`research` supports `query`, `urls`, `sources`, `categories`, `groups`, `operations`, `view`, `include`, `limit`, `offset`, `mode`, and `raw`. Views are `sources`, `groups`, and `operations`; include controls are `inputs`, `output`, and `examples`.

- **Ranked discovery** finds relevant operations by meaning. It has no native offset pagination.
- **Catalogue mode** browses the pageable inventory. For a filtered source with over 100 operations, local relevance ranking applies only within the fetched page. The response flags incomplete ranking and provides the next page.
- **Exact operation IDs** take precedence over descriptive filters.

Responses include machine-readable next-step arguments; catalogue browsing can include the upstream `native_request` for traceability.

## Inspect an operation

```bash
webctx inspect particle/podcasts/episodes/search
```

An inspection returns upstream field names, types, required/optional inputs, allowed values, defaults, response shape, price, and examples when available. It also provides a suggested next call:

```json
{
  "next": {
    "tool": "execute",
    "arguments": {
      "calls": [{
        "id": "particle/podcasts/episodes/search",
        "inputs": {"semantic_search": null, "limit": 25}
      }]
    }
  },
  "fill_before_execution": ["semantic_search"]
}
```

*Illustrative shortened response.* Required inputs with `null` are placeholders, not runnable values. If a source has no published examples, webctx may generate clearly labeled illustrative shapes without claiming they came from the provider.

Use `webctx inspect particle` to browse a source's operations. Add `--raw` to see the upstream response in full.

## Execution and pagination

```bash
webctx execute particle/podcasts/episodes/search --inputs '{"semantic_search":"AI agents","limit":2}'
```

To run up to ten operations in one call:

```bash
webctx execute --calls @requests.json
```

Each operation executes independently. One provider's terms or validation rejection does not need to discard other successful items. Results contain native `data`, operation IDs, provider receipt information, and estimated/reported credits. Firecrawl charges are distinguished from provider-specific data fields also named `credits`.

When the provider supplies a continuation cursor, offset, page token, `search_after`, or continuation link, WebCTX provides a copyable `next` call. **Use a new request ID for each new page or changed query.**

## Credit limits and safety

The hosted Origo server enforces a default **200-credit per-request ceiling** before paid execution. It validates the supplied inputs and requires bounds for per-record operations when necessary. There is no user-facing argument to raise the hosted ceiling. A deployment may configure a lower limit.

The response can show `estimated_credits` and provider-reported `credits_used`. Estimates are not audited billing statements. Provider-account credits and third-party charges remain authoritative.

If a source requires acceptance of third-party terms, the response supplies the upstream terms URL. WebCTX does not accept terms or create provider accounts automatically.

## Retries and idempotency

Paid requests have an idempotency `request_id`. If an execution has an uncertain network result, retry **the same inputs with the same ID**. Do not use that ID for another query or another page. WebCTX does not automatically replay a potentially charged operation.

Within one warm Origo worker, identical successful/partial replays can be served from a 15-minute in-memory cache. Such a replay reports `replayed: true` and `credits_charged_this_request: 0`. The cache prevents repeated upstream execution in that worker, but does **not** prove that a replay on another worker avoided new charges. Concurrent uses of one ID are serialized within a worker.

Mixed-operation batches return successful items alongside failures. After resolving a failed provider's access, execute only the failed operation with a **new** request ID instead of resubmitting already successful paid items.

Rate-limit errors may provide `retry_after_seconds`, `retry_at`, and `retryable` fields. Free catalogue lookups can use short warm-worker caches and local throttling, but upstream quotas still apply.

## Advanced output

`--raw` or `"raw": true` requests the complete upstream Firecrawl response instead of the normal concise projection. It may contain extra fields and provider-specific detail.

The MCP `readOnlyHint` metadata is not a promise that an operation is free: `execute` is still capable of incurring paid credits.

See [Research structured data](/docs/data-sources), [CLI reference](/docs/cli-reference), and [Origo technical reference](/docs/origo-reference).
