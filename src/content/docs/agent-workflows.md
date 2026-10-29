---
title: Use webctx with agents
description: Practical web research and GitHub workflows for coding agents.
order: 13
category: Guides
summary: Find a source, narrow the URL, and pass clean context to your agent.
---

webctx outputs plain text, Markdown, or JSON. An agent can call it from a terminal, read the result directly, or save it to a file. No custom prompt format is required.

## Find technical documentation

```bash
webctx search "Go context cancellation best practices"
webctx read-link https://go.dev/doc/
```

Search gives you candidate links and snippets. Read the most useful URLs to check the actual source text before relying on a claim.

For a focused API question:

```bash
webctx read-link https://docs.example.com/api --question "What are the required request fields?"
```

## Understand a repository

```bash
webctx read-link https://github.com/amxv/webctx
webctx read-link https://github.com/amxv/webctx/tree/main/internal/app
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/tools.go#L130-L220'
```

Start with a compact overview, then open a directory or the exact lines you need.

## Review a pull request

```bash
webctx read-link https://github.com/amxv/webctx/pull/15
webctx read-link https://github.com/amxv/webctx/pull/15/files
webctx read-link https://github.com/amxv/webctx/pull/15/checks
```

Follow a comment or review-thread URL from the output when you need a specific conversation:

```bash
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'
```

## Debug CI

```bash
webctx read-link https://github.com/owner/repo/actions/runs/123
webctx read-link https://github.com/owner/repo/actions/runs/123/job/456
```

A run gives a job overview; a job link gives its steps and an available log preview. This is more manageable than collecting every job log at once.

## Explore a documentation site

```bash
webctx map-site https://some-docs.example --query "authentication"
webctx read-link https://some-docs.example/api/authentication
```

Map broadly, then read selectively. `map-site` paginates large URL lists instead of flooding the next agent call.

## Query structured data

```bash
webctx research "podcast episodes mentioning AI agents"
webctx inspect particle/podcasts/episodes/search
```

Check the schema and price before running `execute`: it can spend paid credits. See [Research structured data](/docs/data-sources).

## Give your agent less noise

A specific link is usually better than a broad page. Prefer `#L80-L120` over a whole file, a copied review-thread anchor over an entire PR, and an Actions job over a complete workflow run. Follow exact URLs printed by webctx rather than inventing a special pagination flag.

For every recognized GitHub URL format, see [GitHub URL reference](/docs/github-urls). For the command syntax, see [CLI reference](/docs/cli-reference).
