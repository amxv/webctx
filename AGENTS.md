# AGENTS.md

Guidance for coding agents working in `webctx`.

## Purpose

This repo contains the Go `webctx` CLI and its hosted Origo MCP transport, sharing one retrieval engine.

It is no longer a generic starter template. Treat the current CLI behavior as the source of truth unless the user explicitly asks to change it.

## Architecture

- `cmd/webctx/main.go`: process entrypoint, exit-code based dispatch.
- `internal/app/app.go`: CLI parsing and top-level command routing.
- `internal/app/tools.go`: search provider clients, ranking, formatting, and HTTP helpers.
- `internal/app/scrape.go`: GitHub raw-content optimization, `.md` fetch path, and env loading. Shared Firecrawl admission control lives in `internal/app/firecrawl_queue.go` and OS-specific file locking helpers.
- `pkg/retrieval/`: public adapter to `internal/app` for Vercel's Go packaging restrictions.
- `pkg/origo/`: authenticated Streamable HTTP MCP with exactly `read_link` and `map_site`.
- `api/mcp.go`: Go Function entrypoint for independent Origo deployment.
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
- `webctx status` and `webctx providers enable|disable <brave|tavily|exa>`
- `webctx keys list|add|remove` for macOS Keychain-managed keys, including Firecrawl; `BRAVE_API_KEYS`, `TAVILY_API_KEYS`, `EXA_API_KEYS`, and `FIRECRAWL_API_KEYS` also work via environment variables
- `webctx read-link <url>`
- `webctx map-site <url>`
- `webctx --version`

Behavioral expectations:

- `search` combines enabled Brave, Tavily (`advanced` depth), and Exa results, then re-ranks them with duplicate-aware scoring and bounded technical-source quality bonuses. It does not auto-read pages, follow links, or consume Firecrawl credits. Exa uses `auto` and content highlights without deprecated `livecrawl` or forced `maxAgeHours`; Firecrawl's read-link freshness behavior is unchanged. `--keyword` uses quoted phrases instead of deprecated Exa `includeText`; fallback Tavily searches enable `exact_match`. Do not label Exa/Brave results as full-text-verified.
- Regular `search` does not run Alexandria discovery. Structured-data discovery belongs to `research`, which already queries Alexandria; explicit `read-link` preserves Firecrawl domain metadata behavior.
- Firecrawl supports `FIRECRAWL_API_KEYS` alongside the singular key. Every key represents a **different account**. Scrape/Map/Search (10 RPM each), Crawl creation (2 RPM), Crawl status (500 RPM), browser-session creation (2 RPM), and two concurrent browser slots are tracked per key by file-backed queues. Stateless requests rotate toward keys with capacity and retry explicit upstream rejections on other eligible accounts. Stateful browser/crawl-job operations stay on their originating key. Paid Alexandria executes deliberately stay on the primary singular key, or first plural key if no singular key exists, so idempotency/replays never cross accounts. Per-key `429` cooldowns and browser slots are shared across all WebCTX processes of the OS user. Locks are never held during network I/O. Override queue directory with `WEBCTX_FIRECRAWL_QUEUE_DIR`; independent machines/serverless workers are not centrally coordinated.
- Search uses atomic, cross-process **per-key** capacity-aware admission even for one key; see `internal/app/search_rate_limits.go`, `search_key_balance.go`, and `search_keys.go`. All configured keys belong to separate accounts. Defaults: Tavily Development 100 RPM per key, Exa 10 QPS per key/team, Brave 50 QPS per key/plan (adapted individually from that key's `X-RateLimit-Policy`). Rotate across eligible keys after reserving capacity. Each `429` cools down only the rejected key, then falls back to another healthy account. Auth/quota/transport failures can fail over, but invalid requests cannot. `Retry-After` and Brave reset headers are honored. An exhausted provider waits up to 1.5s by default, then contributes a partial failure rather than delaying the others. The on-disk state contains only key hashes, timing, and window counts, never secrets. Directory: `WEBCTX_SEARCH_KEY_BALANCER_DIR`; overrides: `WEBCTX_TAVILY_RPM`, `WEBCTX_EXA_QPS`, `WEBCTX_BRAVE_QPS`, `WEBCTX_SEARCH_QUEUE_WAIT_MS`.
- `read-link` keeps the current GitHub raw-content and `.md` fast paths. Firecrawl uses its built-in 30-minute cache, automatic and enhanced proxies, then browser extraction as a final fallback.
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

- The `NPM_TOKEN` GitHub Actions secret must contain a valid npm access token with publish rights for `webctx`; publishing uses `NODE_AUTH_TOKEN` (not OIDC trusted publishing).
- npm package name in `package.json` is publishable under your account/org.
- repository URL matches the release origin used by `scripts/postinstall.js`.

Release binaries should embed the tagged version into `internal/buildinfo.Version` so `webctx --version` matches the release tag.

## Guardrails

- Prefer additive changes and keep the CLI output stable.
- Do not silently change Firecrawl request settings unless the user explicitly wants behavioral changes.
- Do not add search to Origo's MCP tool surface; only `read_link` and `map_site` are registered.
- Keep `origo-api` deployment separate from the existing WebCTX docs Vercel project and tag-driven CLI releases.
- Never introduce Redis, custom KV, or another caching service for Origo: the user explicitly chose Firecrawl's own 30-minute cache.

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

### Docs deployment

The Docs GitHub Actions workflow deploys production only for the watched site paths; Vercel Git auto-deployments and `ignoreCommand` stay disabled. Keep the project-scoped `VERCEL_TOKEN` Actions secret and ID variables configured. Check Astro locally; builds run in CI.

### Origo deployment

The separate `Origo API` GitHub Actions workflow deploys the Go function only for changes to its import graph and Vercel configuration. Use `VERCEL_ORIGO_TOKEN` and `VERCEL_ORIGO_PROJECT_ID` exclusively for Origo; never substitute the WebCTX Docs token or a static `--prebuilt` site bundle. Keep the `/mcp` rewrite and Origo production environment variables intact.
