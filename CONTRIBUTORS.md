# CONTRIBUTORS.md

Maintainer notes for `webctx`.

Origo is the separately deployed MCP transport in this same repository. The CLI, MCP, and ZueDocs share source code but have independent distribution pipelines.

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

## Automatic npm access-token publishing

This project publishes npm releases using the repository's GitHub Actions secret `NPM_TOKEN`. Set it to a valid npm access token with publish access to the `webctx` package and appropriate npm two-factor-authentication settings for automated publishing. GitHub Actions supplies it as `NODE_AUTH_TOKEN` during the `release.yml` workflow's npm step.

If a token expires, rotate it in npm and update `NPM_TOKEN` using `gh secret set NPM_TOKEN --repo amxv/webctx`. Never commit tokens or print them in job logs. Trusted publishing is not used.

If a release's GitHub binaries were successfully published while npm authentication failed, dispatch `npm-publish-retry.yml` with the existing version. Do not create or move the release tag just to retry npm publishing:

```bash
gh workflow run npm-publish-retry.yml --repo amxv/webctx --ref main -f version=0.2.2
```

## Project layout

- `cmd/webctx/main.go`: CLI entrypoint
- `internal/app/`: CLI parsing, search, ranking, scrape, env loading, and Firecrawl queue logic
- `pkg/retrieval/`: small public adapter to the shared read-link / site-map engine
- `pkg/origo/`: authenticated, stateless MCP server with exactly two tools
- `api/mcp.go`: Vercel Go Function entrypoint for Origo
- `vercel.mjs`: project-specific Vercel settings for docs and Origo
- `internal/buildinfo/`: build-time version plumbing for `--version`
- `bin/webctx.js`: npm shim that invokes the packaged native binary
- `scripts/postinstall.js`: downloads the release binary on install and falls back to local `go build`
- `.github/workflows/release.yml`: tag-driven release pipeline
- `AGENTS.md`: guidance for coding agents

## Origo deployment

The `origo-api` Vercel project is linked to this GitHub repo and uses `ORIGO_DEPLOYMENT=1` to select its Go Function configuration. It needs `ORIGO_API_KEY` and `FIRECRAWL_API_KEY` production environment variables; optionally add `GH_TOKEN` for native authenticated GitHub reads. The docs project remains `webctx-docs` and never receives the Origo connection key.

Both projects deploy automatically from `main`. `scripts/should-build.mjs` gates docs and API builds independently, while `v*` GitHub tags still drive the CLI release workflow.

## Notes on package naming

`webctx` is already configured. If you ever rename or move the package, update all of the following together:

- `package.json`
- `bin/webctx.js`
- `scripts/postinstall.js`
- `.github/workflows/release.yml`
- `Makefile`

## Porting reference

The repo includes `docs/porting-status.md` as the running reference for what was ported from `webctx-ts`, what was intentionally excluded, and what future agents should verify before making behavior changes.
