---
title: Use webctx with agents
description: Practical copy-paste workflows for research, repository understanding, code review, CI debugging, and documentation work.
order: 13
category: Guides
summary: Small command sequences that give an agent focused evidence instead of browser noise.
---

## The basic pattern

webctx is most useful as a context feeder:

```text
find useful URLs → read the exact thing you need → hand the text to an agent
```

The output is plain markdown, so there is no special integration step.

## Understand an unfamiliar repository

```bash
webctx read-link https://github.com/amxv/webctx
webctx read-link https://github.com/amxv/webctx/tree/main/internal/app
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/tools.go#L130-L220'
webctx read-link https://github.com/amxv/webctx/issues/6
```

Why this works:

- the repository root gives orientation
- the tree shows one part of the codebase
- the line link narrows source before it reaches the model
- the Issue adds product/history context

You get a useful sequence of evidence without asking the agent to scrape GitHub pages itself.

## Review a Pull Request

```bash
webctx read-link https://github.com/amxv/webctx/pull/15
webctx read-link https://github.com/amxv/webctx/pull/15/files
webctx read-link https://github.com/amxv/webctx/pull/15/checks
```

If a specific review comment matters, use its copied link:

```bash
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'
```

That gives the agent the thread it needs instead of a giant PR page.

## Debug a failing CI job

Start with the run:

```bash
webctx read-link https://github.com/<owner>/<repo>/actions/runs/<run-id>
```

Then open the failed job URL returned by the run:

```bash
webctx read-link https://github.com/<owner>/<repo>/actions/runs/<run-id>/job/<job-id>
```

This is especially useful for coding agents because the job read stays scoped to one log rather than filling context with every job in the workflow.

## Find why a line changed

```bash
webctx read-link https://github.com/<owner>/<repo>/commits/main/path/to/file.go
webctx read-link https://github.com/<owner>/<repo>/commit/<sha>
webctx read-link https://github.com/<owner>/<repo>/blame/main/path/to/file.go
```

Use history to find candidate commits, open the relevant commit, then use blame when you need line-by-line ownership. Blame needs GitHub auth.

## Research current documentation

```bash
webctx search "OpenAI Apps SDK MCP annotations"
webctx read-link https://developers.openai.com/apps-sdk/reference
```

Use search for discovery, not as the final context. Read the primary page once you find it.

## Audit a whole docs site

```bash
webctx map-site https://some-docs.example
webctx read-link https://some-docs.example/getting-started
webctx read-link https://some-docs.example/api/reference
webctx read-link https://some-docs.example/changelog
```

An agent can inspect the URL map first and choose the pages relevant to the task instead of recursively crawling everything.

## Follow a GitHub conversation

```bash
webctx read-link https://github.com/<owner>/<repo>/issues/<number>
webctx read-link https://github.com/<owner>/<repo>/pull/<number>
webctx read-link https://github.com/<owner>/<repo>/discussions/<number>
```

Issue, PR, and Discussion roots are useful navigation surfaces rather than instructions to expand every child page. Large roots keep compact previews/indexes and return copied GitHub URLs for exact comments or threads. Discussions need GitHub auth for the structured reader.

## Keep agent context small

Prefer the most specific URL you have:

```bash
# Better than the whole file
webctx read-link 'https://github.com/<owner>/<repo>/blob/main/file.go#L80-L120'

# Better than the whole PR
webctx read-link 'https://github.com/<owner>/<repo>/pull/42#discussion_r123456'

# Better than every Discussion reply
webctx read-link 'https://github.com/<owner>/<repo>/discussions/42#discussioncomment-123456'

# Better than every Gist file/comment
webctx read-link 'https://gist.github.com/<gist-id>#gistcomment-123456'

# Better than every Actions log
webctx read-link https://github.com/<owner>/<repo>/actions/runs/123/job/456
```

The easiest way to save tokens is often to narrow the source **before** the model sees it.

When an overview says GitHub has more provider pages or that some returned rows were locally omitted, follow the exact/page URL it prints. Do not invent a webctx `--page`, `--all`, or “full” switch: copied provider URLs are the navigation contract. Use an explicit `.diff`/`.patch` URL when bulk raw change text is actually what the task needs.

## Let the URL do the routing

You do not need an agent prompt full of GitHub parsing rules. Teach the agent one command:

```bash
webctx read-link <url>
```

Then let it pass through the exact GitHub URLs it discovers—repository, source, Issue, PR, review thread, diff, check, commit, Actions job, release, Gist, Project, and so on.

That is the core advantage: the agent can keep working in URLs it already understands while webctx turns those URLs into cleaner context.
