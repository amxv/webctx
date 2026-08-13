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

## read-link works for native GitHub URLs but fails elsewhere

Public GitHub repository, blob, tree, Issue, Pull Request, commit, comparison, path-history, Actions, label, and milestone URLs use native/direct provider reads and do not need Firecrawl. Normal pages and unsupported GitHub route families need Firecrawl when the direct `.md` path is not available.

Set:

```text
FIRECRAWL_API_KEY
```

Then retry:

```bash
webctx read-link https://docs.firecrawl.dev/introduction
```

## Private GitHub source reads fail

Configure a GitHub token that can read the repository contents:

```text
GH_TOKEN
```

or, as a fallback:

```text
GITHUB_TOKEN
```

When both contain values, `GH_TOKEN` wins. webctx first keeps the cheap public raw path; if that returns not found and a token is configured, it uses GitHub's authenticated Contents API to resolve and read the source. GitHub can intentionally use a 404 for content the caller cannot see, so a not-found result can mean either an absent resource or insufficient/private access.

If an authenticated read still fails, check that the token is valid and has access to the repository. webctx never requires the `gh` executable for native GitHub reads.

## GitHub blame asks for a token

Blame is intentionally different from ordinary public source/history reads. webctx uses GitHub GraphQL's structured blame ranges, and GitHub GraphQL requires authentication. Configure `GH_TOKEN` or `GITHUB_TOKEN` and retry the same `/blame/<ref>/<path>` URL. Without a token, webctx fails before the provider read and does not fall back to scraped blame UI.

## GitHub Discussions ask for a token

Discussions are another GraphQL-authenticated GitHub surface. Configure `GH_TOKEN` or `GITHUB_TOKEN` for `/discussions` and `/discussions/<number>`. A public repository page may be browser-readable while the structured GraphQL call still requires authentication; webctx reports that requirement directly instead of substituting scraped HTML.

Projects v2 use GitHub's current REST Projects v2 endpoints. Public Projects can be read without a token when GitHub permits anonymous access. A protected Project, or a Project reached with a token that lacks the relevant Project permission, retains GitHub's provider error rather than falling back to scraped board UI. For public GET endpoints, webctx can retry without Authorization when a narrowly scoped fine-grained token is rejected.

GitHub Packages have a separate permission model. A 401 with no configured token is shown as “authentication is required”; a 403 with a configured token means GitHub rejected that credential for the package endpoint. A fine-grained repository token is not automatically a substitute for the package token/scopes GitHub accepts.

## GitHub Search is rate-limited while other GitHub reads still work

GitHub Search uses a distinct rate-limit resource from ordinary core REST reads. webctx reports the resource/reset values from the actual Search response. Exhausting Search therefore does not imply every repository/source/profile read is out of quota, and a core quota failure is not mislabeled as a Search failure.

## GitHub rate-limit errors

Native GitHub rate-limit errors include retry/reset context when GitHub provides it. Anonymous reads have lower provider capacity; configuring `GH_TOKEN` or `GITHUB_TOKEN` can provide authenticated capacity, but webctx does not hide provider rate limits by falling back to a scraped page.

## An Actions job has no log

The job page can still be valid when its log is unavailable. GitHub may return a gone/not-found log after retention expiry, deletion, or before a log is generated. webctx keeps the selected job/step metadata and states that the log is unavailable. It does not substitute unrelated jobs or treat an empty/expired download as successful log content.

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
