---
title: Credentials
description: Add only the provider keys needed for the webctx commands you use.
order: 3
category: Start
summary: Search keys, Firecrawl, and optional GitHub authentication without extra setup ceremony.
---

## Which keys do I need?

| Use | Keys |
| --- | --- |
| `webctx search` | `BRAVE_API_KEY`, `TAVILY_API_KEY`, `EXA_API_KEY` |
| `webctx map-site` | `FIRECRAWL_API_KEY` |
| Normal web-page fallback in `read-link` | `FIRECRAWL_API_KEY` |
| More GitHub capacity and auth-only GitHub reads | `GH_TOKEN` or `GITHUB_TOKEN` |

A GitHub token is optional. Many public repository, source, Issue, PR, commit, release, Search, profile, Gist, activity, deployment, and Project reads work without one when GitHub exposes them publicly.

A token is useful for:

- higher GitHub API limits
- blame ranges
- Discussions
- some Actions job logs
- richer PR review-thread state
- private resources the token can access

GitHub Packages have their own permission rules. A normal fine-grained repository token is not automatically enough for every Package endpoint.

## `.env.local`

For most local setups, use:

```bash
BRAVE_API_KEY=...
TAVILY_API_KEY=...
EXA_API_KEY=...
FIRECRAWL_API_KEY=...
GH_TOKEN=...
```

webctx can load credentials from the environment, `.env.local`, or macOS Keychain. Existing environment variables win.

If both GitHub variables are set, `GH_TOKEN` wins over `GITHUB_TOKEN`.

## macOS Keychain

You can keep a key out of files entirely:

```bash
security add-generic-password -U -s webctx -a GH_TOKEN -w "your-token"
```

Use the environment-variable name as the Keychain account name. The same pattern works for the search and Firecrawl keys.

## Fine-grained GitHub tokens

A fine-grained token can be narrower than GitHub's public read surface. For selected public GitHub GETs, webctx can retry without Authorization if GitHub rejects the token, so adding a narrow token does not unnecessarily break a public read.

Permission errors still remain permission errors when GitHub does not allow the resource anonymously.

For recognized native GitHub resources, adding `FIRECRAWL_API_KEY` does not turn an auth, private/not-found, or rate-limit response into a scraped substitute. Those provider states remain visible so an agent can react correctly. Exact public Package pages are the documented exception: an auth/permission failure may use a clearly labeled best-effort Firecrawl read when that key is configured.
