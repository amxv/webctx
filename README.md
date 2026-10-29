# webctx

Web context for agents, from the terminal.

`webctx` provides six commands: three for web documents and three for structured data.

```bash
webctx search "agent web research"
webctx read-link <url>
webctx map-site <url>
webctx research "podcast conversations about AI agents"
webctx inspect particle/podcasts/episodes/search
webctx execute particle/podcasts/episodes/search --inputs '{"semantic_search":"AI agents","limit":2}'
```

The output is plain text or markdown, so it is easy to hand to ChatGPT, Codex, Claude Code, a shell script, or another tool. Search now retrieves relevant source excerpts automatically instead of returning only URLs. For a specific URL, add an optional question to explore related documentation and surface exact code and API details.

```bash
webctx search "Go MCP SDK read-only tool annotations"
webctx read-link https://docs.example.com/api --question "How do authentication and pagination work?"
```

## Origo: hosted MCP access

**Origo** is the MCP transport over the same WebCTX retrieval engine. Connect an MCP client to `https://api.origo.ashray.xyz/mcp?key=<your-private-key>` to access five tools: `read_link`, `map_site`, `research`, `inspect`, and `execute`. Origo deliberately omits federated web `search` but exposes the complete progressive data-research workflow. `read_link` accepts an optional `question` and keeps URL-only behavior unchanged.

Both surfaces share native GitHub/Markdown readers and Firecrawl fallback behavior. Firecrawl's built-in cache is limited to **30 minutes**. Difficult pages automatically escalate through Firecrawl's proxy fallback and, if necessary, a temporary Browser Sandbox session. There is no Redis or additional cache database.

Source-aware reading also prefers raw JSON, YAML, and OpenAPI files when the URL identifies one; question-focused reads can follow relevant references, including links found in `llms.txt`, and inspect embedded structured JSON. The new `research`, `inspect`, and `execute` commands give agents direct control over Firecrawl Alexandria's catalogue and paid data operations without learning provider-specific transport syntax. The original source URLs remain explicit so an agent can verify facts.

## Structured data: discover → inspect → execute

`research` discovers datasets and operations by meaning, website, source, category, or group. `inspect` retrieves exact parameter types, required fields, response shapes, pricing, and copy-ready `execute` arguments. `execute` validates inputs and estimated charges before independently querying each operation, preserving native JSON, pagination and partial results when another provider rejects access.

```bash
webctx research "podcast conversations about AI agents" --limit 5
webctx research --view sources --limit 5
webctx research --mode catalogue --view operations --sources particle --limit 10
webctx inspect particle/podcasts/episodes/search
webctx execute particle/podcasts/episodes/search --inputs '{"semantic_search":"AI agents","limit":2}'
```

Browse [the structured-data guide](https://webctx.ashray.xyz/docs/data-sources) for filters, paging, batches, full upstream responses, validation, and credit handling. Discovery/inspection is free; `execute` uses paid Alexandria credits. The hosted Origo server enforces a 200-credit limit per request, with no user-facing budget controls. Estimated and actual credits are shown in responses. Third-party provider terms require separate human acceptance; Origo never accepts them automatically.

Origo deploys independently through the `origo-api` Vercel project on pushes to `main`. CLI releases and npm publishing are unchanged. See the [Origo MCP guide](https://webctx.ashray.xyz/docs/origo) for architecture, setup, and security notes.

## Install

```bash
npm i -g webctx
webctx --help
```

Prebuilt binaries are also available from GitHub Releases. Windows builds cover
x86 (32-bit), x64 (AMD64), and ARM64; the npm installer selects the native
binary for your Node.js architecture. macOS and Linux builds cover x64 and ARM64.

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

Normal search asks the enabled Brave, Tavily, and Exa providers, removes duplicate URLs, reranks technical docs/first-party sources, and returns up to 35 links with provider-native snippets or Exa highlights. It **does not automatically read or scrape result pages**. Tavily defaults to quality-oriented `advanced` search depth. Agents can selectively call `webctx read-link` on useful results. One working search provider is enough. Alexandria discovery is separate: use `webctx research` to find structured-data sources and `execute` when you need provider-native data.

```bash
webctx search "next.js server components"
webctx search "react hooks" --exclude youtube.com,medium.com
webctx search "drizzle orm" --keyword "migration guide"
```

### Enable or disable search providers

```bash
webctx status                 # enabled/disabled providers and locally configured keys
webctx providers disable exa  # stop all Exa search requests, even with a key present
webctx providers enable exa   # turn Exa back on later
webctx providers disable brave
webctx providers enable brave
```

Provider settings persist across runs, independently of environment variables and Keychain credentials. The settings file lives in the OS user config directory (`webctx/config.json`); set `WEBCTX_CONFIG_FILE` to override its location. Disabled providers are never queried. `status` only reports key presence, **not** live credit balances or API health.

Multiple keys per provider are supported. Add extra keys on macOS without placing secrets in command arguments or plaintext config files:

```bash
webctx keys list                         # count configured and managed keys (never prints them)
pbpaste | webctx keys add exa --stdin    # securely store a copied new key in macOS Keychain
webctx keys remove exa 1                # remove the newest Keychain-managed Exa key
webctx providers enable exa
```

Or supply `BRAVE_API_KEYS`, `TAVILY_API_KEYS`, or `EXA_API_KEYS` as comma-separated keys (or JSON arrays) in the environment or `.env.local`, alongside the corresponding singular `*_API_KEY`. When more than one key is configured, **requests round-robin across every key, including the legacy singular key**, rather than always preferring the newest one. A small, file-locked cursor shared across CLI processes keeps parallel agents balanced. On authentication, quota, rate-limit, server, or transport failures, WebCTX immediately tries another healthy key; failed keys temporarily cool down across processes, respecting `Retry-After` where available. Invalid requests are not retried. The persistent cursor/cooldown state contains only key hashes, never the credentials. Set `WEBCTX_SEARCH_KEY_BALANCER_DIR` to override the default per-user cache directory. This coordinates the same machine/user, not separate machines or provider-account-wide quotas. Manage only keys/accounts you are authorized to use and honor each provider's usage restrictions. Firecrawl/Alexandria remains separate and is **not** included in search-key rotation, especially for paid operations.

`--keyword` prefers Exa's quality-oriented `auto` search with a quoted phrase when available. Exa's former `includeText` filter was deprecated and was never guaranteed to match an exact phrase. If Exa is disabled, unconfigured, or fails, Brave searches for the quoted phrase and Tavily also applies its `exact_match` filter. The output discloses that Exa and Brave results are not guaranteed to contain the literal phrase in the complete page text. Exa uses relevance-focused highlights and its recommended default content freshness (cached content when available, fetch when needed); `maxAgeHours` is intentionally omitted to avoid forcing slow recrawls for every result. Firecrawl's separate page-reading cache and proxy escalation are unchanged.

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

- `BRAVE_API_KEY`, `TAVILY_API_KEY`, and `EXA_API_KEY` power normal `search`. The plural `*_API_KEYS` names supply additional fallback credentials. Configure any one or more, and optionally disable providers you no longer want to query.
- `FIRECRAWL_API_KEY` powers `map-site`, page-crawl fallbacks, and Alexandria discovery/inspection/paid execution.
- `GH_TOKEN` or `GITHUB_TOKEN` is optional. It increases GitHub capacity and unlocks reads such as blame, Discussions, some Actions job logs, private resources the token can access, and richer PR review state.

Environment variables, `.env.local`, and macOS Keychain are supported. `GH_TOKEN` takes precedence over `GITHUB_TOKEN`.

## Documentation

Start with the guides in `src/content/docs`:

- **Quickstart** — get useful output immediately
- **Read a URL** — GitHub-aware reading and normal web pages
- **Use webctx with agents** — practical research, repo, PR, and CI workflows
- **Search the web** — federated search and filtering
- **Map a site** — discover pages before reading them
- **Research structured data** — browse data sources, inspect exact contracts, and execute paid queries with bounded credits
- **Credentials** — keys and optional GitHub auth
- **How URL reading works** — native/direct fast paths before Firecrawl
- **How search ranking works** — URL normalization, position scoring, and provider agreement

Maintainer notes, repository layout, development commands, and release steps live in [`CONTRIBUTORS.md`](CONTRIBUTORS.md).
