---
title: How search ranking works
description: See how webctx combines Brave, Tavily, and Exa into one deterministic result list.
order: 31
category: How it works
summary: Provider position and independent agreement turn three search result sets into one useful list.
---

## Why combine search engines?

Different search providers are good at different things. A page that appears near the top of several independent result sets is often a stronger candidate than a page surfaced by only one provider.

A normal search therefore runs Brave, Tavily, and Exa together:

```bash
webctx search "durable agent runtimes"
```

Then webctx turns those result sets into one list.

## The ranking pipeline

```text
Brave ───┐
Tavily ──┼─→ remove noisy domains → normalize URLs → score positions → reward agreement → rank
Exa ─────┘
```

The final output is capped at 35 results so a research query does not flood an agent's context.

## 1. Normalize URLs

Before deduplicating, webctx normalizes URLs so tracking differences do not make the same page look unique.

For example, these should represent one page:

```text
https://example.com/guide?utm_source=search
https://example.com/guide?ref=homepage
https://example.com/guide/
```

Common tracking parameters such as `utm_*`, `ref`, `fbclid`, and `gclid` are removed. Meaningful query parameters are preserved.

## 2. Reward high positions

A result gets more points when a provider ranks it highly.

The first positions use this curve:

| Position | Points |
| ---: | ---: |
| 1 | 30 |
| 2 | 27 |
| 3 | 24 |
| 4 | 21 |
| 5 | 19 |
| 6 | 16 |
| 7 | 13 |
| 8 | 11 |
| 9 | 9 |
| 10 | 7 |
| 11 | 5 |
| 12 | 4 |
| 13 | 3 |
| 14 | 2 |
| 15+ | 1 |

Brave, Tavily, and Exa use the same provider weight, so the ranking is driven primarily by **where a page appears and how many providers agree on it**.

## 3. Reward independent agreement

Suppose the providers return this:

```text
Brave   #2  https://docs.example.com/agent-runtime
Tavily  #4  https://docs.example.com/agent-runtime
Exa     #1  https://other.example.com/runtime-guide
```

The repeated `docs.example.com` URL accumulates position points from both Brave and Tavily and receives an additional duplicate bonus.

That is intentional: two independent providers surfacing the same page is useful evidence.

Agreement near the top of the result sets receives a slightly stronger bonus than agreement on low-ranked results.

## 4. Keep output stable

If final scores tie, webctx keeps the original insertion order. This makes the output deterministic and easier to diff or reuse in agent workflows.

## Exclusions happen before ranking

Default noisy domains and anything supplied with `--exclude` are removed before scoring:

```bash
webctx search "react effects" --exclude medium.com,dev.to
```

That means excluded pages cannot consume rank or earn agreement bonuses.

## Keyword mode is intentionally different

`--keyword` is not a federated ranking mode:

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

It uses Exa include-text search when you care about a short phrase appearing on the page. Use normal search when you want cross-provider discovery and agreement; use keyword mode when you already know the textual clue you are looking for.

## A practical research loop

The ranking system is meant to find candidates, not produce the final research context.

```bash
webctx search "agent tool sandbox architecture"
webctx read-link https://the-best-looking-result.example/guide
```

Search tells you **what looks worth reading**. `read-link` turns the selected URL into the context you actually give the agent.
