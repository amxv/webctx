# webctx TypeScript -> Go porting status

This document is the handoff reference for future agents working on `amxv/webctx`.

## Goal

Port the CLI behavior from `amxv/webctx-ts` into pure Go while keeping the command-line interface and provider behavior effectively one-to-one for the CLI use case.

Scope intentionally excludes the MCP/server/dashboard pieces from the TypeScript repo.

## Source areas reviewed in `webctx-ts`

- `cli.ts`
- `tools/search.ts`
- `tools/read-link.ts`
- `tools/map-site.ts`
- `lib/search/brave.ts`
- `lib/search/tavily.ts`
- `lib/search/exa.ts`
- `lib/ranking.ts`
- `lib/utils.ts`
- `lib/scraping.ts`
- `lib/firecrawl-queue.ts`
- `lib/rate-limiter.ts`

## Completed

### CLI surface

Implemented in Go:

- `webctx search <query> [--exclude domain1,domain2] [--keyword phrase]`
- `webctx read-link <url>`
- `webctx map-site <url>`
- `webctx --help`
- `webctx --version`

Notes:

- `--version` now prints the bare version string, matching the TypeScript CLI.
- Error handling is exit-code based in Go rather than promise rejection based.

### Search port

Implemented:

- Brave HTTP client
- Tavily HTTP client using direct HTTP API instead of the TypeScript SDK
- Exa HTTP client
- provider fan-out with per-provider timeout
- duplicate-aware reranking
- excluded-domain filtering
- HTML entity decoding
- keyword truncation to 5 words for Exa include-text mode
- top 35 result output limit

Current behavior matches the TypeScript CLI design:

- normal search mode queries Brave + Tavily + Exa
- keyword mode queries Exa only
- user/domain exclusions are applied after provider collection, matching the TypeScript tool flow

### Read-link port

Implemented:

- GitHub raw-content fast path
- `.md` fast path with HEAD probe
- Firecrawl scrape fallback with the same agent-oriented request settings
- PDF parser enablement for `.pdf` URLs

Kept settings aligned with the TypeScript CLI:

- `formats: ["markdown"]`
- `onlyMainContent: true`
- `skipTlsVerification: true`
- `blockAds: true`
- `removeBase64Images: true`
- `maxAge: 600000`
- same excluded tags list

### Map-site port

Implemented with the same Firecrawl map settings:

- `sitemap: "include"`
- `includeSubdomains: true`
- `ignoreQueryParameters: true`
- `limit: 5000`

### Firecrawl queue / rate limiting

Implemented in Go:

- singleton-style queue wrapper
- token bucket limiter at 10 requests/minute
- serialized queue processing for Firecrawl operations

This is not a literal line-by-line port, but preserves the same operational intent.

### Release/publish setup

Updated to be release-ready for the real `webctx` CLI:

- GitHub Actions workflow now builds `webctx` instead of the old template placeholder
- release binaries embed the tagged version into `internal/buildinfo.Version`
- npm metadata now describes the actual CLI instead of the template
- README / agent / maintainer docs updated for the real repo

## Intentionally not ported

- MCP/server behavior
- Next.js app/dashboard code
- database/logging layers unrelated to the CLI

These can be added later only if explicitly requested.

## Current repo files of interest

- `cmd/webctx/main.go`
- `internal/app/app.go`
- `internal/app/tools.go`
- `internal/app/scrape.go`
- `internal/app/app_test.go`
- `.github/workflows/release.yml`
- `scripts/postinstall.js`
- `README.md`

## Verification already completed

- `go test ./...`
- `go build ./cmd/webctx`

## Live validation notes

Live CLI validation was run against a real `.env.local` on the Sprite machine.

Confirmed working live:

- combined `search` path returns real web results
- public GitHub blob `read-link` fast path works
- Firecrawl-backed `read-link` works
- Firecrawl-backed `map-site` works

Observed external/provider constraints during live validation:

- `search --keyword` currently depends on Exa-only results and could not be fully validated because the live Exa account returned `NO_MORE_CREDITS`
- private GitHub blob URLs are not readable via unauthenticated raw-content fetch, so they fall through to the general scrape path

These findings were from live provider behavior, not from compile/test failures in the Go port.

## Good next checks for future agents

1. Run live end-to-end checks against real provider keys for:
   - normal multi-provider search
   - Exa keyword-only search mode
   - GitHub raw-content read-link
   - `.md` fast path read-link
   - Firecrawl scrape fallback
   - Firecrawl map-site

2. Compare a handful of live outputs from `webctx-ts` and Go `webctx` for formatting parity.

3. If performance tuning is needed, focus on:
   - HTTP client reuse
   - provider timeout tuning
   - Firecrawl queue behavior under concurrent use

## Constraints to preserve

- Keep the CLI output simple and agent-friendly.
- Keep the Firecrawl request settings stable unless explicitly asked to change them.
- Keep the release asset naming contract stable unless postinstall/workflow are updated together.
