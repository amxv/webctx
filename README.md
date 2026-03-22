# webctx

`webctx` is a pure Go CLI for agent-friendly web search and page extraction.

It ports the CLI behavior from the TypeScript/Bun `webctx-ts` repository into a release-ready Go codebase that ships through both GitHub Releases and npm.

## What it does

- `search`: combines Brave, Tavily, and Exa search results, deduplicates them, and re-ranks them using the same scoring logic as the original TypeScript CLI
- `read-link`: returns clean markdown for a single URL using a fast GitHub raw-content path, a `.md` fast path, and Firecrawl scraping fallback
- `map-site`: returns a sitemap-style list of URLs and metadata from Firecrawl with the same agent-oriented defaults as the original CLI

The CLI deliberately focuses on the command-line tool behavior. The MCP/server code from the TypeScript repo is not part of this Go port.

## Install

Global npm install:

```bash
npm i -g @amxv/webctx
webctx --help
```

Build from source:

```bash
git clone https://github.com/amxv/webctx.git
cd webctx
make build
./dist/webctx --help
```

## Commands

```bash
webctx --help
webctx --version
webctx search <query> [--exclude domain1,domain2] [--keyword phrase]
webctx read-link <url>
webctx map-site <url>
```

Examples:

```bash
webctx search "next.js server components"
webctx search "react hooks" --exclude youtube.com,vimeo.com
webctx search "drizzle orm" --keyword "migration guide"
webctx read-link https://docs.example.com/guide
webctx map-site https://example.com
```

## Environment variables

The CLI loads `.env.local` when present and reads provider credentials from the environment.

Required by command:

- `search`
  - `BRAVE_API_KEY`
  - `TAVILY_API_KEY`
  - `EXA_API_KEY`
- `read-link`
  - `FIRECRAWL_API_KEY` for non-GitHub / non-`.md` URLs
- `map-site`
  - `FIRECRAWL_API_KEY`

## Release and distribution

This repo publishes in two ways:

- GitHub Releases for native binaries
- npm for `npm i -g @amxv/webctx`

The release workflow triggers on `v*` tags and does the following:

1. runs Go and Node quality checks
2. builds cross-platform binaries
3. creates a GitHub Release with those assets
4. publishes the npm package using the tag version

## Project layout

- `cmd/webctx/main.go`: CLI entrypoint
- `internal/app/`: CLI parsing, search, ranking, scrape, and Firecrawl queue logic
- `internal/buildinfo/`: build-time version plumbing for `--version`
- `bin/webctx.js`: npm shim that invokes the packaged native binary
- `scripts/postinstall.js`: downloads the release binary on install and falls back to local `go build`
- `.github/workflows/release.yml`: tag-driven release pipeline
- `docs/porting-status.md`: progress log for the TypeScript-to-Go CLI port
- `AGENTS.md`: guidance for coding agents
- `CONTRIBUTORS.md`: maintainer/release notes

See `AGENTS.md`, `CONTRIBUTORS.md`, and `docs/porting-status.md` for the repo-specific implementation and maintenance details.
