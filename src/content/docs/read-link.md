---
title: Read-link command
description: Convert a URL into clean markdown using GitHub raw-content fast paths, direct markdown detection, and Firecrawl fallback scraping.
order: 5
category: Commands
summary: The behavior of `webctx read-link`.
---

## Basic usage

```bash
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
```

`read-link` returns a markdown document. If a title can be found, output starts with an H1, then the original URL, then the extracted content.

## GitHub fast path

For GitHub file URLs, webctx converts the page URL into a raw GitHub URL before using any scraping provider.

Repository root URLs are treated as README requests. If `README.md` is not found, webctx also tries `readme.md`, `Readme.md`, and `README`.

Tree URLs are not treated as files and fall through to the other paths.

## Direct markdown path

For direct markdown-style URLs, webctx checks whether a `.md` document is available. If the given URL does not end in `.md`, it tries the same URL with `.md` appended.

The HEAD response must look like markdown or plain text and have enough content length to be useful. Then webctx fetches the markdown directly and derives the title from the first `#` heading when possible.

## Firecrawl fallback

When the fast paths do not work, webctx uses Firecrawl:

```text
endpoint: https://api.firecrawl.dev/v2/scrape
formats: markdown
onlyMainContent: true
skipTlsVerification: true
blockAds: true
removeBase64Images: true
maxAge: 600000
```

The request excludes common non-content tags such as scripts, styles, navigation, footers, headers, asides, SVGs, images, and ad selectors.

## PDF handling

If the URL ends in `.pdf`, webctx asks Firecrawl to use the PDF parser.

## Rate limiting

Firecrawl scrape requests pass through a process-local queue with a token bucket limiter. It starts with 10 tokens and refills one token every six seconds. The queue keeps scrape calls serialized so agent workflows do not burst into Firecrawl.
