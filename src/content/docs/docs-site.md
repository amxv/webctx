---
title: Docs site maintenance
description: Run, edit, validate, and deploy the Astro documentation site embedded in the webctx repository.
order: 13
category: Reference
summary: How to maintain the Astro docs without disrupting the npm CLI package.
---

## Local development

The Astro docs site shares the root `package.json` with the npm CLI package. Use docs-prefixed scripts:

```bash
npm run docs:dev
```

or with Bun:

```bash
bun run docs:dev
```

Astro usually serves the site at:

```text
http://localhost:4321
```

## Files to edit

```text
src/data/docs.ts                site metadata, repo link, categories, nav
src/pages/index.astro           overview page
src/pages/docs/index.astro      grouped docs index
src/pages/docs/[...slug].astro  article route
src/pages/docs.md.ts            raw markdown index
src/pages/docs/[...slug].md.ts  raw markdown route
src/content/docs/*.md           docs pages
src/styles/global.css           visual system
```

Most content changes belong in `src/content/docs`.

## Validate docs

```bash
npm run docs:check
npm run docs:build
```

or:

```bash
bun run docs:check
bun run docs:build
```

The docs build outputs static files to `dist`. Do not commit generated output.

## Validate the CLI after docs changes

Docs-only changes should not affect the Go CLI or npm shim, but run the existing checks before pushing:

```bash
go test ./...
npm test
```

For broader maintainer confidence:

```bash
make check
```

## npm package safety

The package `files` list controls what is included in the published npm package. The Astro source is not part of the CLI runtime path, so docs files can live in the repository without changing what the installed `webctx` command executes.
