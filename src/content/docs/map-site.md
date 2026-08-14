---
title: Map a site
description: Discover the useful URLs on a site before deciding which pages to read.
order: 12
category: Guides
summary: Turn a docs or product site into a navigable URL inventory.
---

## Basic usage

```bash
webctx map-site https://docs.firecrawl.dev
```

The result is a list of discovered URLs with titles or descriptions when available.

## Why map first?

A docs site can contain hundreds or thousands of pages. An agent usually does not need all of them.

Use `map-site` to see the shape of the site, then read only the pages that matter:

```bash
webctx map-site https://some-docs.example
webctx read-link https://some-docs.example/getting-started
webctx read-link https://some-docs.example/api/reference
```

This keeps research broad at the discovery step and focused at the reading step.

## Credential

`map-site` uses:

```text
FIRECRAWL_API_KEY
```
