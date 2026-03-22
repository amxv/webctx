# CONTRIBUTORS.md

Maintainer notes for `webctx`.

## Prerequisites

- Go `1.26+`
- Node `18+`
- npm account with publish rights for the package name in `package.json`
- GitHub repo admin access

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

## Release process

1. Ensure `main` is green:

```bash
make check
```

2. Confirm the release workflow is targeting `webctx` and that `package.json` still points to the correct GitHub repository.

3. Prepare release tag:

```bash
make release-tag VERSION=0.1.0
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

## Notes on package naming

`webctx` is already configured. If you ever rename or move the package, update all of the following together:

- `package.json`
- `bin/webctx.js`
- `scripts/postinstall.js`
- `.github/workflows/release.yml`
- `Makefile`

## Porting reference

The repo includes `docs/porting-status.md` as the running reference for what was ported from `webctx-ts`, what was intentionally excluded, and what future agents should verify before making behavior changes.
