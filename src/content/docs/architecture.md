---
title: How URL reading works
description: See how webctx chooses the cleanest way to read a URL before falling back to a full page crawl.
order: 30
category: How it works
summary: Native GitHub reads, direct markdown, and Firecrawl form an automatic cost-and-quality ladder.
---

## One command, different paths

You always run the same command:

```bash
webctx read-link <url>
```

webctx chooses how to read the URL for you.

```text
URL
 │
 ├─ supported structured source? ──→ read it directly
 │   GitHub repos, source, Issues, PRs, CI, history, ...
 │
 ├─ clean markdown available? ─────→ fetch the markdown
 │
 └─ otherwise ─────────────────────→ crawl the rendered page with Firecrawl

                                   ↓
                           useful markdown/text
```

The goal is simple: **do the cheapest faithful thing first, and only crawl a page when crawling is actually necessary.**

## Why this matters

A browser page contains much more than the thing an agent asked for: navigation, menus, buttons, avatars, sidebars, repeated metadata, scripts, and layout text.

If the URL already identifies the useful resource, webctx can skip that noise completely.

For example:

```bash
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#L1-L20'
```

That URL already means “these source lines.” Fetching the source directly is faster and gives a cleaner result than rendering the GitHub page and trying to recover those lines afterward.

The same idea applies to more structured GitHub URLs:

```bash
# One Issue conversation
webctx read-link https://github.com/amxv/webctx/issues/6

# One PR review thread
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'

# One Actions job and its log
webctx read-link https://github.com/<owner>/<repo>/actions/runs/<run-id>/job/<job-id>
```

webctx treats the URL as a selector, not merely an address.

## GitHub fast paths

GitHub is the deepest optimization because its URLs carry a lot of meaning and GitHub exposes structured/raw provider data for many resources.

webctx can use those sources to return focused context for things such as:

- repository overviews and directory listings
- source files, line ranges, and Markdown sections
- Issues and exact comments
- PR conversations, review threads, diffs, commits, and checks
- commit history, comparisons, and blame
- Actions runs, jobs, logs, and artifacts
- releases, Discussions, Gists, Search, profiles, Projects, and deployments

This has three useful properties for agents:

1. **Less context.** The response contains the selected resource instead of GitHub UI chrome.
2. **More precision.** Fragments such as source lines, Issue comments, review threads, and diff anchors stay meaningful.
3. **Better truthfulness.** Provider limits, missing permissions, pagination, expired logs, and incomplete results can be surfaced directly instead of being hidden by a rendered page.

You do not need to learn separate webctx subcommands for these resources. Copy the GitHub URL and use `read-link`.

## Direct markdown before crawling

GitHub is not the only fast path.

For normal sites, webctx checks whether a clean markdown representation is available before asking Firecrawl to render and extract the page.

```bash
webctx read-link https://some-docs.example/guide
```

If the site exposes a usable markdown version, webctx can fetch that directly. Otherwise it moves on to the crawler.

This is especially useful for documentation sites where the authored text is already available without browser rendering.

## Firecrawl is the general fallback

When there is no faithful native or direct-text path, webctx uses Firecrawl:

```bash
webctx read-link https://example.com/article
```

Rendered-page extraction is the broadest path. It handles ordinary websites that need HTML cleanup or browser-style extraction, while the faster paths handle resources that can be represented more precisely.

`map-site` also uses Firecrawl when you want to discover URLs before reading them:

```bash
webctx map-site https://docs.example.com
```

## One deliberate best-effort exception

GitHub Package pages are unusual: a Package can be publicly visible in the browser while GitHub's Package API rejects the available credential.

For an exact public Package URL, webctx first prefers the structured API. If that fails specifically because of authentication or permission and Firecrawl is configured, it can crawl the public Package page instead.

```bash
webctx read-link https://github.com/orgs/<org>/packages/container/package/<name>
```

That output is visibly marked **best-effort** because page extraction can contain UI noise or be incomplete.

webctx does not use this escape hatch to hide GitHub rate limits, private/not-found resources, or unrelated native GitHub failures.

## Authentication improves the fast path

Most public GitHub reads do not require a token. Adding `GH_TOKEN` or `GITHUB_TOKEN` gives webctx more GitHub capacity and can unlock richer reads such as:

- blame
- Discussions
- some Actions job logs
- private resources your token can access
- resolved/outdated PR review-thread state

Fine-grained tokens can be narrower than GitHub's public surface. For public endpoints where GitHub permits it, webctx can retry without Authorization if the token itself is rejected.

## Architecture at a glance

The whole CLI is intentionally small:

```text
search    ──→ Brave + Tavily + Exa ──→ normalize + rank ──→ markdown links
read-link ──→ native/direct fast paths ──→ Firecrawl fallback ──→ markdown/text
map-site  ──→ Firecrawl map ──→ URL inventory
```

That is the product architecture: **find good pages, read them with the most faithful available path, and keep the result easy to hand to another tool.**

For repository layout, build details, or implementation code, see [`CONTRIBUTORS.md`](https://github.com/amxv/webctx/blob/main/CONTRIBUTORS.md) or the source itself.
