---
title: Changelog
description: "Release notes for webctx."
order: 99
category: Reference
summary: Version-by-version changes for the webctx CLI.
---

This changelog tracks code and product changes in webctx. It intentionally skips docs-site-only updates.

## 0.2.4 - 2026-10-10

- Fixed native Markdown retrieval for documentation sites: use bounded GET requests rather than positive HEAD content-length assumptions, and recognize directory index.md routes. Canonical Stripe and Cloudflare URLs now prefer publisher Markdown while retaining the original URL and precise Markdown source URL.
- Hardened long CAPTCHA/challenge detection before accepting Firecrawl or browser fallback responses. Failed challenge resolution is reported instead of leaking hCaptcha boilerplate; optional WEBCTX_DEBUG=1 traces retrieval stage, outcome, and duration without URL query credentials.
- Improved question-driven exploration with publisher llms.txt indices, same-topic prioritization, getting-started guides, REST documentation, webhook instructions, and follow-ups when explicit requested evidence is missing.
- Preserved complete code fences and fixed excerpt-budget escape for JSON-rich Markdown. Legacy migration content is demoted for new-project questions, and missing requested fields are called out rather than implied.
- Added regression tests based on live Origo MCP cases covering Stripe Checkout, Cloudflare Durable Objects, and Alexandria REST documentation. Origo continues exposing exactly two read-only tools and sharing the Webctx Go backend.

## 0.2.3 - 2026-10-10

- Improved question-focused reads in Webctx CLI and Origo MCP: complete code, cURL, and API examples are retained rather than truncated in the middle of a fence.
- Follow the published llms.txt index to the canonical OpenAPI specification for REST/API questions. Include exact method and path contracts, schema fields, and published request payloads, with authoritative source URLs.
- Fixed the Alexandria case: discovery and execution examples stay intact, and the missing firecrawl/find-tools inspection REST example comes from Firecrawl's published OpenAPI specification.
- Preserve existing URL-only behavior and Origo's two-tool MCP interface; no paid Alexandria provider execution is performed.

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
