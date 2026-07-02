---
title: Changelog
description: "Release notes for webctx."
order: 99
category: Reference
summary: Version-by-version changes for the webctx CLI.
---

This changelog tracks code and product changes in webctx. It intentionally skips docs-site-only updates.

## 0.1.1 — 2026-03-22

- Fixed credential loading from binary environment variables and the OS keychain.
- Ignored the local `tmp` workspace.

## 0.1.0 — 2026-03-22

- Ported the webctx CLI to Go.
- Prepared the npm release flow for distributing the Go binary through npm.
- Fixed build-all artifact names.
- Added an environment example for provider credentials.
