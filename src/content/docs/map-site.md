---
title: Map-site command
description: Use Firecrawl's map endpoint to return a sitemap-style list of URLs for a website.
order: 6
category: Commands
summary: The behavior of `webctx map-site`.
---

## Basic usage

```bash
webctx map-site https://docs.firecrawl.dev
```

`map-site` returns a count followed by URLs and any available titles or descriptions.

## Firecrawl map request

`map-site` sends a request to:

```text
https://api.firecrawl.dev/v2/map
```

Request settings:

```json
{
  "sitemap": "include",
  "includeSubdomains": true,
  "ignoreQueryParameters": true,
  "limit": 5000
}
```

That makes it useful for discovering docs pages, support pages, blog collections, changelogs, and other site-level context before asking an agent to read specific pages.

## Credentials

`map-site` requires:

```text
FIRECRAWL_API_KEY
```

If the key is missing, webctx prints an error that explains the three supported credential locations: environment variables, `.env.local`, and macOS Keychain.
