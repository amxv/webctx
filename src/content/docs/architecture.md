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
internal/app/github_pull_views.go native PR files/commits/checks/diff/patch views
internal/app/github_commits.go native commits, compare, path history, blame
internal/app/github_actions.go native Actions workflows/runs/jobs/logs/artifacts
internal/app/github_refs.go   branches, tags, releases, forks, stars, watchers
internal/app/github_discussions_gists.go authenticated Discussions + public Gists
internal/app/github_search_profiles.go bounded Search + provider-resolved User/Organization profiles
internal/app/github_activity_deployments.go activity/statistics + deployment/environment/status history
internal/app/github_packages_projects.go exact Packages + REST Projects v2
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
- Pull Request files with SHA-256 diff selectors and provider-cap/patch-omission truth
- PR commit lists, Check Runs + commit statuses, targeted check annotations, and raw diff/patch media
- commit detail/comments with paginated files and raw commit diff/patch media
- comparisons with paginated commits plus first-page/300-file provider truth
- bounded path history with provider-backed slash-ref/path resolution
- authenticated GraphQL blame ranges
- bounded Actions workflow/run navigation, full selected-run jobs/artifacts, and exact job log reads
- redirected plaintext/ZIP log decoding with unavailable/expired state truth
- bounded branch/tag/release/fork/stargazer/subscriber navigation and exact release assets
- authenticated GraphQL Discussion conversations plus REST Gist files/comments/revisions/truncation handling
- bounded GitHub Search projections with separate Search-quota truth and provider-resolved profile tabs
- bounded repository activity, provider-computed statistics, and deployment environment/status history
- exact package/version views and bounded REST Projects v2 item projections
- provider-backed slash-ref/path resolution when the ref/path split is required
- REST request versioning, optional GitHub auth, response headers/status, Link pagination primitives, and GitHub-specific errors

Issue-specific rendering lives in `internal/app/github_issues.go`; PR-conversation rendering lives in `internal/app/github_pulls.go`; focused PR files/commits/checks/raw-media rendering lives in `internal/app/github_pull_views.go`; repository commit/compare/history/blame rendering lives in `internal/app/github_commits.go`; Actions rendering lives in `internal/app/github_actions.go`; ref/release/social navigation lives in `internal/app/github_refs.go`; Discussions/Gists live in `internal/app/github_discussions_gists.go`; Search/profile navigation lives in `internal/app/github_search_profiles.go`; activity/statistics/deployments live in `internal/app/github_activity_deployments.go`; exact Packages/Projects v2 live in `internal/app/github_packages_projects.go`. They reuse the same `GitHubTarget`, `GitHubClient`, native success/error/unsupported boundary, provider pagination, and ref-resolution/error rules while keeping resource responsibilities separate. The classifier claims only route families that have a faithful native reader; other GitHub pages retain generic fallback behavior, and GitHub security pages are intentionally excluded.

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
