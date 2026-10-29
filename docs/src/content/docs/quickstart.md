---
title: Quickstart
description: Install webctx and run your first search, URL read, and site map.
order: 1
category: Start
summary: A few commands to get useful context into your agent.
---

## Install webctx

```bash
npm install -g webctx
webctx --version
```

## Choose a task

**Find documentation:**

```bash
webctx search "Go context cancellation"
```

Search queries Brave, Tavily, and Exa when enabled and configured. You get one ranked list with short source snippets. It does **not** automatically open every result page.

**Read a URL:**

```bash
webctx read-link https://github.com/amxv/webctx
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#L1-L20'
```

The first link returns a repository overview; the second asks for specific source lines. For normal web pages, webctx tries clean text and falls back to Firecrawl extraction.

**Find pages on a site:**

```bash
webctx map-site https://docs.firecrawl.dev
```

`map-site` returns a manageable page list, not an unbounded crawl of every URL.

**Ask a focused question:**

```bash
webctx read-link https://docs.example.com/api --question "How does pagination work?"
```

Question mode looks for relevant sections and can follow a small set of related documentation links.

## Configure credentials

You only need keys for the capabilities you use:

| Task | Credentials |
| --- | --- |
| Search | One or more of `BRAVE_API_KEY`, `TAVILY_API_KEY`, `EXA_API_KEY` |
| Map sites or extract ordinary web pages | `FIRECRAWL_API_KEY` |
| Discover, inspect, or execute structured data | `FIRECRAWL_API_KEY` |
| Read most public GitHub URLs | Usually no token needed; `GH_TOKEN` helps with rate limits and permissions |

For example, put the keys you use into a `.env.local` file or your environment:

```dotenv
BRAVE_API_KEY=your-key
FIRECRAWL_API_KEY=your-key
```

Check what's configured:

```bash
webctx status
```

See [Credentials](/docs/credentials) for Keychain setup and multiple accounts.

## Work with structured data

Use three commands when you need data records rather than pages:

```bash
webctx research "podcast conversations about AI agents"
webctx inspect particle/podcasts/episodes/search
webctx execute particle/podcasts/episodes/search --inputs '{"semantic_search":"AI agents","limit":2}'
```

`research` and `inspect` discover available operations and their schemas. **`execute` uses paid Firecrawl Alexandria credits.** Read [Research structured data](/docs/data-sources) before running paid queries.

## Where next?

- [Use webctx with agents](/docs/agent-workflows) for real workflows.
- [Read a URL](/docs/read-link) for GitHub and website examples.
- [CLI reference](/docs/cli-reference) for every flag.
