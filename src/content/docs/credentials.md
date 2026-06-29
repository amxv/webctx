---
title: Credentials
description: Configure Brave, Tavily, Exa, and Firecrawl keys through environment variables, .env.local files, or macOS Keychain.
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

`webctx read-link` uses `FIRECRAWL_API_KEY` only when the GitHub raw-content path and direct markdown path do not work.

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
```

Keychain lookup is skipped on non-macOS systems.

## Missing key errors

When a key is missing, webctx explains which one is needed and where to put it. For example, a `map-site` command without Firecrawl credentials reports that `FIRECRAWL_API_KEY` is missing and points to environment variables, `.env.local`, or macOS Keychain.
