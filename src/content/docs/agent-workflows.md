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
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/app.go#L130-L180'
webctx read-link https://github.com/amxv/webctx/blob/main/docs/porting-status.md
webctx read-link https://github.com/amxv/webctx/tree/main/internal/app
webctx read-link https://github.com/amxv/webctx/issues/6
webctx read-link 'https://github.com/amxv/webctx/issues?q=is%3Aissue'
```

The repository root is an orientation read: compact metadata, a bounded README preview, and native navigation hints. Direct public blobs use the raw-content fast path and return the full source; line fragments narrow a file before it reaches the agent. Tree URLs return a one-level listing without GitHub navigation chrome. Issue URLs preserve substantive conversation, while Issue/search/label/milestone list views remain compact and bounded.

When you already have an exact Issue comment URL, keep its `#issuecomment-...` fragment. webctx resolves that comment directly instead of reading the whole Issue first.

For private repository source, configure optional `GH_TOKEN` or `GITHUB_TOKEN`. Public GitHub reads stay anonymous by default.

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
