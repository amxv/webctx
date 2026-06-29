---
title: Troubleshooting
description: Diagnose missing keys, empty search results, provider failures, Firecrawl errors, npm install failures, markdown fallback behavior, and release issues.
order: 12
category: Reference
summary: Common webctx failure modes and the fastest checks.
---

## Search says credentials are missing

`search` needs Brave, Tavily, and Exa credentials for normal mode:

```text
BRAVE_API_KEY
TAVILY_API_KEY
EXA_API_KEY
```

Check the current shell:

```bash
env | grep -E 'BRAVE_API_KEY|TAVILY_API_KEY|EXA_API_KEY'
```

Then check `.env.local` near the binary and in the current working directory.

## Keyword mode fails

Keyword mode uses Exa only:

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

Confirm `EXA_API_KEY` is available. Also remember that the keyword phrase is truncated to five words before being sent as include-text criteria.

## read-link works for GitHub but fails elsewhere

Public GitHub file URLs use raw content and may not need Firecrawl. Normal pages need Firecrawl when the `.md` fast path is not available.

Set:

```text
FIRECRAWL_API_KEY
```

Then retry:

```bash
webctx read-link https://docs.firecrawl.dev/introduction
```

## Private GitHub blob URLs fail

The GitHub fast path uses unauthenticated raw content fetches. Private repositories are not readable through that path unless the raw URL is publicly accessible. Use a local clone, an authenticated fetch outside webctx, or another workflow for private content.

## map-site fails

`map-site` always uses Firecrawl:

```bash
webctx map-site https://docs.firecrawl.dev
```

Check `FIRECRAWL_API_KEY`, Firecrawl account limits, and whether the target site blocks crawling.

## npm install cannot download a binary

The postinstall script downloads a release asset from GitHub. If that fails, it falls back to `go build` when Go and source files are present.

Check:

```bash
node -v
go version
npm i -g webctx
```

If the package version has no matching GitHub Release asset, install from source or publish the missing release asset.

## Version looks wrong

`webctx --version` reads build info set at compile time. Maintainer builds should pass linker flags through the Makefile or postinstall script.

Use:

```bash
make build VERSION=0.1.2
./dist/webctx --version
```

## Release workflow fails on npm publish

Confirm the GitHub secret exists:

```bash
gh secret list --repo amxv/webctx
```

The required secret is:

```text
NPM_TOKEN
```

Also confirm `package.json` version matches the tag and package publish permissions are still valid.
