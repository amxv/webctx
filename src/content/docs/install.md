---
title: Install
description: Install webctx from npm, a release binary, or source.
order: 2
category: Start
summary: The shortest install paths for a terminal, CI job, or agent machine.
---

## npm

```bash
npm i -g webctx
webctx --version
```

This is the easiest option for most users.

## Prebuilt binary

GitHub Releases include binaries for:

- macOS Intel
- macOS Apple Silicon
- Linux Intel
- Linux ARM
- Windows Intel

Download the matching release if you prefer not to use npm.

## Build from source

If you already have Go:

```bash
git clone https://github.com/amxv/webctx.git
cd webctx
make build
./dist/webctx --help
```

For a local install from the checkout:

```bash
make install-local
```

## Next

Go to the [Quickstart](/docs/quickstart) and try `read-link` on a GitHub URL you use every day.

If you are changing webctx itself, development and release notes live in `CONTRIBUTORS.md` in the repository.
