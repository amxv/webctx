# webctx

Minimal template for shipping a Go CLI with:

- a local command runner (`Makefile`)
- npm global install wrapper (`bin/webctx.js`)
- automatic GitHub Release + npm publish on tag

## Install (template example)

```bash
npm i -g @amxv/webctx
webctx --help
```

## Commands in this starter

```bash
webctx --help
webctx hello
webctx hello <name>
webctx --version
```

## Customize this template

1. Rename your command and entrypoint:
- `cmd/webctx`
- `bin/webctx.js`
- `package.json` (`bin`, `config.cliBinaryName`)
- `.github/workflows/release.yml` (`CLI_BINARY`)

2. Update module + repo identity:
- `go.mod` module path
- `package.json` (`name`, `repository`, `homepage`, `bugs`)

3. Replace starter logic:
- `internal/app/app.go`
- `internal/app/app_test.go`

4. Keep release flow:
- push tags like `v0.2.0`
- workflow builds binaries + creates GitHub release + publishes npm

## Project layout

- `cmd/webctx/main.go`: CLI entrypoint
- `internal/app/`: command logic
- `internal/buildinfo/`: build-time version plumbing for `--version`
- `scripts/postinstall.js`: installs binary from GitHub release (falls back to local `go build`)
- `.github/workflows/release.yml`: automated release pipeline
- `AGENTS.md`: instructions for coding agents
- `CONTRIBUTORS.md`: maintainer/release operations

See `AGENTS.md` and `CONTRIBUTORS.md` for complete dev/release instructions.
