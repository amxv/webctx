---
title: How search ranking works
description: Scoring and deduplication across Brave, Tavily, and Exa.
order: 28
category: Reference
summary: How provider positions, agreement, and technical relevance affect search results.
---

A normal `webctx search` asks the available enabled Brave, Tavily, and Exa accounts in parallel. It returns one ranked Markdown list of at most **35 URLs** with titles and provider-supplied snippets. No result pages are automatically read.

## Ranking steps

```text
Brave ───┐
Tavily ──┼─→ exclude domains → normalize URLs → score positions + agreement → quality bonus → rank
Exa ─────┘
```

### 1. Exclude unwanted domains

Default noisy domains and anything from `--exclude` are removed before scoring:

```bash
webctx search "react effects" --exclude medium.com,dev.to
```

### 2. Normalize URLs

Tracking differences should not make duplicate pages look unique. For example:

```text
https://example.com/guide?utm_source=search
https://example.com/guide?ref=homepage
https://example.com/guide/
```

These normalize to the same URL. Common tracking parameters are removed; meaningful parameters remain.

### 3. Score position and independent agreement

Every enabled search provider has the same scoring weight. Higher positions get more points:

| Position | Points | Position | Points |
| ---: | ---: | ---: | ---: |
| 1 | 30 | 8 | 11 |
| 2 | 27 | 9 | 9 |
| 3 | 24 | 10 | 7 |
| 4 | 21 | 11 | 5 |
| 5 | 19 | 12 | 4 |
| 6 | 16 | 13 | 3 |
| 7 | 13 | 14 | 2 |

Positions 15 and later get 1 point. A URL found by more than one provider accumulates points from each appearance and receives an extra agreement bonus (stronger when it appears in a top-five position).

### 4. Add a bounded technical relevance bonus

For coding-oriented searches, local scoring also considers title-term matches, documentation URLs, API/SDK paths, and known first-party sources tied to the searched ecosystem. Not every GitHub result is considered official: the repository organization and query terms need to match.

These are deterministic local checks. No additional network requests or page crawls are made to compute the bonus.

### 5. Preserve stable order

Equal final scores keep original insertion order. When multiple providers return a URL, Exa's useful highlight text can be kept even when another provider supplied the strongest title.

## Keyword searches

`--keyword` is different from normal multi-provider discovery. It prefers Exa with the phrase quoted in the query. If Exa is unavailable, Brave searches the quoted phrase and Tavily can enforce `exact_match`. This does **not** mean every resulting Exa/Brave page has been checked for the phrase in its full text.

## Search versus reading

The search APIs provide candidate URLs and snippets. Read a selected source with `webctx read-link` when you need actual page evidence. See [Search the web](/docs/search) for examples and [Rate limits](/docs/rate-limits) for provider admission behavior.
