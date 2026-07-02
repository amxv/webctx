# AGENTS.md

Guidance for coding agents working in `webctx`.

## Purpose

This repo contains the pure Go port of the `webctx` CLI.

It is no longer a generic starter template. Treat the current CLI behavior as the source of truth unless the user explicitly asks to change it.

## Architecture

- `cmd/webctx/main.go`: process entrypoint, exit-code based dispatch.
- `internal/app/app.go`: CLI parsing and top-level command routing.
- `internal/app/tools.go`: search provider clients, ranking, formatting, and HTTP helpers.
- `internal/app/scrape.go`: GitHub raw-content optimization, `.md` fetch path, Firecrawl queue, and env loading.
- `internal/app/app_test.go`: unit tests for CLI behavior and core helpers.
- `bin/webctx.js`: npm shim that invokes the packaged native binary.
- `scripts/postinstall.js`: downloads release binary on install, falls back to `go build`.
- `.github/workflows/release.yml`: tag-driven release pipeline.
- `docs/porting-status.md`: progress log and remaining work for future agents.

## Local commands

Use `make` targets:

- `make fmt`
- `make test`
- `make vet`
- `make lint`
- `make check`
- `make build`
- `make build-all`
- `make install-local`

Direct commands:

- `go test ./...`
- `go vet ./...`
- `npm run lint`

## Current CLI contract

Preserve these commands unless the user explicitly asks to change them:

- `webctx search <query> [--exclude domain1,domain2] [--keyword phrase]`
- `webctx read-link <url>`
- `webctx map-site <url>`
- `webctx --version`

Behavioral expectations:

- `search` combines Brave, Tavily, and Exa results, then re-ranks them with duplicate-aware scoring.
- `read-link` keeps the current GitHub raw-content fast path, `.md` fast path, and Firecrawl fallback settings.
- `map-site` keeps the current Firecrawl map request settings.
- The CLI should remain agent-friendly and emit plain markdown/text output.

## How to change things safely

1. Keep binary naming convention unchanged unless you also update postinstall/workflow:
- release assets: `<cli>_<goos>_<goarch>[.exe]`
- npm-installed binary path: `bin/<cli>-bin` (or `.exe` on Windows)

2. If changing search behavior, compare against the TypeScript porting notes in `docs/porting-status.md` first.

3. If adding dependencies, commit `go.sum` and make sure the workflow still passes on a clean checkout.

4. If you change release artifacts or version plumbing, update `Makefile`, `.github/workflows/release.yml`, and `scripts/postinstall.js` together.

## Release contract

Release pipeline triggers on `v*` tags and expects:

- `NPM_TOKEN` GitHub secret present.
- npm package name in `package.json` is publishable under your account/org.
- repository URL matches the release origin used by `scripts/postinstall.js`.

Release binaries should embed the tagged version into `internal/buildinfo.Version` so `webctx --version` matches the release tag.

## Guardrails

- Prefer additive changes and keep the CLI output stable.
- Do not silently change Firecrawl request settings unless the user explicitly wants behavioral changes.
- Do not reintroduce MCP/server code unless requested; this repo is intentionally CLI-only.

## Changelog Guidelines

When cutting a release, update `src/content/docs/changelog.md` before tagging.

- Add a new section for the exact version tag being released.
- Keep the newest version at the top.
- Skip versions that do not have git tags.
- Use commit history and diffs on `main` to summarize code changes.
- This is an OSS project, so internal code changes may be included when useful.
- Do not include docs-site-only changes such as site styling, Zuedocs/package bumps, deploy plumbing, footer/layout changes, or documentation navigation changes.
- Rewrite commit subjects into clear release notes instead of pasting raw commit messages.
- If a release contains only tagging/release metadata, write: `Maintenance release. No direct code behavior changes beyond release preparation.`
