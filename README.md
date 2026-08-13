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
  Reads public GitHub repositories, blobs, trees, Issues, labels, and milestones without a token. Optional `GH_TOKEN` (preferred) or `GITHUB_TOKEN` enables authenticated/private GitHub reads. Uses `FIRECRAWL_API_KEY` for pages that are not handled natively or as direct `.md` content.
- `map-site`
  Uses `FIRECRAWL_API_KEY`

## Why `read-link` is useful

`read-link` is designed to avoid expensive scraping when it does not need to and to keep copied GitHub URLs scoped to what they mean.

- A GitHub repository root returns compact repository metadata, a README preview of roughly 5,000 characters, a full README blob link when needed, and a few useful source/tree/Issue URL examples.
- GitHub blob URLs use the raw-content fast path for public files. `#L20` and `#L20-L40` select source lines, while Markdown heading fragments such as `#installation` select that section. Direct blobs return the full source by default and preserve source comments.
- GitHub tree URLs return a one-level directory listing plus a bounded directory README when present. Slash-containing refs are resolved against GitHub rather than assuming the first path segment is the branch.
- GitHub Issue URLs return compact metadata, the human-visible body, substantive timeline activity, comments, and current relationships. `#issuecomment-<id>` reads one exact comment. Issue/search/label/milestone lists stay bounded instead of expanding every conversation.
- Optional `GH_TOKEN`, then `GITHUB_TOKEN`, is used for authenticated GitHub API reads. Ordinary public reads do not require or prompt for a token.
- For direct markdown-style URLs, it checks the `.md` path.
- GitHub route families that do not yet have a native reader, and normal web pages, continue through the existing direct-markdown and Firecrawl fallbacks.

Native GitHub failures such as not-found/private, authentication, and rate-limit errors stay GitHub-specific instead of silently turning into scraped login/error pages.

Maintainer notes, release steps, project layout, and source-build details are in `CONTRIBUTORS.md`.
