---
title: Map a site
description: Discover a site's pages before deciding which ones to read.
order: 12
category: Guides
summary: Explore a documentation site without flooding an agent with URLs.
---

## Find pages

```bash
webctx map-site https://docs.firecrawl.dev
```

The default output shows up to 30 relevant URLs with titles and descriptions when available. It is a preview, not a promise that every page has been discovered.

## Narrow the list

```bash
webctx map-site https://docs.firecrawl.dev --query "REST API" --limit 20
webctx map-site https://docs.firecrawl.dev --path-prefix /api/ --offset 20
```

Use `--query` to rank pages by a topic, `--path-prefix` to focus on a section, and `--offset` to browse the next page. You can also change the result count with `--limit` (1 to 100) and include translated pages with `--language all`.

## Read what matters

```bash
webctx map-site https://some-docs.example
webctx read-link https://some-docs.example/getting-started
webctx read-link https://some-docs.example/api/reference
```

The map helps your agent decide where to go. `read-link` retrieves the selected page.

`map-site` requires a Firecrawl API key. Additional keys from separate accounts can be configured for higher concurrent throughput. See [Credentials](/docs/credentials), [CLI reference](/docs/cli-reference), and [Rate limits](/docs/rate-limits).
