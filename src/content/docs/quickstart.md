---
title: Quickstart
description: Install webctx, add the keys you need, and get useful web context in a few commands.
order: 1
category: Start
summary: Install it, paste a URL, and start giving agents cleaner web context.
---

## Install

```bash
npm i -g webctx
webctx --help
```

## Try the document commands

Search the web:

```bash
webctx search "agent web research"
```

Turn a URL into useful text:

```bash
webctx read-link https://github.com/amxv/webctx
```

Discover the pages on a site:

```bash
webctx map-site https://docs.firecrawl.dev
```

These three return plain text or markdown that can go straight into an agent, a file, a pipe, or your clipboard.

For structured data from statistical agencies, podcast indexes, finance or other live sources, use `research` to discover operations, `inspect` to view their exact contract, and `execute` to retrieve provider data. See [Research structured data](/docs/data-sources).

Search automatically retrieves relevant source sections alongside the links. You can also ask a question about any URL, and Webctx will follow the most relevant documentation references automatically:

```bash
webctx read-link https://docs.example.com/api --question "How does pagination work?"
```

## The part worth trying first

`read-link` understands many GitHub URLs instead of scraping the whole GitHub page.

```bash
# Just these source lines
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#L1-L20'

# Just this Issue and its conversation
webctx read-link https://github.com/amxv/webctx/issues/6

# A PR as a review conversation
webctx read-link https://github.com/amxv/webctx/pull/15

# A single inline review thread
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'
```

The URL already carries your intent. webctx tries to preserve that intent and return only the useful context.

## Add credentials

Create `.env.local` in the directory where you normally run webctx:

```bash
BRAVE_API_KEY=...
TAVILY_API_KEY=...
EXA_API_KEY=...
FIRECRAWL_API_KEY=...
GH_TOKEN=...
```

You do not need every key for every command:

| What you want to do | Key |
| --- | --- |
| Normal `search` | `BRAVE_API_KEY`, `TAVILY_API_KEY`, `EXA_API_KEY` |
| Map sites or crawl normal pages | `FIRECRAWL_API_KEY` |
| More GitHub capacity / auth-only GitHub views | `GH_TOKEN` or `GITHUB_TOKEN` |

Many public GitHub reads work without a GitHub token. Add one when you want higher limits, blame, Discussions, some Actions logs, private resources your token can access, or richer review-thread state.

## A simple agent loop

```bash
webctx search "next.js caching changes"
webctx read-link https://nextjs.org/docs/app/building-your-application/caching
webctx read-link https://github.com/vercel/next.js/issues/<issue-number>
```

Search finds candidates **and retrieves evidence**. `read-link` lets your agent inspect exact URLs or dig deeper with an optional question. Your agent gets source-grounded context instead of browser UI.
