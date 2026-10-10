---
title: Map a site
description: Discover the useful URLs on a site before deciding which pages to read.
order: 12
category: Guides
summary: Turn a docs or product site into a navigable URL inventory.
---

## Basic usage

```bash
webctx map-site https://docs.firecrawl.dev
```

The result is a **bounded preview of 30 URLs** (not the entire site), with titles and descriptions where available. This prevents thousands of links from flooding an agent's context.

For filtered and paginated structured output:

```bash
webctx map-site https://docs.firecrawl.dev --limit 20 --query "REST API"
webctx map-site https://docs.firecrawl.dev --limit 20 --offset 20 --path-prefix /api/
webctx map-site https://docs.firecrawl.dev --language all --limit 30
```

The MCP `map_site` tool accepts `url`, optional `query`, `limit` (1–100, default 30), `offset`, `path_prefix`, and `language` (default `en`; `all` includes translated pages).

Each result includes `total_discovered`, `total_matching`, bounded `links` objects and a copy-ready `next` call when another page exists. Source mapping may itself be incomplete if the provider's 5,000-URL ceiling is reached.

## Why map first?

A docs site can contain hundreds or thousands of pages. An agent usually does not need all of them.

Use `map-site` to see the shape of the site, then read only the pages that matter:

```bash
webctx map-site https://some-docs.example
webctx read-link https://some-docs.example/getting-started
webctx read-link https://some-docs.example/api/reference
```

This keeps research broad at the discovery step and focused at the reading step. Full inventories remain accessible by paging rather than adding thousands of URLs to one model context.

## Credential

`map-site` uses:

```text
FIRECRAWL_API_KEY
```
