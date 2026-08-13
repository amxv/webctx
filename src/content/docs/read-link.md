---
title: Read-link command
description: Convert URLs into clean text with native GitHub repository/source reads, direct markdown detection, and Firecrawl fallback scraping.
order: 5
category: Commands
summary: The behavior of `webctx read-link`.
---

## Basic usage

```bash
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
```

`read-link` returns terminal-friendly markdown/text. Normal pages keep the familiar title, original URL, and extracted content shape; native GitHub repository and tree views use compact structured metadata instead of scraped GitHub chrome.

## Native GitHub repository reads

Repository roots such as:

```bash
webctx read-link https://github.com/amxv/webctx
```

return a compact frontmatter-style block with high-signal repository metadata, followed by a README preview targeted at roughly 5,000 Unicode characters. The preview is cut at a safe Markdown boundary where possible, and invisible HTML comments from the human repository view are removed.

If the README is longer, webctx includes GitHub's canonical README blob URL for the full source. The root output also includes a short set of useful source/tree URL forms supported by the native reader.

## Blob reads and selectors

Public blob URLs keep the direct raw-content fast path:

```bash
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
```

A direct blob is an explicit source request, so it returns the full text file by default rather than applying the repository-root preview cap. Source HTML comments are preserved.

GitHub line fragments narrow source output:

```bash
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/app.go#L20'
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/app.go#L20-L40'
```

For Markdown files, a heading fragment returns that heading through the next heading of equal or higher level:

```bash
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#install'
```

Unknown, reversed, or out-of-range selectors fail explicitly instead of silently returning a different section or the whole file. Binary files are identified without dumping arbitrary bytes. GitHub's 100 MB Contents/Git-blob provider boundary is surfaced instead of presenting an incomplete source as complete.

## Tree reads

Tree URLs return one directory level rather than repository navigation chrome:

```bash
webctx read-link https://github.com/amxv/webctx/tree/main/internal
```

When the selected directory contains a README, webctx adds a bounded human-view preview and a full blob URL if that preview is truncated. A 1,000-entry Contents API result is marked as potentially incomplete because that is GitHub's documented directory ceiling.

Blob and tree refs are not split at the first slash. Slash-containing branches/tags are resolved against GitHub when ref/path identity is needed, and genuinely overlapping valid splits fail as ambiguous rather than being guessed.

## Optional GitHub authentication

Public repository/source reads work anonymously. To read private source that your account can access, configure `GH_TOKEN` or `GITHUB_TOKEN`; when both contain values, `GH_TOKEN` wins.

Native GitHub API requests send GitHub's pinned REST version header and preserve provider status/rate-limit information. Recognized repository/blob/tree failures remain GitHub-specific, so a private/not-found/rate-limited source is not silently replaced by a scraped login or error page.

GitHub routes that do not yet have a native reader continue through the normal fallback chain. Security pages are intentionally outside native GitHub handling.

## Direct markdown path

For direct markdown-style URLs, webctx checks whether a `.md` document is available. If the given URL does not end in `.md`, it tries the same URL with `.md` appended.

The HEAD response must look like markdown or plain text and have enough content length to be useful. Then webctx fetches the markdown directly and derives the title from the first `#` heading when possible.

## Firecrawl fallback

When no native/direct-markdown path handles the URL, webctx uses Firecrawl:

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
