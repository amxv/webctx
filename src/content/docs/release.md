---
title: Development and release
description: Run quality checks, build local and cross-platform binaries, install locally, tag releases, and publish npm packages.
order: 9
category: Internals
summary: Maintainer workflow for changing and releasing webctx.
---

## Prerequisites

Maintainers need:

- Go `1.26+`
- Node `18+`
- npm publish rights for the `webctx` package
- GitHub repository admin access for releases and secrets

## Local checks

Run the full check target:

```bash
make check
```

It runs:

```text
gofmt
go test ./...
go vet ./...
npm run lint
```

The npm lint path currently checks the JavaScript shim and postinstall script syntax:

```bash
node --check bin/webctx.js
node --check scripts/postinstall.js
```

## Build locally

```bash
make build
./dist/webctx --help
```

The build uses trimpath and linker flags for version metadata.

## Build all release binaries

```bash
make build-all
```

Targets:

```text
darwin/amd64
darwin/arm64
linux/amd64
linux/arm64
windows/amd64
```

## Install locally

```bash
make install-local
```

This installs the built binary to `~/.local/bin/webctx`.

## Release process

1. Run checks:

```bash
make check
```

2. Confirm `package.json` points to `amxv/webctx` and the release workflow targets `webctx`.

3. Create and push a tag:

```bash
make release-tag VERSION=0.1.2
```

4. The GitHub Actions release workflow should build binaries, create the GitHub Release, and publish npm.

## Required secret

The release workflow needs:

```text
NPM_TOKEN
```

Set it with GitHub CLI:

```bash
gh secret set NPM_TOKEN --repo amxv/webctx
```
