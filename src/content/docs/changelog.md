---
title: Changelog
description: "Release notes for webctx."
order: 99
category: Reference
summary: Version-by-version changes for the webctx CLI.
---

This changelog tracks code and product changes in webctx. It intentionally skips docs-site-only updates.

## 0.2.2 - 2026-10-09

- Added the Origo MCP transport to the shared Go retrieval engine, exposing `read_link` and `map_site` without changing the CLI's `search`, `read-link`, or `map-site` commands.
- Improved Firecrawl read-link recovery with staged proxy escalation and a disposable browser fallback for inaccessible or blocked pages.
- Enabled Firecrawl's built-in 30-minute scrape cache (`maxAge`) without adding an application-managed cache or database.
- Hardened URL validation against local/private addresses, rejected empty or challenge-page extractions, and stopped bypassing TLS certificate verification during Firecrawl scrapes.
- Fixed the browser fallback to use supported DOM text extraction and added MCP, proxy, browser, and URL-validation regression tests.

## 0.2.1 - 2026-08-15

- Completed native GitHub context coverage across issues, pull requests, commits, Actions, releases, Discussions, Gists, packages, projects, search, profiles, activity, and deployments.
- Bounded large GitHub roots and provider fan-out so broad reads remain useful without overwhelming the default context.
- Closed the GitHub read-link review gaps and documented the cross-family acceptance coverage.

## 0.2.0 — 2026-08-14

- Added native GitHub reads for repositories, issues, pull requests, commits, Actions, releases, Discussions, Gists, packages, projects, search, profiles, activity, and deployments.
- Added focused GitHub subresource views, bounded pagination, release assets, job logs and artifacts, history, compare, and blame support.
- Made large GitHub roots navigation-first across Issues, PRs, commits, releases, trees, Discussions, Gists, statistics, deployments, lists, Search, Packages, Projects, and workflow/history views, with exact copied selectors and explicit raw `.diff`/`.patch` drill-downs.
- Separated GitHub/provider incompleteness from local context omission, and bounded long human-authored index metadata so large provider pages cannot amplify a default read.
- Reconciled anonymous and authenticated GitHub behavior while preserving crawler fallback for public package pages.
- Preserved insertion order when search results receive tied scores.

## 0.1.1 — 2026-03-22

- Fixed credential loading from binary environment variables and the OS keychain.
- Ignored the local `tmp` workspace.

## 0.1.0 — 2026-03-22

- Ported the webctx CLI to Go.
- Prepared the npm release flow for distributing the Go binary through npm.
- Fixed build-all artifact names.
- Added an environment example for provider credentials.
