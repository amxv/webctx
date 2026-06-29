---
title: Install and distribution
description: Understand the npm install path, GitHub release assets, source builds, local installs, and supported platforms.
order: 2
category: Start
summary: How webctx is packaged as a native Go binary behind an npm CLI shim.
---

## Distribution model

webctx ships in two forms:

- GitHub Releases for native binaries
- npm for `npm i -g webctx`

The npm package contains a JavaScript shim at `bin/webctx.js`. During installation, `scripts/postinstall.js` attempts to download the native release asset for the current platform and architecture.

## Release asset names

The postinstall script expects assets named like this:

```text
webctx_darwin_amd64
webctx_darwin_arm64
webctx_linux_amd64
webctx_linux_arm64
webctx_windows_amd64.exe
```

The release tag is derived from `package.json` version. Version `0.1.1` maps to release tag `v0.1.1`.

## npm install path

```bash
npm i -g webctx
```

On install, webctx detects platform and architecture, downloads the matching GitHub Release asset, writes it into the package `bin` directory, and marks it executable on Unix-like systems.

## Fallback Go build

If the release asset download fails, the postinstall script tries a local build:

```bash
go build -trimpath -ldflags="-s -w -X github.com/amxv/webctx/internal/buildinfo.Version=0.1.1" -o bin/webctx-bin ./cmd/webctx
```

The fallback requires Go to be installed and the source files to be present in the package.

## Source build path

```bash
make build
./dist/webctx --help
```

For cross-platform release artifacts:

```bash
make build-all
```

`make build-all` creates binaries for macOS Intel, macOS Apple Silicon, Linux Intel, Linux ARM, and Windows Intel.

## Local install path

```bash
make install-local
```

This builds `dist/webctx`, then installs it to `~/.local/bin/webctx`.
