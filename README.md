# webctx

`webctx` is a pure Go CLI for agent-friendly web search and page extraction.

It gives you three commands:

- `search`: combines Brave, Tavily, and Exa results, then deduplicates and re-ranks them
- `read-link`: turns a page into clean markdown
- `map-site`: returns a sitemap-style list of URLs for a site


## Documentation Site

This repository includes an Astro documentation site for webctx. It covers installation, provider credentials, search, read-link, map-site, ranking, architecture, npm distribution, release checks, troubleshooting, and docs maintenance.

Run the docs site locally with:

```bash
npm install
npm run docs:dev
```

Validate the docs site with:

```bash
npm run docs:check
npm run docs:build
```

The Astro docs content lives in `src/content/docs`, with site-wide navigation and metadata in `src/data/docs.ts`.

## Install

```bash
npm i -g webctx
webctx --help
```

You can also download a prebuilt binary from GitHub Releases if you do not want the npm install path.

## Commands

```bash
webctx --version
webctx search <query> [--exclude domain1,domain2] [--keyword phrase]
webctx read-link <url>
webctx map-site <url>
```

## Quick examples

```bash
webctx search "next.js server components"
webctx search "react hooks" --exclude youtube.com,vimeo.com
webctx search "drizzle orm" --keyword "migration guide"
webctx read-link https://github.com/openai/openai-cookbook/blob/main/README.md
webctx map-site https://example.com
```

## API keys

`webctx` can read API keys in three ways:

1. regular environment variables
2. a `.env.local` file next to the binary
3. macOS Keychain

If you want the simplest local setup, create a `.env.local` file in the same directory as the binary:

```bash
cp .env.local.example .env.local
```

On macOS, you can also store credentials in Keychain under service `webctx`, with account names matching the env var names:

```bash
security add-generic-password -U -s webctx -a BRAVE_API_KEY -w "your-brave-key"
security add-generic-password -U -s webctx -a TAVILY_API_KEY -w "your-tavily-key"
security add-generic-password -U -s webctx -a EXA_API_KEY -w "your-exa-key"
security add-generic-password -U -s webctx -a FIRECRAWL_API_KEY -w "your-firecrawl-key"
security add-generic-password -U -s webctx -a GH_TOKEN -w "your-github-token"
```

Required keys by command:

- `search`
  Uses `BRAVE_API_KEY`, `TAVILY_API_KEY`, and `EXA_API_KEY`
- `read-link`
  Reads public GitHub repositories, blobs, trees, Issues, Pull Requests, commits, comparisons, path history, Actions runs/jobs, branches, tags, releases, forks, stars, watchers, Gists, Search results, public profiles, labels, and milestones without a token. Optional `GH_TOKEN` (preferred) or `GITHUB_TOKEN` enables authenticated/private GitHub reads, PR thread-state enrichment, structured blame ranges, and Discussions. Uses `FIRECRAWL_API_KEY` for pages that are not handled natively or as direct `.md` content.
- `map-site`
  Uses `FIRECRAWL_API_KEY`

## Why `read-link` is useful

`read-link` is designed to avoid expensive scraping when it does not need to and to keep copied GitHub URLs scoped to what they mean.

- A GitHub repository root returns compact repository metadata, a README preview of roughly 5,000 characters, a full README blob link when needed, and a few useful source/tree/Issue/PR URL examples.
- GitHub blob URLs use the raw-content fast path for public files. `#L20` and `#L20-L40` select source lines, while Markdown heading fragments such as `#installation` select that section. Direct blobs return the full source by default and preserve source comments.
- GitHub tree URLs return a one-level directory listing plus a bounded directory README when present. Slash-containing refs are resolved against GitHub rather than assuming the first path segment is the branch.
- GitHub Issue URLs return compact metadata, the human-visible body, substantive timeline activity, comments, and current relationships. `#issuecomment-<id>` reads one exact comment. Issue/search/label/milestone lists stay bounded instead of expanding every conversation.
- GitHub Pull Request conversation URLs combine the PR body, normal comments, meaningful timeline events, formal reviews, and inline review threads without duplicate review events. `#issuecomment-<id>`, `#discussion_r<id>`, and `#pullrequestreview-<id>` select exact conversation context. Public PRs work anonymously; a token optionally enriches inline threads with GitHub resolved/outdated state.
- Pull Request `/files`, `/commits`, and `/checks` URLs are separate native views. Files preserve patches and current GitHub `#diff-<sha256(path)>L/R...` selectors, commits stay compact, and checks keep Check Runs distinct from commit statuses. `?check_run_id=<id>` narrows to one check plus its annotations. Direct `.diff` and `.patch` URLs preserve their raw media.
- Commit URLs return identity/message/verification/stats, paginated changed-file patches, and commit comments. Commit `.diff`/`.patch` forms stay raw; the JSON file view surfaces GitHub's 3,000-file ceiling rather than implying completeness past it.
- Compare URLs preserve base/head/status/ahead-behind state and all paginated commits. GitHub exposes changed files only on the first compare page and up to 300 files, so webctx states that ceiling when reached. Repository `/commits/<ref>/<path>` URLs stay page-bounded and resolve slash-containing refs against provider history data.
- Blame URLs are native but require `GH_TOKEN` or `GITHUB_TOKEN`: GitHub's structured blame ranges come from GraphQL. Without auth, webctx returns a concise token hint instead of scraping blame UI.
- GitHub Actions repository/workflow pages stay bounded; a selected run returns run state plus all jobs/artifacts, with exact job URLs. Only `/actions/runs/<run>/job/<job>` fetches that job's log, keeping unrelated logs out of context. Plain or ZIP-delivered logs are decoded as text, while expired/unavailable logs and artifacts stay explicit.
- `/branches`, `/tags`, `/releases`, `/forks`, `/stargazers`, and `/watchers` are compact bounded repository-navigation views. Exact release/latest URLs preserve full release notes and paginated asset metadata. Stars and watchers stay distinct: GitHub's historical `watchers_count` aliases stars, while `/watchers` is backed by the subscribers API for actual watchers/subscribers.
- Repository Discussions use GitHub GraphQL and therefore require `GH_TOKEN`/`GITHUB_TOKEN`; detail reads preserve all paginated comments/replies. Public Gists use REST, preserve source comments, support copied file/line anchors, include comments/revisions, and replace or explicitly mark API-truncated files using their raw URLs.
- GitHub `/search` URLs stay page-bounded and preserve the copied query/type/sort/order/page semantics for repository, Issue, Pull Request, code, commit, and user searches. Search quota errors remain distinct from ordinary REST quota errors. One-segment public profiles resolve GitHub's provider `type` before choosing User versus Organization tabs—webctx never guesses from the account name.
- Repository `/activity` and supported graph pages return compact provider activity/statistics instead of scraped charts. GitHub statistics can be cached upstream or return HTTP 202 while computing; webctx reports that state rather than pretending a fresh request means freshly computed metrics. Deployment lists stay bounded, while `/deployments/<environment>` includes that page's deployment/status history and explicitly avoids implying indefinite provider retention.
- Exact GitHub Package pages whose URL supplies owner scope, package type, and package name return compact package metadata plus a bounded versions page when the configured GitHub credential has the package permission/token type required by GitHub. Exact Projects v2 URLs under `/orgs/.../projects/<n>` or `/users/.../projects/<n>` use GitHub's current REST Projects v2 API and return the first 50 items with an explicit more-items marker. Public Project reads can stay anonymous; a fine-grained token that is narrower than the public endpoint is retried anonymously rather than making the public URL fail. Package indexes without enough identity, wiki/settings/security/admin pages, and archive/binary payload routes remain generic/excluded rather than receiving fake native output.
- Optional `GH_TOKEN`, then `GITHUB_TOKEN`, is used for authenticated GitHub API reads. Ordinary public reads do not require or prompt for a token.
- For direct markdown-style URLs, it checks the `.md` path.
- GitHub route families that do not yet have a native reader, and normal web pages, continue through the existing direct-markdown and Firecrawl fallbacks.

Native GitHub failures such as not-found/private, authentication, and rate-limit errors stay GitHub-specific instead of silently turning into scraped login/error pages.

Maintainer notes, release steps, project layout, and source-build details are in `CONTRIBUTORS.md`.
