---
title: Architecture
description: Learn how the Go CLI, app package, provider clients, scrape helpers, npm shim, postinstall script, and release pipeline fit together.
order: 8
category: Internals
summary: The code map for webctx maintainers and agent contributors.
---

## Repository layout

```text
cmd/webctx/main.go          CLI entrypoint
internal/app/app.go         argument parsing and command dispatch
internal/app/tools.go       search, read-link dispatch, map-site, ranking, provider calls
internal/app/github.go      native GitHub routing, provider client, source/tree rendering
internal/app/github_issues.go native Issues, comments, lists, labels, milestones, relationships
internal/app/github_pulls.go native PR conversations, reviews, inline threads, exact anchors
internal/app/scrape.go      direct markdown path, Firecrawl queue, env loading
internal/buildinfo          build-time version plumbing
bin/webctx.js               npm executable shim
scripts/postinstall.js      release binary downloader and Go build fallback
Makefile                    build, test, release, local install targets
docs/porting-status.md      parity notes from the TypeScript implementation
```

## CLI entrypoint

`cmd/webctx/main.go` passes `os.Args[1:]` to `app.Run` and exits with the returned code.

The CLI intentionally avoids a heavy framework. `internal/app/app.go` handles:

- `--help`
- `--version` and `-v`
- `search`
- `read-link`
- `map-site`
- unknown-command errors
- `--exclude` and `--keyword` flag parsing

## Provider clients

`internal/app/tools.go` contains search/scrape provider requests and the top-level `read-link` fallback chain:

- Brave web search
- Tavily search
- Exa search
- Firecrawl scrape
- Firecrawl map

It also contains output formatting, result ranking, excluded-domain filtering, HTML entity decoding, URL normalization, and missing-credential errors.

## Native GitHub reader

`internal/app/github.go` owns the GitHub-native boundary for the route families webctx can represent faithfully. It currently handles:

- repository roots with compact metadata and bounded README previews
- public raw blob reads plus line/range and Markdown-heading selectors
- authenticated/private blob fallback through the GitHub Contents API
- one-level tree listings and directory README previews
- Issue conversations and exact `#issuecomment-...` selectors
- bounded Issue/search/label/milestone list and detail views
- current parent/sub-issue, dependency, and Issue-field relationships
- Pull Request conversations, formal reviews, REST-grouped inline review threads, and exact PR anchors
- optional authenticated GraphQL enrichment for resolved/outdated PR review-thread state
- provider-backed slash-ref/path resolution when the ref/path split is required
- REST request versioning, optional GitHub auth, response headers/status, Link pagination primitives, and GitHub-specific errors

Issue-specific rendering lives in `internal/app/github_issues.go`; PR-conversation rendering lives in `internal/app/github_pulls.go`. Both reuse the same `GitHubTarget`, `GitHubClient`, native success/error/unsupported boundary, provider pagination, and human-body sanitization responsibilities. The classifier claims only route families that have a faithful native reader. Focused PR files/commits/checks views remain separate until their native readers land; other GitHub pages retain generic fallback behavior, and GitHub security pages are intentionally excluded.

## Scrape and credential helpers

`internal/app/scrape.go` contains the generic direct/fallback paths and credential loading:

- detect direct markdown availability
- queue Firecrawl scrape requests with a token bucket limiter
- load `.env.local` files
- load missing keys from macOS Keychain

## npm packaging

`bin/webctx.js` is the npm-facing command. It invokes the native binary installed next to it.

`scripts/postinstall.js` downloads a prebuilt GitHub Release asset named for the current platform and architecture. If that fails and Go is available, it builds `./cmd/webctx` locally.

## Versioning

`internal/buildinfo` provides the runtime version. Builds set it with Go linker flags:

```bash
-X github.com/amxv/webctx/internal/buildinfo.Version=0.1.1
```

The Makefile and postinstall fallback both pass that value so `webctx --version` matches the package release.
