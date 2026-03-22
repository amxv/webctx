# CONTRIBUTORS.md

Maintainer notes for `webctx`.

## Prerequisites

- Go `1.26+`
- Node `18+`
- npm account with publish rights for the package name in `package.json`
- GitHub repo admin access

## Build from source

```bash
git clone https://github.com/amxv/webctx.git
cd webctx
make build
./dist/webctx --help
```

## Local development

```bash
make check
make build
./dist/webctx --help
```

Example command checks:

```bash
./dist/webctx --version
./dist/webctx search "golang http client"
./dist/webctx read-link https://github.com/amxv/webctx-ts/blob/main/cli.ts
./dist/webctx map-site https://example.com
```

Install command locally:

```bash
make install-local
webctx --help
```

## Release and distribution

This repo ships in two ways:

- GitHub Releases for native binaries
- npm for `npm i -g webctx`

The release workflow triggers on `v*` tags and does the following:

1. runs Go and Node quality checks
2. builds cross-platform binaries
3. creates a GitHub Release with those assets
4. publishes the npm package using the tag version

## Release process

1. Ensure `main` is green:

```bash
make check
```

2. Confirm the release workflow is targeting `webctx` and that `package.json` still points to the correct GitHub repository.

3. Prepare release tag:

```bash
make release-tag VERSION=x.y.z
```

4. GitHub Actions `release` workflow runs automatically:
- quality checks
- cross-platform binary build
- GitHub release publish
- npm publish

## Required GitHub secret

- `NPM_TOKEN`: npm automation token with publish rights for your package.

Set via GitHub CLI:

```bash
gh secret set NPM_TOKEN --repo amxv/webctx
```

## npm token setup

Create token at npm:

- Profile -> Access Tokens -> Create New Token
- Use an automation/granular token scoped to required package/org

Validate auth locally:

```bash
npm whoami
```

## Project layout

- `cmd/webctx/main.go`: CLI entrypoint
- `internal/app/`: CLI parsing, search, ranking, scrape, env loading, and Firecrawl queue logic
- `internal/buildinfo/`: build-time version plumbing for `--version`
- `bin/webctx.js`: npm shim that invokes the packaged native binary
- `scripts/postinstall.js`: downloads the release binary on install and falls back to local `go build`
- `.github/workflows/release.yml`: tag-driven release pipeline
- `AGENTS.md`: guidance for coding agents

## Notes on package naming

`webctx` is already configured. If you ever rename or move the package, update all of the following together:

- `package.json`
- `bin/webctx.js`
- `scripts/postinstall.js`
- `.github/workflows/release.yml`
- `Makefile`

## Porting reference

The repo includes `docs/porting-status.md` as the running reference for what was ported from `webctx-ts`, what was intentionally excluded, and what future agents should verify before making behavior changes.
