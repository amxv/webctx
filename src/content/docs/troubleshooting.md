---
title: Troubleshooting
description: Fix missing credentials, rate limits, GitHub permissions, and installation issues.
order: 29
category: Reference
summary: Start with the error and follow the shortest useful fix.
---

## `search` says a key is missing

Normal search needs a key for **at least one enabled search provider**:

```text
BRAVE_API_KEY
TAVILY_API_KEY
EXA_API_KEY
```

Add one through an environment variable, `.env.local`, or macOS Keychain. Run `webctx status` to check which providers are enabled and whether keys are configured. Disable providers you do not use with `webctx providers disable exa`.

If you have multiple keys from separate accounts, configure `*_API_KEYS` or use `webctx keys add`. WebCTX coordinates rate limits between agents using the same key on the same machine. See [Credentials](/docs/credentials) and [Rate limits](/docs/rate-limits).

## Search gets throttled

A `429` means the upstream account rejected a request. WebCTX cools down that key and uses another eligible account if configured. If all accounts are busy, a provider can be skipped after a short wait so results from other providers remain available.

Monthly credits are different from requests-per-second or requests-per-minute quotas. Check your provider dashboard if credits may be exhausted. `webctx status` does not query balances.

## `read-link` handles GitHub but not a normal webpage

Normal website extraction may require Firecrawl:

```dotenv
FIRECRAWL_API_KEY=your-key
```

Check the key, account quota, and whether the site blocks extraction. Some documentation pages work through direct Markdown without Firecrawl; not every website does.

## `map-site` fails

Mapping needs a Firecrawl key (`FIRECRAWL_API_KEY` or `FIRECRAWL_API_KEYS`). Check the site URL, Firecrawl account limits, and whether the target can be mapped. Site discovery may be incomplete if the provider reaches its URL ceiling.

## A GitHub URL asks for authentication

Add `GH_TOKEN` or `GITHUB_TOKEN`. Public GitHub reads often work anonymously, but blame, Discussions, some Actions logs, private resources, and extra review state need authentication or higher permissions.

A fine-grained token may lack access to a particular endpoint, even when other GitHub reads work. GitHub's permission error is not something a webpage scrape can reliably bypass.

## GitHub says a page is incomplete

Some GitHub roots intentionally return a compact overview. Distinguish:

- **More data upstream:** GitHub has another page, a provider limit, or a computing/expired artifact.
- **Locally omitted items:** WebCTX did not expand every child in the overview; use the printed URL to open the exact item.

For pageable lists, follow the Next/Previous URL. For source lines, comments, threads, diff hunks, or job logs, follow the more specific GitHub URL from the output. See [GitHub URL reference](/docs/github-urls).

## An Actions job has no log

GitHub controls log availability and retention. WebCTX still returns job and step metadata when available. A large downloadable log may appear as a focused preview with a provider URL for more detail.

## A public GitHub Package was scraped

A public Package webpage can be visible even when GitHub's Package API rejects the available token. For **exact public Package URLs**, webctx may return a clearly marked best-effort Firecrawl read after an authentication/permission error. It does not use that exception for private/not-found resources or GitHub rate limits.

## npm installation fails

Try the current release first:

```bash
npm install -g webctx
webctx --version
```

If npm cannot retrieve a binary for your platform, [download one](https://github.com/amxv/webctx/releases) or [build from source](/docs/install). Building requires Go.

For further diagnostics, consult [CLI reference](/docs/cli-reference), [Credentials](/docs/credentials), or the contributor instructions in [CONTRIBUTORS.md](https://github.com/amxv/webctx/blob/main/CONTRIBUTORS.md).
