---
title: How URL reading works
description: Native sources, direct Markdown, and Firecrawl fallbacks.
order: 27
category: Reference
summary: The retrieval strategy behind read-link and Origo read_link.
---

One command handles different source types:

```bash
webctx read-link <url>
```

webctx chooses the most faithful available representation before attempting page extraction.

```text
URL
 ├─ recognized structured GitHub URL → native/raw provider read
 ├─ raw JSON, YAML, OpenAPI, or text → direct fetch
 ├─ clean published Markdown available → direct Markdown fetch
 └─ ordinary rendered webpage → Firecrawl Scrape
                                  ↳ enhanced proxy if necessary
                                  ↳ temporary browser extraction if blocked
                                      ↓
                              readable Markdown/text
```

This avoids paying to scrape a page when the source already offers accurate structured or readable content.

## Native GitHub reads

GitHub URLs can refer to repositories, directories, source ranges, issues, pull requests, reviews, diffs, commits, blame, CI jobs, releases, Discussions, Gists, Projects, and other recognized resources.

- **Overview URLs** return a bounded index and links to exact children.
- **Exact selectors** target the specified source lines, comment, review thread, diff hunk, or job.
- **Raw `.diff` / `.patch` links** preserve the requested bulk representation.

A compact overview can omit items even when GitHub has returned them; webctx distinguishes these omissions from upstream pagination and rate limits. Some exact machine-generated content, such as large CI logs, can still be previewed with links for further reading.

See [GitHub URL reference](/docs/github-urls) for exact forms and limitations.

## Direct text and Markdown

Before crawling a normal URL, webctx checks whether an authentic text/Markdown version is available. For documentation sites, this can include published `.md` variants and directory `index.md` pages. It can also read raw structured documents such as JSON, YAML, and OpenAPI definitions.

`--question` is a separate, bounded exploration step: it selects relevant source sections, can follow related same-host documentation links, and may inspect `llms.txt`, OpenAPI documents, or public embedded JSON when useful. A normal URL-only read does not trigger that extra research loop.

## Firecrawl fallback

When a direct read is unavailable, webctx asks Firecrawl to extract the main content. Firecrawl's own scrape cache is set to **30 minutes**. Challenge screens and empty responses can trigger enhanced extraction or a temporary browser session. Browser extraction is best-effort, not a guarantee that blocked sites are accessible.

`map-site` uses Firecrawl to discover URLs rather than extracting every discovered page. Firecrawl requests are scheduled against per-key quotas across local CLI processes; see [Rate limits and concurrency](/docs/rate-limits).

## Permissions remain authoritative

If a URL is recognized as a native GitHub resource, a permission, private/not-found, or rate-limit error from GitHub is not hidden behind a generic HTML scrape. An unsupported GitHub URL shape can use the ordinary webpage path instead.

The narrow exception is a **public GitHub Package URL** whose Package API rejects authentication/permission: if Firecrawl is configured, webctx may return a clearly labeled best-effort page extraction.

A GitHub token can improve limits and unlock more resource types, but it never bypasses GitHub's permissions.

## Search is a different operation

`search` queries Brave, Tavily, and Exa, ranks URLs, and returns provider-supplied snippets. **It does not run `read-link` on its results**. This separation keeps discovery fast and avoids using Firecrawl credits on every possible result.

The same core readers power webctx CLI and Origo MCP. Origo exposes `read_link`, `map_site`, and structured-data tools, but not the CLI's federated `search`.

For the result scoring model, see [How search ranking works](/docs/ranking).
