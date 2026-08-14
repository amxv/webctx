---
title: Search the web
description: Search Brave, Tavily, and Exa together and get one clean list of useful pages.
order: 10
category: Guides
summary: Find candidate pages without manually comparing three search engines.
---

## Search normally

```bash
webctx search "golang http client retries"
```

webctx asks Brave, Tavily, and Exa, removes duplicate URLs, and returns one markdown list with short summaries.

When several providers independently surface the same page, that agreement helps the page rise in the final list.

## Remove noisy sites

```bash
webctx search "react useEffect cleanup" --exclude medium.com,dev.to
```

webctx already excludes common video and social sites such as YouTube, TikTok, Instagram, and Facebook. `--exclude` adds your own domains for one search.

## Look for a specific phrase

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

`--keyword` uses Exa's include-text search when you care more about a phrase appearing on the page than broad provider agreement.

## Use the results with `read-link`

A good research loop is:

```bash
webctx search "OpenAI Apps SDK MCP annotations"
webctx read-link https://developers.openai.com/apps-sdk/reference
```

Search is for discovery. `read-link` is for turning the promising result into context an agent can actually use.

## Output

```markdown
Total Results: 12

- [Result title](https://example.com/page)
    - Short summary of the page
```

The final list is intentionally bounded so search does not flood an agent's context.
