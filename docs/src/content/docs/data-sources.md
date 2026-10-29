---
title: Research structured data
description: Find a dataset, inspect its inputs, and run a query.
order: 14
category: Guides
summary: Use Firecrawl Alexandria for structured data beyond webpages.
---

When you need records rather than webpages, use **research → inspect → execute**. webctx provides access to Firecrawl Alexandria operations without requiring you to learn each data provider's API.

**Important:** Finding and inspecting operations is free. **Executing them uses paid Firecrawl Alexandria credits.**

## 1. Find an operation

```bash
webctx research "podcast conversations about AI agents" --limit 5
```

webctx searches available data operations and gives you IDs you can inspect. You can also browse by source:

```bash
webctx research --view sources --limit 5
webctx research --mode catalogue --view operations --sources particle --limit 10
```

## 2. See what it needs

```bash
webctx inspect particle/podcasts/episodes/search
```

The response explains the exact input fields, required values, response format, and estimated price. It also provides a suggested `execute` call with any missing inputs clearly identified.

## 3. Query the source

```bash
webctx execute particle/podcasts/episodes/search \
  --inputs '{"semantic_search":"AI agents","limit":2}'
```

Execution returns structured JSON, the source operation ID, credit information, and pagination hints when available. For multiple operations you can send a batch:

```bash
webctx execute --calls '[
  {"id":"fred-stlouisfed-org/economic-data/series_observations","inputs":{"series_id":"CPIAUCSL","limit":2}},
  {"id":"fred-stlouisfed-org/economic-data/series_observations","inputs":{"series_id":"UNRATE","limit":2}}
]'
```

Each operation is handled independently, so a rejected item need not prevent other items from succeeding.

## Before running a paid query

Check the price and inputs shown by `inspect`. webctx validates requests and enforces a per-request credit ceiling. If a provider requires additional terms, webctx shows the provider's acceptance URL; it does not agree on your behalf.

If you need to retry after an uncertain network failure, keep the original request ID and unchanged inputs. For new data or another page, use a new ID. More on safe retries, schemas, pagination, prices, batches, and provider terms: [Structured data reference](/docs/data-reference).

## Related guides

- [Origo MCP](/docs/origo) for accessing these operations from an MCP client.
- [Credentials](/docs/credentials) to set up Firecrawl.
- [CLI reference](/docs/cli-reference) for flags and JSON input files.
