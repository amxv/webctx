---
title: CLI reference
description: A compact reference for webctx commands, flags, environment variables, output formats, and maintainer targets.
order: 11
category: Reference
summary: The command map for users and maintainers.
---

## User commands

```bash
webctx --help
webctx --version
webctx search "next.js server components"
webctx search "react hooks" --exclude youtube.com,vimeo.com
webctx search "drizzle orm" --keyword "migration guide"
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
webctx map-site https://docs.firecrawl.dev
```

## Search flags

```text
--exclude domain1,domain2
```

Adds domains to the default exclusion list.

```text
--keyword phrase
```

Switches search into Exa-only include-text mode. The phrase is truncated to five words.

## Environment variables

```text
BRAVE_API_KEY       used by search
TAVILY_API_KEY      used by search
EXA_API_KEY         used by search and keyword mode
FIRECRAWL_API_KEY   used by read-link fallback and map-site
```

## Output formats

`search` returns markdown links with indented summaries.

`read-link` returns a markdown document with title, URL, and content.

`map-site` returns a URL list with optional titles and descriptions.

## Maintainer commands

```bash
make help
make fmt
make test
make vet
make lint
make check
make build
make build-all
make install-local
make clean
make release-tag VERSION=0.1.2
```

## npm scripts

```bash
npm test
npm run lint
npm run docs:dev
npm run docs:check
npm run docs:build
npm run docs:preview
```

The docs scripts are for the Astro site. The existing `test` and `lint` scripts continue to validate the npm shim and postinstall script.
