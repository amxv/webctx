---
title: Quickstart
description: Install webctx, configure provider keys, and run the first search, read-link, and map-site commands.
order: 1
category: Start
summary: The shortest path from install to useful web context.
---

## What webctx does

`webctx` is a pure Go command-line tool for getting web context into agent workflows without opening a browser manually.

It has three user-facing commands:

```bash
webctx search "go http client retry patterns"
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
webctx map-site https://docs.firecrawl.dev
```

The output is plain text or markdown, so it can be pasted directly into ChatGPT, Codex, Claude Code, terminal agents, scripts, notes, or issue comments.

## Install from npm

The npm package installs a platform-specific native binary when a matching GitHub Release asset exists:

```bash
npm i -g webctx
webctx --help
webctx --version
```

If a release asset is not available for the current platform, the postinstall script falls back to a local Go build when Go is installed and the package includes source files.

## Build from source

```bash
git clone https://github.com/amxv/webctx.git
cd webctx
make build
./dist/webctx --help
```

For local installation from the repository:

```bash
make install-local
webctx --help
```

## Add credentials

Search uses Brave, Tavily, and Exa. Link reading and site mapping use Firecrawl when fast markdown paths are not enough.

Create a `.env.local` file where webctx can find it:

```bash
BRAVE_API_KEY=brave_demo_key
TAVILY_API_KEY=tavily_demo_key
EXA_API_KEY=exa_demo_key
FIRECRAWL_API_KEY=firecrawl_demo_key
```

webctx checks environment variables first, then `.env.local`, then macOS Keychain.

## First commands

Run a normal federated search:

```bash
webctx search "next.js server components"
```

Exclude noisy domains:

```bash
webctx search "react hooks" --exclude youtube.com,vimeo.com
```

Force an Exa include-text search for a short keyword phrase:

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

Read a page into markdown:

```bash
webctx read-link https://github.com/amxv/webctx/blob/main/CONTRIBUTORS.md
```

Map a site:

```bash
webctx map-site https://docs.firecrawl.dev
```
