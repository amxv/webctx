---
title: Search the web
description: Find and rank relevant web pages across multiple search providers.
order: 10
category: Guides
summary: One query, one deduplicated list of useful links.
---

## Search across providers

```bash
webctx search "Go context cancellation"
```

webctx searches enabled Brave, Tavily, and Exa providers in parallel. It combines the results, removes duplicate URLs, and favors relevant documentation, API references, and official sources for technical queries. Any one configured provider is enough to search.

Each result has a link, a title, and a short snippet or Exa highlight supplied by its search provider. **Search does not automatically read or scrape the linked pages.** Open a useful result with `read-link` when you need the actual source content:

```bash
webctx read-link https://go.dev/doc/
```

## Refine results

Exclude domains you do not want:

```bash
webctx search "react effects" --exclude youtube.com,medium.com
```

Look for a known phrase:

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

Keyword mode prefers Exa with a quoted phrase. If Exa is unavailable, Brave and Tavily provide a fallback search. The resulting pages are **not all verified to contain the phrase in their full text**.

## Choose search providers

```bash
webctx status
webctx providers disable exa
webctx providers enable exa
```

Provider preferences persist between runs. `status` reports whether a key is configured, not how many credits remain.

## Multiple agents and API keys

You can configure multiple API keys from **separate accounts** for Brave, Tavily, Exa, and Firecrawl. WebCTX's **capacity-aware round-robin load balancer** routes requests to keys with available quota instead of blindly rotating, maximizing throughput under free-tier limits while coordinating all agents on the same machine. A throttled account does not block a healthy one; if an entire search provider is busy, other providers can still return results.

Start with [Credentials](/docs/credentials); see [Rate limits and concurrency](/docs/rate-limits) for limits, file locks, cooldowns, and overrides.

## Learn more

- [How search ranking works](/docs/ranking)
- [CLI reference](/docs/cli-reference)
- [Use webctx with agents](/docs/agent-workflows)
