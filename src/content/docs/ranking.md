---
title: Search ranking
description: Understand URL normalization, provider weighting, duplicate bonuses, excluded-domain filtering, and result caps.
order: 7
category: Internals
summary: How webctx turns provider results into a single ranked list.
---

## Pipeline

The normal search pipeline is:

1. query Brave, Tavily, and Exa concurrently
2. collect each provider's results
3. filter default and custom excluded domains
4. normalize URLs for deduplication
5. score by provider, position, and duplicates
6. return the top 35 results

If all providers fail because credentials are missing, the error lists the missing key names. If providers fail for other reasons, the error includes provider-specific failure messages.

## URL normalization

webctx lowercases scheme and host, removes trailing slashes from paths, and strips common tracking parameters:

```text
utm_source
utm_medium
utm_campaign
utm_term
utm_content
ref
fbclid
gclid
```

Other query parameters are preserved and sorted through Go's URL encoding.

## Position points

Higher provider positions receive more points. The first positions use this score table:

```text
1: 30
2: 27
3: 24
4: 21
5: 19
6: 16
7: 13
8: 11
9: 9
10: 7
11: 5
12: 4
13: 3
14: 2
15 and below: 1
```

## Provider weights

Brave, Tavily, and Exa currently use a weight of `1.0`. The ranking function also recognizes a `Ref` provider weight of `1.25`, which preserves parity with the older TypeScript ranking model.

## Duplicate bonus

When the same normalized URL appears from more than one provider, webctx adds a duplicate bonus. URLs that rank in the top five get a stronger duplicate bonus than lower-ranked results.

If a URL appears more than three times, a small duplicate penalty is applied to avoid over-rewarding repeated entries.

## Stable ordering

When final scores tie, URLs are sorted lexicographically. That keeps output deterministic for tests and easier to compare in agent workflows.
