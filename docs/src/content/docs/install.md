---
title: Install
description: Install the CLI using npm, a release binary, or Go.
order: 2
category: Start
summary: Install webctx on macOS, Linux, or Windows.
---

## npm (recommended)

```bash
npm install -g webctx
webctx --version
```

The npm installer downloads a prebuilt binary for your operating system and architecture. If a binary cannot be downloaded, it can fall back to building with Go when available.

## Download a binary

Choose your platform from [GitHub Releases](https://github.com/amxv/webctx/releases):

| Operating system | Architectures |
| --- | --- |
| macOS | Intel (x64), Apple Silicon (ARM64) |
| Linux | x64, ARM64 |
| Windows | x86 (32-bit), x64, ARM64 |

Extract the binary, make it executable when needed, and add it to your `PATH`.

## Build from source

Requires Go (see the version in `go.mod`).

```bash
git clone https://github.com/amxv/webctx.git
cd webctx
make build
./dist/webctx --version
```

To install that build into `~/.local/bin`:

```bash
make install-local
```

## Next

Follow the [Quickstart](/docs/quickstart) and [set up credentials](/docs/credentials) for the commands you plan to use. For development, tests, and releases, see [CONTRIBUTORS.md](https://github.com/amxv/webctx/blob/main/CONTRIBUTORS.md).
