---
title: Credentials
description: Configure search, Firecrawl, and optional GitHub credentials through environment variables, .env.local files, or macOS Keychain.
order: 3
category: Credentials
summary: The key-loading model for all webctx commands.
---

## Required keys by command

`webctx search` uses:

```text
BRAVE_API_KEY
TAVILY_API_KEY
EXA_API_KEY
```

`webctx read-link` can use these optional GitHub tokens for authenticated/private GitHub reads and GraphQL-only enrichment:

```text
GH_TOKEN
GITHUB_TOKEN
```

Public GitHub repository, blob, tree, Issue, Pull Request, commit, comparison, path-history, Actions, branch/tag/release/social-list, public Gist, label, and milestone reads do not require a token when GitHub exposes the selected public resource anonymously. A GitHub token additionally enables private/native reads, optional PR review-thread resolved/outdated enrichment, structured blame ranges, and repository Discussions through GraphQL. Blame and Discussions specifically require one of these token variables. When both contain values, `GH_TOKEN` takes precedence. `FIRECRAWL_API_KEY` is used only when a URL is not handled by a native/direct-markdown path and needs the Firecrawl fallback.

`webctx map-site` uses:

```text
FIRECRAWL_API_KEY
```

## Loading order

At startup, webctx loads credentials in this order:

1. existing environment variables
2. `.env.local` files near the executable
3. `.env.local` in the current working directory
4. macOS Keychain entries for missing keys

Existing environment variables win. A `.env.local` file never overwrites a key that is already set in the process environment.

## .env.local files

A local credentials file can look like this:

```bash
BRAVE_API_KEY=brave_demo_key
TAVILY_API_KEY=tavily_demo_key
EXA_API_KEY=exa_demo_key
FIRECRAWL_API_KEY=firecrawl_demo_key
GH_TOKEN=
GITHUB_TOKEN=
```

webctx checks these candidate paths:

```text
same directory as the webctx executable
parent directory of the executable directory
current working directory
```

Blank lines and comments are ignored. Lines may start with `export`, and quoted values are accepted.

## macOS Keychain

On macOS, webctx looks up missing credentials under service `webctx`. The account name must match the environment variable name:

```bash
security add-generic-password -U -s webctx -a BRAVE_API_KEY -w brave_demo_key
security add-generic-password -U -s webctx -a TAVILY_API_KEY -w tavily_demo_key
security add-generic-password -U -s webctx -a EXA_API_KEY -w exa_demo_key
security add-generic-password -U -s webctx -a FIRECRAWL_API_KEY -w firecrawl_demo_key
security add-generic-password -U -s webctx -a GH_TOKEN -w github_demo_token
```

Keychain lookup is skipped on non-macOS systems.

## Missing key errors

When a key is missing, webctx explains which one is needed and where to put it. For example, a `map-site` command without Firecrawl credentials reports that `FIRECRAWL_API_KEY` is missing and points to environment variables, `.env.local`, or macOS Keychain.
