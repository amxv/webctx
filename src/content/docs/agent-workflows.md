---
title: Agent workflows
description: Use webctx as a terminal context tool for coding agents, research agents, and documentation agents.
order: 10
category: Start
summary: Practical ways to feed web search, markdown pages, and site maps into agent loops.
---

## Why agents like webctx

Agents often need web context in a form that is easy to paste, store, diff, summarize, or cite. webctx returns terminal-friendly text instead of browser UI state.

The three useful primitives are:

```text
search for candidate pages
read a page as markdown
map a site into URL candidates
```

## Research loop

```bash
webctx search "OpenAI Apps SDK MCP tool annotations"
webctx read-link https://developers.openai.com/apps-sdk/reference
```

Use this when an agent needs recent docs or implementation context before editing code.

## Documentation audit loop

```bash
webctx map-site https://docs.firecrawl.dev
webctx read-link https://docs.firecrawl.dev/introduction
webctx read-link https://docs.firecrawl.dev/api-reference/endpoint/scrape
```

Use this to gather the important pages before asking an agent to write or update docs.

## Repo understanding loop

```bash
webctx read-link https://github.com/amxv/webctx
webctx read-link https://github.com/amxv/webctx/blob/main/CONTRIBUTORS.md
webctx read-link https://github.com/amxv/webctx/blob/main/docs/porting-status.md
```

GitHub README and blob URLs use the raw-content fast path, which avoids the scraping provider when the content is public.

## Noise control

Use default and custom exclusions when web search is returning low-value result types:

```bash
webctx search "go cli release npm native binary" --exclude reddit.com,medium.com
```

Default exclusions already remove common video and social domains.

## Keyword targeting

When a query needs a very specific phrase, use Exa keyword mode:

```bash
webctx search "firecrawl api" --keyword "maxAge excludeTags"
```

The keyword is truncated to five words before being sent as Exa include-text criteria.
