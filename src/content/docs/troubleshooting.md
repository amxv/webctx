---
title: Troubleshooting
description: Quick fixes for missing keys, GitHub permissions, rate limits, Firecrawl failures, and installation problems.
order: 21
category: Reference
summary: Start with the error webctx gave you, then check the matching provider or credential.
---

## Search says credentials are missing

Normal search needs:

```text
BRAVE_API_KEY
TAVILY_API_KEY
EXA_API_KEY
```

Add them to the environment or `.env.local`, then retry.

`--keyword` only needs `EXA_API_KEY`.

## `read-link` works for GitHub but not normal websites

Normal web pages often need:

```text
FIRECRAWL_API_KEY
```

Try:

```bash
webctx read-link https://example.com
```

If Firecrawl is configured and the site still fails, the site may be blocking extraction or Firecrawl may be rate-limited.

## A GitHub read asks for authentication

Set one of:

```text
GH_TOKEN
GITHUB_TOKEN
```

`GH_TOKEN` wins when both are set.

Some common authenticated reads are blame, Discussions, certain Actions job logs, richer PR review-thread state, and private resources your token can access.

A browser-visible GitHub page is not always anonymously available through GitHub's structured API. webctx reports the provider's actual auth state rather than pretending the data is available.

## A fine-grained GitHub token gets `403`

Fine-grained tokens only have the permissions you granted them. A token can work for one repository endpoint and still be rejected by Packages, social lists, Projects owned by another organization, or other GitHub surfaces.

For selected public GETs, webctx can retry anonymously when a narrow token is rejected. If GitHub also rejects the anonymous request, webctx keeps the permission error.

## A GitHub Package returns best-effort page content

This is intentional for exact public Package pages.

If GitHub's Package API rejects the read for auth or permission reasons and `FIRECRAWL_API_KEY` is configured, webctx can crawl the public Package page instead. The output starts with a **best-effort** warning because scraped GitHub pages can contain UI noise or incomplete dynamically loaded data.

Rate limits and private/not-found resources do not use this fallback.

## GitHub says you are rate-limited

webctx includes the rate-limit resource and reset/retry information GitHub returned.

Anonymous GitHub capacity is much smaller than authenticated capacity. Add `GH_TOKEN` or `GITHUB_TOKEN` if you want authenticated limits.

webctx does not hide GitHub rate limits by scraping the page instead; a rate-limited structured read should stay visibly rate-limited.

## An Actions job has no log

The job can still be valid even when its log is no longer downloadable. GitHub controls log availability and retention.

webctx keeps the job/step metadata and tells you the log is unavailable instead of substituting another job.

## `map-site` fails

`map-site` always needs:

```text
FIRECRAWL_API_KEY
```

Check the key, Firecrawl account limits, and whether the target site is crawlable.

## npm install fails

Try the latest npm path first:

```bash
npm i -g webctx
webctx --version
```

If you cannot use the prebuilt package on your platform, build from source:

```bash
git clone https://github.com/amxv/webctx.git
cd webctx
make build
./dist/webctx --version
```

For contributor-specific build/release troubleshooting, see `CONTRIBUTORS.md`.
