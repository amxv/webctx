# webctx

Web context for agents, from the terminal.

`webctx` gives you three small commands:

```bash
webctx search "agent web research"
webctx read-link <url>
webctx map-site <url>
```

The output is plain text or markdown, so it is easy to hand to ChatGPT, Codex, Claude Code, a shell script, or another tool. Search now retrieves relevant source excerpts automatically instead of returning only URLs. For a specific URL, add an optional question to explore related documentation and surface exact code and API details.

```bash
webctx search "Go MCP SDK read-only tool annotations"
webctx read-link https://docs.example.com/api --question "How do authentication and pagination work?"
```

## Origo: hosted MCP access

**Origo** is the MCP transport over the same WebCTX retrieval engine. Connect an MCP client to `https://api.origo.ashray.xyz/mcp?key=<your-private-key>` to access exactly two tools: `read_link` and `map_site`. Origo does not expose `search`; the CLI continues to provide all three commands. `read_link` accepts an optional `question` to retrieve focused source context while keeping ordinary URL-only reads intact.

Both surfaces share native GitHub/Markdown readers and Firecrawl fallback behavior. Firecrawl's built-in cache is limited to **30 minutes**. Difficult pages automatically escalate through Firecrawl's proxy fallback and, if necessary, a temporary Browser Sandbox session. There is no Redis or additional cache database.

Source-aware reading also prefers raw JSON, YAML, and OpenAPI files when the URL identifies one; question-focused reads can follow up to two relevant pages on the same host, including links found in `llms.txt`, and inspect embedded structured JSON. Firecrawl Alexandria tool matching and search discovery are automatic when supported, but **no paid Alexandria provider tools are executed**. The original source URLs remain explicit so an agent can verify facts.

Origo deploys independently through the `origo-api` Vercel project on pushes to `main`. CLI releases and npm publishing are unchanged. See the [Origo MCP guide](https://webctx.ashray.xyz/docs/origo) for architecture, setup, and security notes.

## Install

```bash
npm i -g webctx
webctx --help
```

Prebuilt binaries are also available from GitHub Releases.

## Why `read-link` is useful

Paste the URL you already have. webctx tries to understand what that URL means and returns the useful part instead of browser chrome.

```bash
# Repository overview + README preview
webctx read-link https://github.com/amxv/webctx

# Exact source lines
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#L1-L20'

# One directory
webctx read-link https://github.com/amxv/webctx/tree/main/internal/app

# Issue conversation
webctx read-link https://github.com/amxv/webctx/issues/6

# Pull request conversation
webctx read-link https://github.com/amxv/webctx/pull/15

# A single inline review thread
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'

# Files changed, commits, or checks
webctx read-link https://github.com/amxv/webctx/pull/15/files
webctx read-link https://github.com/amxv/webctx/pull/15/commits
webctx read-link https://github.com/amxv/webctx/pull/15/checks

# Exact Actions job + log when GitHub allows it
webctx read-link https://github.com/amxv/webctx/actions/runs/<run-id>/job/<job-id>
```

The same idea works for commits, comparisons, path history, blame, releases, Discussions, Gists, GitHub Search, profiles, Projects, deployments, and other supported GitHub views.

Large GitHub roots are intentionally navigation-first: webctx keeps authoritative metadata plus bounded previews/indexes and prints the exact GitHub URLs needed to go deeper. Copied source/comment/thread/diff/check/Gist selectors read the selected item, while explicit `.diff`/`.patch` URLs remain bulk raw representations. Provider pagination/ceilings and webctx's own local omissions are reported as separate facts.

For normal websites, webctx tries a clean markdown path first and falls back to Firecrawl when it needs rendered-page extraction. Exact public GitHub Package pages have one explicit best-effort exception: if GitHub's Package API rejects the read for auth or permission reasons, webctx can crawl the public page with Firecrawl and clearly labels the result as best-effort.

Recognized native GitHub auth, private/not-found, and rate-limit failures stay authoritative rather than being hidden by a page crawl. GitHub routes outside the native grammar can still use the normal website fallback path.

## Search

Normal search asks Brave, Tavily, and Exa, removes duplicate URLs, automatically reads three promising sources, follows up to two relevant links, and returns excerpts plus the ranked list. Alexandria tool discovery is included when available, without paid execution.

```bash
webctx search "next.js server components"
webctx search "react hooks" --exclude youtube.com,medium.com
webctx search "drizzle orm" --keyword "migration guide"
```

## Map a site

Use `map-site` when you want to discover the useful pages before reading them.

```bash
webctx map-site https://docs.firecrawl.dev
```

A common agent workflow is:

```bash
webctx map-site https://some-docs.example
webctx read-link https://some-docs.example/getting-started
webctx read-link https://some-docs.example/api/reference
```

## Credentials

The simplest local setup is a `.env.local` file:

```bash
BRAVE_API_KEY=...
TAVILY_API_KEY=...
EXA_API_KEY=...
FIRECRAWL_API_KEY=...
GH_TOKEN=...
```

- `BRAVE_API_KEY`, `TAVILY_API_KEY`, and `EXA_API_KEY` power normal `search`.
- `FIRECRAWL_API_KEY` powers `map-site` and page-crawl fallbacks.
- `GH_TOKEN` or `GITHUB_TOKEN` is optional. It increases GitHub capacity and unlocks reads such as blame, Discussions, some Actions job logs, private resources the token can access, and richer PR review state.

Environment variables, `.env.local`, and macOS Keychain are supported. `GH_TOKEN` takes precedence over `GITHUB_TOKEN`.

## Documentation

Start with the guides in `src/content/docs`:

- **Quickstart** — get useful output immediately
- **Read a URL** — GitHub-aware reading and normal web pages
- **Use webctx with agents** — practical research, repo, PR, and CI workflows
- **Search the web** — federated search and filtering
- **Map a site** — discover pages before reading them
- **Credentials** — keys and optional GitHub auth
- **How URL reading works** — native/direct fast paths before Firecrawl
- **How search ranking works** — URL normalization, position scoring, and provider agreement

Maintainer notes, repository layout, development commands, and release steps live in [`CONTRIBUTORS.md`](CONTRIBUTORS.md).
