---
title: Read a URL
description: Paste a useful URL and get focused text without manually cleaning browser pages.
order: 11
category: Guides
summary: Keep the meaning of GitHub URLs, read normal pages cleanly, and give agents only the context they asked for.
---

## One command

```bash
webctx read-link <url>
```

`read-link` is useful because the URL often already tells webctx what you want.

A repository URL means “give me an overview.” A source line anchor means “give me these lines.” An Issue URL means “give me the Issue conversation.” A PR review-thread anchor means “give me this thread.” An Actions job URL means “give me this job and its log.”

webctx keeps that intent instead of returning the whole browser page.

## Understand a repository

Start broad:

```bash
webctx read-link https://github.com/amxv/webctx
```

A repository root gives you compact metadata, a README preview, and useful URLs for going deeper.

Read one directory:

```bash
webctx read-link https://github.com/amxv/webctx/tree/main/internal/app
```

Read a file:

```bash
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
```

Read only the lines you care about:

```bash
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#L1-L20'
```

For Markdown files, a heading link can narrow to that section too:

```bash
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#install'
```

## Follow an Issue

```bash
webctx read-link https://github.com/amxv/webctx/issues/6
```

You get the useful Issue body, conversation, and state changes without GitHub navigation chrome.

If you copied a link to one comment, keep the anchor:

```bash
webctx read-link 'https://github.com/<owner>/<repo>/issues/<number>#issuecomment-<id>'
```

webctx uses it to select the comment instead of expanding the whole Issue first.

## Review a Pull Request

Read the conversation:

```bash
webctx read-link https://github.com/amxv/webctx/pull/15
```

Read one inline review thread:

```bash
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'
```

Read the diff, commit list, or checks separately:

```bash
webctx read-link https://github.com/amxv/webctx/pull/15/files
webctx read-link https://github.com/amxv/webctx/pull/15/commits
webctx read-link https://github.com/amxv/webctx/pull/15/checks
```

That separation is intentional. A review conversation, a diff, and CI status are different kinds of context and usually should not be dumped into one response.

If GitHub auth is available, review threads can include extra state such as whether a thread is resolved or outdated.

## Debug CI without dumping every log

Start with a run:

```bash
webctx read-link https://github.com/<owner>/<repo>/actions/runs/<run-id>
```

The run gives you job and artifact state plus exact job URLs. Then open only the job you need:

```bash
webctx read-link https://github.com/<owner>/<repo>/actions/runs/<run-id>/job/<job-id>
```

When GitHub permits the log download, the job URL returns that job's steps and substantive log. It does not pull logs from unrelated jobs.

## Trace how code changed

One commit:

```bash
webctx read-link https://github.com/<owner>/<repo>/commit/<sha>
```

Compare two refs:

```bash
webctx read-link 'https://github.com/<owner>/<repo>/compare/main...feature'
```

History for one path:

```bash
webctx read-link https://github.com/<owner>/<repo>/commits/main/path/to/file.go
```

Blame for one path:

```bash
webctx read-link https://github.com/<owner>/<repo>/blame/main/path/to/file.go
```

Blame uses GitHub's structured blame data and needs `GH_TOKEN` or `GITHUB_TOKEN`.

## Explore the rest of a GitHub project

`read-link` also understands useful views such as:

```bash
# Releases
webctx read-link https://github.com/amxv/webctx/releases/latest

# Repository activity
webctx read-link https://github.com/cli/cli/activity

# Deployment environment
webctx read-link https://github.com/<owner>/<repo>/deployments/production

# Discussion
webctx read-link https://github.com/vercel/next.js/discussions/<number>

# Gist file range
webctx read-link 'https://gist.github.com/<owner>/<gist-id>#file-readme-md-L10-L20'

# GitHub Search page
webctx read-link 'https://github.com/search?q=agent+runtime&type=repositories&s=stars&o=desc'

# User or organization profile
webctx read-link https://github.com/torvalds

# Project
webctx read-link https://github.com/orgs/github/projects/12106
```

You do not need to memorize a separate webctx command for each of these. Copy the GitHub URL and use `read-link`.

## GitHub Packages: best-effort when the API is unavailable

Exact Package URLs first try GitHub's structured Package data:

```bash
webctx read-link https://github.com/orgs/<org>/packages/container/package/<name>
```

GitHub Packages have unusual credential rules. A public Package page can be visible in the browser while the Package API rejects an ordinary fine-grained repository token.

If that happens for an auth or permission error and `FIRECRAWL_API_KEY` is available, webctx can crawl the public Package page instead. The output is clearly marked **best-effort** because page scraping can be incomplete or contain GitHub UI noise.

webctx does **not** use that escape hatch for GitHub rate limits, private/not-found resources, security/admin pages, or unrelated GitHub resource families.

## Normal websites work too

```bash
webctx read-link https://example.com/article
```

For non-GitHub pages, webctx looks for a clean markdown version when possible and falls back to Firecrawl when rendered-page extraction is needed.

The result stays simple:

```markdown
# Page title

**URL:** https://example.com/article

<useful page content>
```

## Add GitHub auth when it helps

Many public GitHub reads work without a token. Add one when you need more capacity or a GitHub view that requires authentication:

```bash
GH_TOKEN=...
```

Useful authenticated cases include:

- blame
- Discussions
- some Actions job logs
- richer PR review-thread state
- private resources your token can access

Fine-grained tokens can have narrower permissions than GitHub's public surface. For selected public reads, webctx can retry without Authorization if the token is rejected.

## Trust the warnings

Some provider results are naturally bounded, truncated, still computing, expired, or permission-limited. webctx calls those states out instead of pretending the result is complete.

For an agent, that distinction matters: clean context is only useful when it is also honest about what it could not see.
