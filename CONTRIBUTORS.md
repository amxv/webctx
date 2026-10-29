# CONTRIBUTORS.md

Maintainer notes for `webctx`.

Origo is the separately deployed MCP transport in this monorepo. The Go CLI and MCP server share the same Go module; npm distribution and the Astro documentation site have independent pipelines.

## Prerequisites

- Go `1.26+`
- Node `18+` for the npm CLI, Node `22.12+` and Bun `1.4+` for docs development
- npm account with publish rights for the package name in `npm/package.json`
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

Public documentation lives in `docs/src/content/docs/`: Start and Guides explain
common tasks, while Reference contains flags, configuration, quotas, and
implementation details. To check a documentation change:

```bash
cd docs
bun install --frozen-lockfile --ignore-scripts
bun run check
bun run build
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

Windows release binaries cover 32-bit x86 (`windows/386`), x64 (`windows/amd64`),
and native ARM64 (`windows/arm64`). The release pipeline smoke-tests all three
executables on GitHub-hosted Windows x64/ARM64 runners before publishing.

The release workflow triggers on `v*` tags and does the following:

1. runs Go and Node quality checks
2. builds cross-platform binaries
3. creates a GitHub Release with those assets
4. publishes the npm package using the tag version

## Release process

1. Update `npm/package.json` to the new version and add a corresponding section to
   `docs/src/content/docs/changelog.md`.

2. Ensure the code and documentation checks pass:

```bash
make check
make docs-check
make docs-build
```

3. Confirm the release workflow targets `webctx`, and verify the package name,
   license, repository URL, and GitHub Actions secrets. Commit and push the
   reviewed source, tests, and docs to `main` before creating the tag.

4. Create the tag on that commit and push it:

```bash
make release-tag VERSION=x.y.z
```

5. GitHub Actions `release` workflow runs automatically:
- quality checks
- cross-platform binary build
- GitHub release publish
- npm publish

## Automatic npm access-token publishing

This project publishes npm releases using the repository's GitHub Actions secret `NPM_TOKEN`. Set it to a valid npm access token with publish access to the `webctx` package and appropriate npm two-factor-authentication settings for automated publishing. GitHub Actions supplies it as `NODE_AUTH_TOKEN` during the `release.yml` workflow's npm step.

If a token expires, rotate it in npm and update `NPM_TOKEN` using `gh secret set NPM_TOKEN --repo amxv/webctx`. Never commit tokens or print them in job logs. Trusted publishing is not used.

If a release's GitHub binaries were successfully published while npm authentication failed, dispatch `npm-publish-retry.yml` with the existing version. Do not create or move the release tag just to retry npm publishing:

```bash
gh workflow run npm-publish-retry.yml --repo amxv/webctx --ref main -f version=<existing-version>
```

## Project layout

```text
webctx/
├── cmd/webctx/       Go CLI entrypoint
├── internal/         Shared retrieval, search, and command implementation
├── pkg/              Go adapters and Origo MCP server
├── api/mcp.go        Vercel Go Function entrypoint
├── npm/              npm CLI package, shim, and packaging scripts
├── docs/             Standalone Astro docs site, its dependencies and lockfile
├── notes/            Historical porting notes
├── tests/            Root deployment boundary checks
├── go.mod            One shared Go module for CLI and MCP
└── Makefile          Go commands and docs convenience targets
```

The npm package is **staged** with `node npm/scripts/prepare-package.mjs` before
publishing. This creates the ignored `npm/.publish/` directory containing the
shim, Go module sources, README, and license. The published package can still
build the native CLI from source if downloading its release binary fails.
The docs site is never bundled into npm.

## Origo deployment

The `origo-api` Vercel project uses `ORIGO_DEPLOYMENT=1` to select its Go Function configuration. It needs `ORIGO_API_KEY` and `FIRECRAWL_API_KEY` production environment variables; optionally add `GH_TOKEN` for native authenticated GitHub reads. The docs project remains `webctx-docs` and never receives the Origo connection key.

The independent `Docs` and `Origo API` GitHub Actions workflows filter their own build inputs before deploying to Vercel. Origo uses the `VERCEL_ORIGO_TOKEN` Actions secret and `VERCEL_ORIGO_PROJECT_ID` variable, separate from the docs credentials. Git-triggered Vercel deployments are disabled; `v*` tags still drive CLI releases.

## Notes on package naming

`webctx` is already configured. If you ever rename or move the package, update all of the following together:

- `npm/package.json`
- `npm/bin/webctx.js`
- `npm/scripts/postinstall.js`
- `npm/scripts/prepare-package.mjs`
- `.github/workflows/release.yml`
- `Makefile`

## Porting reference

The repo includes `notes/porting-status.md` as the running reference for what was ported from `webctx-ts`, what was intentionally excluded, and what future agents should verify before making behavior changes.
