---
title: Rate limits and concurrency
description: Per-key request budgets, local coordination, retry behavior, and overrides.
order: 22
category: Reference
summary: How webctx shares capacity across keys and concurrent coding agents.
---

WebCTX is designed for multiple agents launching separate CLI processes on one computer. Its **capacity-aware round-robin load balancer** distributes requests across multiple API keys based on available capacity, helping maximize throughput even on free tiers. **All processes using the same key share that key's local rate-limit state**, while keys from separate provider accounts have independent budgets.

## Default limits

The scheduler uses these published free-tier request limits as defaults (October 2026). Provider plans and API policies can change; the upstream service remains authoritative.

| Provider / operation | Default rate | Local quota scope |
| --- | ---: | --- |
| Tavily Search | 100/minute | Per key |
| Exa Search | 10/second | Per key/team |
| Brave Search | 50/second | Per key/plan |
| Firecrawl Scrape | 10/minute | Per key |
| Firecrawl Map | 10/minute | Per key |
| Firecrawl Search | 10/minute | Per key |
| Firecrawl Crawl creation | 2/minute | Per key |
| Firecrawl Crawl status | 500/minute | Per key |
| Firecrawl browser-session creation | 2/minute | Per key |
| Firecrawl browser execute | 10/minute | Per key |
| Firecrawl concurrent browsers | 2 | Per key |

These are **request-rate controls**, not monthly credit allocations. Individual plans may also limit credits, jobs, or account usage. There are no extra locally imposed hourly or daily search request caps; do not infer that a provider has none.

## Search key scheduling

Each incoming `search` requests capacity from every enabled provider in parallel. For a given provider:

1. Read the available keys, deduplicate them, and identify each by a one-way hash.
2. Under a short file lock, inspect each key's sliding-window capacity and cooldown.
3. Select an eligible key in rotating order and **reserve its request slot before sending HTTP**.
4. On a key-specific `429` or credential/quota failure, cool down that key and consider another healthy key. A 400-class invalid query is not retried.
5. If every key is busy, wait briefly for an imminent slot; otherwise let the other search providers return results.

Brave's `X-RateLimit-Policy` response header can lower or raise the learned one-second limit **for that specific key**. `Retry-After` and relevant reset headers guide cooldowns. The default admission wait is 1,500 milliseconds. The HTTP request still has its own timeout once dispatched.

## Firecrawl scheduling

Scrape, map, search, and crawl creation use separate rolling-minute budgets for each Firecrawl account. Where an operation uses the Browser Sandbox, it also needs an available browser slot on that account.

Stateless operations can choose another account with available capacity. A `429` pauses the affected **account and endpoint**, without blocking that account's other endpoints or another account. Browser sessions and crawl-job-specific operations stay on their originating account. Charged Alexandria execution stays on the selected primary account rather than moving between keys.

The Firecrawl queue holds file locks only during admission. It does not hold them while waiting for the network. Browser slot locks stay held while their operations or sessions are active.

## Cross-process scope

By default, state lives in the current OS user's cache directory. Separate CLI processes on the **same machine, using the same state directory and key** coordinate with file locks. Secrets are not written to the state files; only hashes and timing information are retained.

This is **not** a distributed rate limiter. Another machine, another OS user with a different cache directory, or an independent serverless worker may use the same upstream account without seeing this machine's reservations. The provider may still return a `429`.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `WEBCTX_TAVILY_RPM` | `100` | Per-key Tavily search request cap per minute. |
| `WEBCTX_EXA_QPS` | `10` | Per-key Exa search requests per second. |
| `WEBCTX_BRAVE_QPS` | `50` | Initial Brave requests per second; an observed per-key policy can override it. |
| `WEBCTX_SEARCH_QUEUE_WAIT_MS` | `1500` | Maximum local admission wait for a search provider; valid 0–30000. |
| `WEBCTX_SEARCH_KEY_BALANCER_DIR` | OS user cache | Shared search key and rate-state directory. |
| `WEBCTX_FIRECRAWL_QUEUE_DIR` | OS user cache | Shared Firecrawl queues and browser locks. |

Set custom rates only to match your actual account plan. Each key's backend quotas still apply independently. These overrides do not increase credits or provider limits.

## Failure behavior

- **`429`**: Honor upstream retry guidance; cool down the rejected key or endpoint and try an eligible independent account when safe.
- **Authentication or exhausted credits**: Keep the failed key out of rotation temporarily; another healthy account may continue.
- **Invalid request**: Surface the error instead of repeating a request that no account can satisfy.
- **Ambiguous paid Firecrawl failure**: Do not blindly switch accounts and risk a second charge.

See [Credentials](/docs/credentials) for adding keys and [Troubleshooting](/docs/troubleshooting) for common failures.
