---
title: GitHub URL reference
description: Supported GitHub link formats, precise selectors, and large-result behavior.
order: 24
category: Reference
summary: Copy a GitHub URL into read-link and keep its intended meaning.
---

`webctx read-link` recognizes structured GitHub URLs and prefers GitHub's own data over scraping GitHub's HTML. Use the URL you would normally copy from GitHub.

## Common URLs

| You want | Example URL to pass to `webctx read-link` |
| --- | --- |
| Repository overview | `https://github.com/owner/repo` |
| Directory | `https://github.com/owner/repo/tree/main/src` |
| File | `https://github.com/owner/repo/blob/main/README.md` |
| Source line range | `https://github.com/owner/repo/blob/main/README.md#L1-L20` |
| Markdown section | `https://github.com/owner/repo/blob/main/README.md#install` |
| Issue | `https://github.com/owner/repo/issues/42` |
| Exact issue comment | `https://github.com/owner/repo/issues/42#issuecomment-123456` |
| Pull request | `https://github.com/owner/repo/pull/42` |
| PR diff/files | `https://github.com/owner/repo/pull/42/files` |
| PR commits | `https://github.com/owner/repo/pull/42/commits` |
| PR checks | `https://github.com/owner/repo/pull/42/checks` |
| Exact review thread | `https://github.com/owner/repo/pull/42#discussion_r123456` |
| Commit | `https://github.com/owner/repo/commit/<sha>` |
| Compare two refs | `https://github.com/owner/repo/compare/main...feature` |
| File history | `https://github.com/owner/repo/commits/main/src/file.go` |
| Blame | `https://github.com/owner/repo/blame/main/src/file.go` |
| Actions run | `https://github.com/owner/repo/actions/runs/123` |
| Actions job | `https://github.com/owner/repo/actions/runs/123/job/456` |
| Release | `https://github.com/owner/repo/releases/latest` |
| Discussion | `https://github.com/owner/repo/discussions/42` |
| Gist | `https://gist.github.com/owner/<gist-id>` |

Replace the example owner, repository, IDs, and references with real values. URLs containing `#` or query parameters should be quoted in the shell.

## Overview vs exact item vs raw change

- **Overview:** A repository, issue, PR, Actions run, or other container URL returns useful metadata, a bounded preview/index, and links to its children. It is not intended to dump every item at once.
- **Exact item:** A copied line, comment, review, diff, Gist, or check selector asks for that item. WebCTX preserves the selected human-authored text when available rather than applying an overview's size budget.
- **Raw representation:** A `.diff` or `.patch` URL requests the full raw text representation. Use these intentionally when you want bulk changes.

For example:

```bash
# Short overview first
webctx read-link https://github.com/owner/repo/pull/42

# Then a specific review thread
webctx read-link 'https://github.com/owner/repo/pull/42#discussion_r123456'

# Or an explicit raw diff
webctx read-link https://github.com/owner/repo/pull/42.diff
```

## Issues, pull requests, and reviews

Issue roots can include a full small conversation, or a compact timeline for large ones. Follow a copied `#issuecomment-<id>` anchor to read one comment. The `#issue-<id>` anchor targets an issue/PR description when the database ID is known.

PR roots keep descriptions, comments, timeline events, submitted reviews, and inline threads distinct. Common anchors include `#issuecomment-*`, `#pullrequestreview-*`, and `#discussion_r*`. The `/files`, `/commits`, and `/checks` paths expose different PR views. On large files views, use the printed `#diff-*` selector for a particular file or hunk. Check views include `?check_run_id=<id>` links to focus on individual checks.

PR search pages, such as `https://github.com/owner/repo/pulls?q=is%3Apr+is%3Aopen`, retain their search filters. GitHub may represent PR searches under `/issues` with `is:pr`; webctx preserves that meaning.

## Actions, history, and changes

An Actions run returns a compact job index. A specific job URL returns structured steps and, when available, a bounded log preview. The job output includes GitHub's log API URL for a deeper read. The provider's signed download URL expires quickly and is not exposed as a permanent link.

Commit roots include authorship, message, stats, changed-file index, and comment index. Use their printed diff selectors to read a specific file's changes. For bulk text, append `.diff` or `.patch` to a commit or compare URL.

History and blame work at file-path scope. Blame requires GitHub authentication.

## Other supported GitHub views

```bash
# Repository activity and a deployment
webctx read-link https://github.com/cli/cli/activity
webctx read-link https://github.com/owner/repo/deployments/production

# Discussion, including an exact comment
webctx read-link https://github.com/vercel/next.js/discussions/42
webctx read-link 'https://github.com/vercel/next.js/discussions/42#discussioncomment-123456'

# Gist file or comment
webctx read-link 'https://gist.github.com/owner/<gist-id>#file-readme-md-L10-L20'
webctx read-link 'https://gist.github.com/<gist-id>#gistcomment-123456'

# Search, profile, or Project
webctx read-link 'https://github.com/search?q=agent+runtime&type=repositories&s=stars&o=desc'
webctx read-link https://github.com/torvalds
webctx read-link https://github.com/orgs/github/projects/12106
```

Also supported: branches, tags, workflow lists, Packages, releases/assets, contributors, statistics, Gists/revisions, profile tabs, deployments, and other recognized GitHub views. Some views require a token or additional repository permissions.

## Pagination and limits

webctx distinguishes two situations:

- **More data at GitHub:** The API has another page, imposes a ceiling, is still computing, or no longer retains an artifact.
- **Omitted by webctx:** An overview intentionally shows an index instead of expanding every child. Follow the exact URL it prints to get that item.

Normal pageable lists display every row returned in the current provider page and include next/previous links when available. Aggregate overviews can omit entries locally and say so. You do not need a webctx-specific pagination mode; follow the printed GitHub URL.

Large generated logs, machine annotations, and summaries can still be previewed even when a selected item is exact. Explicit full provider URLs are included when available.

## Authentication and error handling

Add `GH_TOKEN` (or `GITHUB_TOKEN`) for higher API capacity, private resources, Discussions, blame, some Actions logs, and richer review state. A narrow fine-grained token may not have permission for every resource; GitHub's permission error remains visible.

Recognized native GitHub resources do **not** fall back to a generic scrape when GitHub returns a rate limit, private/not-found state, or authentication error. The exception is an **exact public GitHub Package page**: if the Package API rejects authentication/permission and Firecrawl is configured, webctx can return a clearly labeled *best-effort* page extraction. Unsupported GitHub URL shapes can use the normal webpage reader.

See [Read a URL](/docs/read-link), [Credentials](/docs/credentials), and [Troubleshooting](/docs/troubleshooting).
