---
title: Read a URL
description: Read any useful URL as clean text; webctx automatically chooses native, direct-markdown, or crawl paths.
order: 11
category: Guides
summary: Give webctx a URL; it chooses the cleanest available read path and preserves precise URL intent when it can.
---

## Read any useful URL

```bash
webctx read-link <url>
```

`read-link` is the general page-reading command. Give it a documentation page, an article, a GitHub URL, a Gist, or another useful web page and get terminal-friendly text back.

```bash
# Normal web page
webctx read-link https://docs.firecrawl.dev/introduction

# Markdown/source page
webctx read-link https://github.com/amxv/webctx/blob/main/README.md

# Precise GitHub resource
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'
```

You do not choose a parser. webctx chooses the cleanest available path automatically.

## What happens automatically

webctx prefers more direct and structured reads before paying for a full page crawl:

```text
supported structured URL → native/direct provider read
clean markdown available → fetch markdown directly
otherwise               → Firecrawl page extraction
```

GitHub has the deepest optimization because its URLs often encode exactly what you want: a repository, source range, Issue, PR thread, CI job, commit, or other structured resource. Normal web pages are still first-class inputs; they simply use the direct-markdown or Firecrawl paths when there is no richer native interpretation.

See [How URL reading works](/docs/architecture) for the short technical model.

## Read a normal website

```bash
webctx read-link https://example.com/article
```

When a clean markdown representation is available, webctx prefers it. Otherwise it uses Firecrawl to extract the rendered page. The result stays simple:

```markdown
# Page title

**URL:** https://example.com/article

<useful page content>
```

## GitHub URLs keep their meaning

A GitHub URL is more than a page address. Its path, query, and fragment often describe the exact context you meant to copy. webctx preserves that intent instead of flattening everything into a generic page scrape.

For example, a repository URL means “give me an overview,” while a source line anchor means “give me these lines.” An Issue or PR root is a navigable overview when the conversation is large; a copied comment/thread anchor means “give me this exact item.” An Actions job URL means “give me this job's structured steps plus a bounded log preview.”

That distinction is intentional:

- **Overview URLs** keep metadata, the most useful rows/previews, exact child URLs, and truthful omission/provider-limit facts near a compact context budget.
- **Exact URLs** such as copied source/comment/thread/file/check selectors narrow to the selected semantic item. Human-authored selected text is kept complete when the provider makes that identity available.
- **Raw URLs** such as commit/PR/compare `.diff` and `.patch` representations stay explicit bulk reads rather than being silently summarized.

When a provider page contains more data than a compact view should expand, webctx separates **more data upstream** from **rows omitted locally**. Follow the copied GitHub URL for the next page or exact child; there is no separate webctx pagination language to learn.

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

Small Issues stay in the useful full-conversation form when the bounded provider data is complete and the result remains compact. Larger or provider-paginated Issues switch to an overview with a body preview, relationship facts, a bounded chronological timeline index, and separate notes for provider continuation versus local omission.

To read the full Issue description without expanding the conversation, keep GitHub's Issue-body anchor:

```bash
webctx read-link 'https://github.com/<owner>/<repo>/issues/<number>#issue-<issue-id>'
```

Large Issue overviews include this exact URL when GitHub supplies the Issue database ID.

If you copied a link to one comment, keep the anchor:

```bash
webctx read-link 'https://github.com/<owner>/<repo>/issues/<number>#issuecomment-<id>'
```

webctx uses it to select the comment instead of expanding the whole Issue first.

## Review a Pull Request

Browse a repository's Pull Requests without GitHub page chrome:

```bash
webctx read-link https://github.com/amxv/webctx/pulls
webctx read-link 'https://github.com/vercel/next.js/pulls?q=is%3Apr+is%3Aopen'
```

Copied PR searches stay PR searches even when GitHub uses an `/issues?q=is:pr...` URL. Native pageable lists use compact provider pages and render every row returned on that page, so following the printed Next/Previous URL does not skip locally hidden rows.

Read the Pull Request overview:

```bash
webctx read-link https://github.com/amxv/webctx/pull/15
```

The root stays compact: it previews the description and gives ordinary conversation comments their own selector index, separate from compact timeline/state events, submitted reviews, and inline review threads. Follow the printed `#issue-<id>` URL when you need the complete PR description without loading the rest of the conversation.

If you already copied a GitHub anchor, keep it. `#issuecomment-*`, `#pullrequestreview-*`, and `#discussion_r*` select the corresponding full comment, review, or thread instead of rebuilding the PR root.

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

That separation is intentional. A review conversation, a diff, a commit index, and CI status are different kinds of context and usually should not be dumped into one response. Small Files views can still show complete patches; large ones switch to a compact file index with exact `#diff-*` selectors so you can open only the file or hunk you need. PR commit lists keep a compact commit index and call out GitHub's own provider ceiling separately from anything webctx omits locally.

Checks start with status/conclusion rollups and put failures or active work ahead of routine successes. Every indexed check run includes a focused `?check_run_id=<id>` URL. Focused checks keep source annotation coordinates while previewing oversized machine-generated summaries/details and link the provider's deeper Details URL when available.

If GitHub auth is available, review threads can include extra state such as whether a thread is resolved or outdated. Root overviews only read the first bounded GraphQL thread-state page; exact thread URLs can do deeper provider work when that selected thread requires it.

## Debug CI without dumping every log

Start with a run:

```bash
webctx read-link https://github.com/<owner>/<repo>/actions/runs/<run-id>
```

The run gives you job-state rollups, a failure/active-first job index, artifact totals, and exact job URLs without expanding every artifact. Then open only the job you need:

```bash
webctx read-link https://github.com/<owner>/<repo>/actions/runs/<run-id>/job/<job-id>
```

When GitHub permits the log download, the job URL returns every structured step plus a bounded log preview. Failed jobs bias the preview toward failed-step/error context; other large logs use a deterministic head/tail preview. The output also includes GitHub's stable job-log API endpoint for an explicit full-log read. GitHub redirects that endpoint to a signed download URL that expires after one minute, and webctx does not print that redirect URL.

## Trace how code changed

One commit:

```bash
webctx read-link https://github.com/<owner>/<repo>/commit/<sha>
```

Commit roots stay compact: the commit message, authorship/verification/stats, parents, changed-file index, and commit-comment index are kept, while file patches and long comment bodies are not expanded into the root. Every indexed file uses GitHub's exact `#diff-<sha256(path)>` selector; copied left/right line-range anchors narrow that selected patch further. Copied `#commitcomment-<id>` anchors read only that commit comment.

When you explicitly want the complete textual change instead of the semantic overview, use the raw representations printed by the commit output:

```bash
webctx read-link https://github.com/<owner>/<repo>/commit/<sha>.diff
webctx read-link https://github.com/<owner>/<repo>/commit/<sha>.patch
```

Compare two refs:

```bash
webctx read-link 'https://github.com/<owner>/<repo>/compare/main...feature'
```

Compare roots likewise keep bounded commit/file indexes and provider completeness facts instead of embedding every patch or chasing every commit page. Use the printed `.diff` or `.patch` comparison URL when the raw comparison is what you actually need.

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

# Exact Gist comment
webctx read-link 'https://gist.github.com/<gist-id>#gistcomment-<id>'

# GitHub Search page
webctx read-link 'https://github.com/search?q=agent+runtime&type=repositories&s=stars&o=desc'

# User or organization profile
webctx read-link https://github.com/torvalds

# Project
webctx read-link https://github.com/orgs/github/projects/12106
```

You do not need to memorize a separate webctx command for each of these. Copy the GitHub URL and use `read-link`.

### Discussions and Gists stay navigable

Discussion roots preview the main post and keep a compact conversation index instead of expanding every reply page. Each indexed comment or reply keeps GitHub's copied `#discussioncomment-<id>` URL; pass that exact URL back to `read-link` when you need the complete selected comment. Accepted answers remain identified in the overview.

Gist roots similarly index files, comments, and revisions without fetching every full file into one response. Use a copied `#file-*` anchor for the complete selected file or line range, or `#gistcomment-<id>` for one exact Gist comment. Both owner-qualified Gist links and GitHub's ownerless hash-ID links are supported.

## GitHub Packages: best-effort when the API is unavailable

Exact Package URLs first try GitHub's structured Package data:

```bash
webctx read-link https://github.com/orgs/<org>/packages/container/package/<name>
```

GitHub Packages have unusual credential rules. A public Package page can be visible in the browser while the Package API rejects an ordinary fine-grained repository token.

If that happens for an auth or permission error and `FIRECRAWL_API_KEY` is available, webctx can crawl the public Package page instead. The output is clearly marked **best-effort** because page scraping can be incomplete or contain GitHub UI noise.

webctx does **not** use that escape hatch for GitHub rate limits, private/not-found resources, security/admin pages, or unrelated GitHub resource families.

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

Recognized native GitHub reads also keep provider failures authoritative. If GitHub says a resource needs authentication, is private/not found, or is rate-limited, webctx does not silently replace that structured failure with a page scrape. Unsupported GitHub routes that webctx does not claim as native can still use the normal direct-markdown/Firecrawl path. Exact public Package pages are the one documented best-effort exception described above.

## Large GitHub repository views

Some GitHub pages can represent thousands of child objects. `webctx` keeps those pages navigable instead of dumping the whole provider response into one read.

- **Release details** preview long notes and index a bounded set of assets while keeping the canonical GitHub release page for the complete notes and downloads.
- **Repository trees** index a bounded set of entries and preview the README. If GitHub returns its 1,000-entry Contents API ceiling, that provider limit is reported separately from anything `webctx` omits locally.
- **Contributor statistics** prioritize the highest commit totals and bound the displayed contributor rows instead of printing every contributor. GitHub's fixed weekly statistics stay compact, and a temporary `202` remains a provider-computing state rather than an empty result.
- **Deployments** stay identity-first. Environment detail shows the latest returned status for each bounded deployment and notes when older status history exists upstream rather than expanding the entire history inline.
- **Code-frequency statistics** retain aggregate additions/deletions across the provider result while indexing only the recent weekly buckets when a repository has a long history.
- **Pageable lists** such as Issues, PRs, branches, tags, releases, search results, workflows, profile tabs, Packages, activity, deployments, and commit history ask GitHub for a small stable page and render every returned row. Their printed Next/Previous URLs therefore resume without skipping a locally omitted tail.
- **Aggregate/root indexes** such as a PR conversation map, Actions run, large Files/Checks view, Project, tree, contributor statistics, or release assets may still omit child rows locally; those views report the omission separately and print exact child/subresource URLs wherever GitHub provides them.

Use the direct GitHub URLs printed in these views when you need to move from the compact overview to the provider's full page.
