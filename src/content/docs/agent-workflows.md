---
title: Agent workflows
description: Use webctx as a terminal context tool for coding agents, research agents, and documentation agents.
order: 10
category: Start
summary: Practical ways to feed web search, markdown pages, and site maps into agent loops.
---

## Why agents like webctx

Agents often need web context in a form that is easy to paste, store, diff, summarize, or cite. webctx returns terminal-friendly text instead of browser UI state.

The three useful primitives are:

```text
search for candidate pages
read a page as markdown
map a site into URL candidates
```

## Research loop

```bash
webctx search "OpenAI Apps SDK MCP tool annotations"
webctx read-link https://developers.openai.com/apps-sdk/reference
```

Use this when an agent needs recent docs or implementation context before editing code.

## Documentation audit loop

```bash
webctx map-site https://docs.firecrawl.dev
webctx read-link https://docs.firecrawl.dev/introduction
webctx read-link https://docs.firecrawl.dev/api-reference/endpoint/scrape
```

Use this to gather the important pages before asking an agent to write or update docs.

## Repo understanding loop

```bash
webctx read-link https://github.com/amxv/webctx
webctx read-link https://github.com/amxv/webctx/blob/main/CONTRIBUTORS.md
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/app.go#L130-L180'
webctx read-link https://github.com/amxv/webctx/blob/main/docs/porting-status.md
webctx read-link https://github.com/amxv/webctx/tree/main/internal/app
webctx read-link https://github.com/amxv/webctx/issues/6
webctx read-link 'https://github.com/amxv/webctx/issues?q=is%3Aissue'
webctx read-link https://github.com/cli/cli/pull/13250
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'
webctx read-link 'https://github.com/cli/cli/pull/13250/files#diff-553490f999984ba28c4af0d7ffa919d10b5419f04a73f00141ee0b5a51c142e6R24'
webctx read-link https://github.com/cli/cli/pull/13250/commits
webctx read-link https://github.com/cli/cli/pull/13250/checks
webctx read-link https://github.com/amxv/webctx/commit/c6d90181d7caffe6d41458eed696eb5fb48b177f
webctx read-link 'https://github.com/cli/cli/compare/trunk...trunk'
webctx read-link https://github.com/cli/cli/commits/andyfeller/test/README.md
webctx read-link https://github.com/amxv/webctx/actions
webctx read-link https://github.com/amxv/webctx/actions/runs/<run-id>
webctx read-link https://github.com/amxv/webctx/actions/runs/<run-id>/job/<job-id>
webctx read-link https://github.com/amxv/webctx/branches
webctx read-link https://github.com/amxv/webctx/releases/latest
webctx read-link https://github.com/vercel/next.js/discussions/35773
webctx read-link 'https://gist.github.com/<owner>/<gist-id>#file-readme-md-L10-L20'
```

The repository root is an orientation read: compact metadata, a bounded README preview, and native navigation hints. Direct public blobs use the raw-content fast path and return the full source; line fragments narrow a file before it reaches the agent. Tree URLs return a one-level listing without GitHub navigation chrome. Issue and Pull Request URLs preserve substantive conversation, while Issue/search/label/milestone list views remain compact and bounded.

When you already have an exact Issue comment URL, keep its `#issuecomment-...` fragment. webctx resolves that comment directly instead of reading the whole Issue first.

For Pull Requests, keep exact `#issuecomment-...`, `#discussion_r...`, and `#pullrequestreview-...` anchors too. They select a normal PR comment, one inline review thread, or one formal review rather than returning the entire PR conversation. Public PR conversations work anonymously; optional GitHub auth adds resolved/outdated review-thread state when GitHub GraphQL is available.

Use the PR tab URL when you need a different kind of context rather than filtering a full conversation afterward: `/files` returns patches, `/commits` returns the PR commit list, and `/checks` returns Check Runs plus commit statuses. Files Changed links copied with `#diff-...L/R...` narrow to the selected file/hunk before output. A checks URL with `?check_run_id=...` returns only that check and its annotations.

For repository history, use the URL that already encodes the scope you need. `/commit/<sha>` returns one commit plus its files/comments; `/compare/<base>...<head>` returns comparison state/commits/files; `/commits/<ref>/<path>` gives one bounded path-history page. Slash-containing refs are provider-resolved instead of split at a fixed segment.

Blame is the exception to ordinary public reads: `/blame/<ref>/<path>` needs `GH_TOKEN` or `GITHUB_TOKEN` because webctx uses GitHub GraphQL's structured blame ranges. Without a token it fails early with auth guidance rather than scraping the UI.

For Actions, start broad and narrow by URL. `/actions` or `/actions/workflows/<workflow>` gives a bounded run list; `/actions/runs/<run>` gives run/job/artifact state without logs; the canonical `/actions/runs/<run>/job/<job>` URL fetches only that job and its log. This keeps a large workflow from dumping every job log into the agent's context.

Repository navigation stays URL-scoped too. Use `/branches` or `/tags` to discover refs, then follow the returned `/tree/<ref>` link for source. Use `/releases` for a compact release index and `/releases/tag/<tag>` or `/releases/latest` when you actually want the full notes/assets. `/stargazers` and `/watchers` are intentionally different lists: stars versus real subscribers/watchers.

Discussions and Gists have different auth/completeness rules. Discussions need a GitHub token because the structured conversation comes from GraphQL; exact Discussions paginate the whole comment/reply conversation. Public Gists do not need auth, and a copied `#file-...-L...` anchor is the cheapest way to select one file/range. If GitHub marks a Gist file truncated, follow the raw URL rather than trusting the partial API body.

For private repository source, configure optional `GH_TOKEN` or `GITHUB_TOKEN`. Public GitHub reads stay anonymous by default.

## Noise control

Use default and custom exclusions when web search is returning low-value result types:

```bash
webctx search "go cli release npm native binary" --exclude reddit.com,medium.com
```

Default exclusions already remove common video and social domains.

## Keyword targeting

When a query needs a very specific phrase, use Exa keyword mode:

```bash
webctx search "firecrawl api" --keyword "maxAge excludeTags"
```

The keyword is truncated to five words before being sent as Exa include-text criteria.
