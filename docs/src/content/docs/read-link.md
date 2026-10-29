---
title: Read a URL
description: Get useful text from webpages, documentation, and precise GitHub links.
order: 11
category: Guides
summary: Paste a link and let webctx choose how to read it.
---

## Read a webpage

```bash
webctx read-link https://docs.firecrawl.dev/introduction
```

webctx tries a direct source or clean Markdown version first. When it needs to extract an ordinary rendered webpage, it uses Firecrawl. The response includes the source URL and readable content instead of navigation and other page clutter.

For sites that require extraction, configure a [Firecrawl key](/docs/credentials).

## Ask a question about a page

```bash
webctx read-link https://docs.example.com/api --question "How do I authenticate requests?"
```

With `--question`, webctx finds relevant sections, preserves useful code examples, and may follow a small number of related documentation links. It identifies the sources it used and points out missing or unreadable references.

Without the question flag, webctx reads the specified URL without that extra exploration.

## Use the GitHub URL you already have

GitHub links often identify something more specific than a webpage. webctx recognizes many of them and reads the underlying source directly.

```bash
# Repository overview
webctx read-link https://github.com/amxv/webctx

# One directory
webctx read-link https://github.com/amxv/webctx/tree/main/internal/app

# Only the linked source lines
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#L1-L20'

# An issue and its conversation
webctx read-link https://github.com/amxv/webctx/issues/6

# One code review thread
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'

# One CI job
webctx read-link https://github.com/cli/cli/actions/runs/123456/job/7890
```

Repository, issue, and PR overview pages keep responses manageable and provide direct links for drilling down. A copied line-range or comment anchor targets that specific item. Bulk `.diff` and `.patch` links remain raw change text.

For more GitHub link types, see the [GitHub URL reference](/docs/github-urls).

## When a read is incomplete

webctx marks provider pagination, omitted overview content, blocked pages, and missing permissions rather than silently presenting partial information as complete. Follow the direct links in its response if you need a narrower or fuller resource.

Many public GitHub pages work without authentication. A `GH_TOKEN` helps with private repos, blame, some logs, Discussions, and higher GitHub API limits.

For details about direct reads, crawl fallbacks, and known exceptions, see [How URL reading works](/docs/architecture) or [Troubleshooting](/docs/troubleshooting).
