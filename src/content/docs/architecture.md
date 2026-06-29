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
internal/app/tools.go       search, read-link, map-site, ranking, provider calls
internal/app/scrape.go      GitHub raw path, markdown path, Firecrawl queue, env loading
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

`internal/app/tools.go` contains provider-specific requests:

- Brave web search
- Tavily search
- Exa search
- Firecrawl scrape
- Firecrawl map

It also contains output formatting, result ranking, excluded-domain filtering, HTML entity decoding, URL normalization, and missing-credential errors.

## Scrape helpers

`internal/app/scrape.go` contains the fast paths and credential loading:

- parse GitHub repository, blob, and tree URLs
- convert GitHub file URLs to raw content URLs
- fetch root repository README files
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
