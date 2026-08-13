---
title: Read-link command
description: Convert URLs into clean text with native GitHub repository/source/Issue/Pull Request reads, direct markdown detection, and Firecrawl fallback scraping.
order: 5
category: Commands
summary: The behavior of `webctx read-link`.
---

## Basic usage

```bash
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
```

`read-link` returns terminal-friendly markdown/text. Normal pages keep the familiar title, original URL, and extracted content shape; native GitHub repository/source/Issue/Pull Request/commit/compare/history/list views use compact structured metadata instead of scraped GitHub chrome. Raw diff/patch views remain raw.

## Native GitHub repository reads

Repository roots such as:

```bash
webctx read-link https://github.com/amxv/webctx
```

return a compact frontmatter-style block with high-signal repository metadata, followed by a README preview targeted at roughly 5,000 Unicode characters. The preview is cut at a safe Markdown boundary where possible, and invisible HTML comments from the human repository view are removed.

If the README is longer, webctx includes GitHub's canonical README blob URL for the full source. The root output also includes a short set of useful source/tree/Issue/Pull Request URL forms supported by the native reader.

## Blob reads and selectors

Public blob URLs keep the direct raw-content fast path:

```bash
webctx read-link https://github.com/amxv/webctx/blob/main/README.md
```

A direct blob is an explicit source request, so it returns the full text file by default rather than applying the repository-root preview cap. Source HTML comments are preserved.

GitHub line fragments narrow source output:

```bash
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/app.go#L20'
webctx read-link 'https://github.com/amxv/webctx/blob/main/internal/app/app.go#L20-L40'
```

For Markdown files, a heading fragment returns that heading through the next heading of equal or higher level:

```bash
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#install'
```

Unknown, reversed, or out-of-range selectors fail explicitly instead of silently returning a different section or the whole file. Binary files are identified without dumping arbitrary bytes. GitHub's 100 MB Contents/Git-blob provider boundary is surfaced instead of presenting an incomplete source as complete.

## Tree reads

Tree URLs return one directory level rather than repository navigation chrome:

```bash
webctx read-link https://github.com/amxv/webctx/tree/main/internal
```

When the selected directory contains a README, webctx adds a bounded human-view preview and a full blob URL if that preview is truncated. A 1,000-entry Contents API result is marked as potentially incomplete because that is GitHub's documented directory ceiling.

Blob and tree refs are not split at the first slash. Slash-containing branches/tags are resolved against GitHub when ref/path identity is needed, and genuinely overlapping valid splits fail as ambiguous rather than being guessed.

## Optional GitHub authentication

Public repository/source reads work anonymously. To read private source that your account can access, configure `GH_TOKEN` or `GITHUB_TOKEN`; when both contain values, `GH_TOKEN` wins.

Native GitHub API requests send GitHub's pinned REST version header and preserve provider status/rate-limit information. Recognized native failures remain GitHub-specific, so a private/not-found/rate-limited resource is not silently replaced by a scraped login or error page.

## Issue reads, comments, and lists

Repository Issue URLs are native reads:

```bash
webctx read-link https://github.com/cli/cli/issues/14134
```

An Issue detail returns compact state/title/author/time metadata, the human-visible body, substantive timeline changes, and the complete selected conversation by following GitHub's returned pagination links. Bot and non-maintainer comments are preserved. Invisible HTML automation markers are removed from Issue/comment bodies without changing direct source-file behavior.

When GitHub exposes them, the read also includes current parent/sub-issue relationships, blocked-by/blocking dependencies, Issue type, milestone, labels, assignees, pin state, and Issue field values. A resource that is actually a Pull Request is not flattened into an Issue conversation; the canonical Pull Request URL is reported instead.

An exact Issue comment fragment narrows the provider read before rendering:

```bash
webctx read-link 'https://github.com/cli/cli/issues/14134#issuecomment-5261879950'
```

Repository Issue list/search URLs stay bounded to the selected page and preserve useful filters/query/page state:

```bash
webctx read-link 'https://github.com/amxv/webctx/issues?state=all&page=1'
webctx read-link 'https://github.com/amxv/webctx/issues?q=is%3Aissue'
```

Pull Requests returned by GitHub's Issues REST/Search APIs are excluded from Issue-list views. GitHub's search `incomplete_results` signal and the actual `search` rate-limit resource are surfaced rather than presented as complete/core-quota results.

Stable repository label and milestone pages are native too:

```bash
webctx read-link https://github.com/amxv/webctx/labels
webctx read-link https://github.com/amxv/webctx/labels/enhancement
webctx read-link https://github.com/amxv/webctx/milestones
```

These list/detail views stay page-bounded and link to the Issues they describe rather than recursively expanding conversations.

## Pull Request conversations and exact anchors

A Pull Request conversation URL is native:

```bash
webctx read-link https://github.com/cli/cli/pull/13250
```

The conversation read combines compact PR identity/state/base/head/change-count metadata with the human-visible body, normal Issue-style comments, meaningful timeline transitions, submitted reviews, and inline review comments grouped into threads. Timeline `reviewed` events are not re-rendered when the same review comes from GitHub's complete reviews endpoint, so review bodies appear once. Bot and non-member content is preserved.

Inline threads are reconstructed from REST `in_reply_to_id`, path, and line/range coordinates, so anonymous public reads retain the substantive thread even without GraphQL. When `GH_TOKEN` or `GITHUB_TOKEN` is configured, webctx additionally asks GitHub GraphQL for provider-truthful resolved/outdated thread state. If that optional enrichment fails, the successful REST conversation remains available and the output says only that enrichment was unavailable.

Copied GitHub conversation anchors narrow the read before rendering unrelated PR content:

```bash
webctx read-link 'https://github.com/cli/cli/pull/13250#issuecomment-4447874096'
webctx read-link 'https://github.com/cli/cli/pull/13250#discussion_r3118513169'
webctx read-link 'https://github.com/cli/cli/pull/13250#pullrequestreview-4148860648'
```

The first uses the shared normal-comment identity from Issues. A `discussion_r` anchor returns that inline thread with reply context. A `pullrequestreview` anchor returns the selected formal review and its review comments.

The conversation output points to the PR's `/files`, `/commits`, and `/checks` views as useful next URLs. Those focused tabs are separate native resource views rather than being expanded into the conversation.

## Pull Request files, commits, and checks

The Files Changed view returns changed files and provider patches, not PR conversation text:

```bash
webctx read-link https://github.com/cli/cli/pull/13250/files
```

GitHub's current file fragment convention is supported directly. A file hash is SHA-256 of the path, and optional `L`/`R` line suffixes select the provider patch hunk for old/new-side lines:

```bash
webctx read-link 'https://github.com/cli/cli/pull/13250/files#diff-553490f999984ba28c4af0d7ffa919d10b5419f04a73f00141ee0b5a51c142e6'
webctx read-link 'https://github.com/cli/cli/pull/13250/files#diff-553490f999984ba28c4af0d7ffa919d10b5419f04a73f00141ee0b5a51c142e6R24'
```

Unknown/stale hashes or lines fail explicitly instead of selecting a different file/hunk. Renames retain the previous filename. When GitHub omits a patch—for example binary, oversized, or otherwise provider-omitted content—webctx says the patch is unavailable rather than inventing diff text. GitHub's documented 3,000-file Pull Request maximum is surfaced as an incomplete provider boundary if reached.

The commits view is compact and does not expand file patches:

```bash
webctx read-link https://github.com/cli/cli/pull/13250/commits
```

The checks view uses the PR head commit and keeps GitHub Check Runs separate from legacy commit statuses:

```bash
webctx read-link https://github.com/cli/cli/pull/13250/checks
```

Both check-run pages and combined-status pages follow GitHub's returned pagination links. webctx displays the provider's combined status state but does not turn Check Runs/statuses into a made-up branch-protection or mergeability verdict.

When GitHub's current checks URL carries a `check_run_id`, only that Check Run and all of its paginated annotations are returned:

```bash
webctx read-link 'https://github.com/cli/cli/pull/13250/checks?check_run_id=75849937564'
```

The selected check is verified against the PR head SHA before annotations are rendered.

Direct PR diff/patch forms retain raw media semantics:

```bash
webctx read-link https://github.com/cli/cli/pull/13250.diff
webctx read-link https://github.com/cli/cli/pull/13250.patch
```

These are not wrapped as a conversation or reformatted into per-file Markdown sections.

## Commits, comparisons, history, and blame

A commit URL returns the selected commit rather than repository chrome:

```bash
webctx read-link https://github.com/amxv/webctx/commit/c6d90181d7caffe6d41458eed696eb5fb48b177f
```

The compact metadata includes commit identity, author/committer timestamps, verification state, and change totals. The commit message is preserved, changed files/patches follow GitHub's returned pagination, and substantive commit comments are included with human-view invisible HTML comments removed. GitHub documents a 3,000-file maximum for a commit's JSON file listing; webctx marks that provider ceiling as incomplete instead of presenting it as a complete diff.

Direct commit diff/patch forms preserve GitHub's raw media:

```bash
webctx read-link https://github.com/amxv/webctx/commit/c6d90181d7caffe6d41458eed696eb5fb48b177f.diff
webctx read-link https://github.com/amxv/webctx/commit/c6d90181d7caffe6d41458eed696eb5fb48b177f.patch
```

Comparison URLs keep the base/head relationship visible:

```bash
webctx read-link 'https://github.com/cli/cli/compare/trunk...feature/foo'
```

The native projection includes status, ahead/behind counts, every comparison commit returned through GitHub pagination, and the changed-file patches available on the first provider page. GitHub's compare API exposes files only on that first page and up to 300 files, so reaching that boundary is surfaced as potentially incomplete. Comparison `.diff`/`.patch` forms preserve raw media in the same way as commits and Pull Requests.

Path history stays bounded to the selected GitHub page instead of expanding the repository's entire history:

```bash
webctx read-link 'https://github.com/cli/cli/commits/andyfeller/test/README.md'
webctx read-link 'https://github.com/cli/cli/commits/andyfeller/test/README.md?page=2'
```

The ref/path split is provider-resolved against commit history, so slash-containing refs work without assuming the first post-route segment is the whole branch. Ambiguous valid splits fail explicitly. Returned `Link` pagination becomes concise previous/next GitHub URLs.

Blame uses GitHub's structured GraphQL ranges and therefore requires authentication:

```bash
webctx read-link https://github.com/cli/cli/blame/andyfeller/test/README.md
```

With `GH_TOKEN` or `GITHUB_TOKEN`, webctx resolves the slash-containing ref/path and returns compact line ranges with commit, author, date, and message identity. Without a token it returns a concise auth-required result before making provider requests; it never disguises missing GraphQL auth by scraping the blame page.

## GitHub Actions runs, jobs, logs, and artifacts

Repository Actions and workflow URLs are bounded navigation reads:

```bash
webctx read-link https://github.com/amxv/webctx/actions
webctx read-link https://github.com/amxv/webctx/actions/workflows/release.yml
```

The repository view returns a compact workflow list plus one selected page of workflow runs. Workflow detail returns workflow identity/state and one selected page of its runs. Stable REST-style filters such as branch, event, status, actor, creation range, head SHA, and page are preserved when present; an unproven GitHub UI `query=` filter is rejected rather than silently ignored.

A selected workflow run is a structured overview, not a log dump:

```bash
webctx read-link https://github.com/amxv/webctx/actions/runs/<run-id>
```

It includes run identity/status/conclusion/event/ref/SHA/timestamps plus all paginated jobs for the latest attempt and all paginated artifacts. Each job uses GitHub's exact `html_url` when available, so an agent can request only the job it needs. Artifact metadata includes size, expiry state/time, and GitHub's archive download URL; an expired artifact is labeled expired rather than offered as though it were still downloadable.

Only a canonical job URL fetches a job log:

```bash
webctx read-link https://github.com/amxv/webctx/actions/runs/<run-id>/job/<job-id>
```

The reader first verifies that the job belongs to the selected run, renders the structured job steps, then fetches only that job's log. GitHub log delivery may redirect to storage; webctx relies on redirect-safe HTTP behavior so the configured GitHub Authorization header is not forwarded to a different storage host. Plaintext logs are preserved, ZIP-delivered log archives are unpacked deterministically, and malformed/non-text/oversized archives fail truthfully rather than appearing as successful logs.

If GitHub reports a job log as gone/not found—for example expired, deleted, or not yet generated—the job metadata still renders with an explicit unavailable-log note. Run pages never prefetch logs. webctx does not advertise or accept Actions step fragments because a stable current GitHub step-fragment contract has not been proven.

Check Run annotations remain available through the native PR/check targeting described above; Actions run/job output does not duplicate them as a second inconsistent model.

## Branches, tags, releases, and repository navigation

Repository branch and tag lists are compact, bounded navigation reads:

```bash
webctx read-link https://github.com/amxv/webctx/branches
webctx read-link https://github.com/amxv/webctx/tags
```

Branches include the current commit SHA and protected state when GitHub reports it. Tags include their commit SHA. Both link into the existing native `/tree/<ref>` source reader, so slash-containing refs remain source-navigation URLs rather than a competing synthetic branch-detail route.

Release lists stay bounded and never expand every release body:

```bash
webctx read-link https://github.com/amxv/webctx/releases
```

An exact release or latest release is authoritative and preserves the full selected release notes rather than the repository-root 5k preview budget:

```bash
webctx read-link https://github.com/amxv/webctx/releases/latest
webctx read-link https://github.com/amxv/webctx/releases/tag/v0.1.1
```

Release metadata includes tag/target/author/times/draft/prerelease state. Assets are fetched through all GitHub pagination and rendered with size, media type, download count, and browser download URL. Invisible HTML comments are removed from the human-view release body just as they are from Issues and Pull Requests.

Stable repository social/navigation lists are also bounded:

```bash
webctx read-link https://github.com/amxv/webctx/forks
webctx read-link https://github.com/amxv/webctx/stargazers
webctx read-link https://github.com/amxv/webctx/watchers
```

`/stargazers` means users who starred the repository and uses GitHub's stargazer media type when available so star timestamps are preserved. `/watchers` deliberately maps to GitHub's subscribers endpoint: these are actual repository watchers/subscribers, **not** `watchers_count`. GitHub's REST `watchers`/`watchers_count` fields are historical aliases for star count, so webctx never labels those aliases as subscriber/watch counts. Fork rows likewise use `stargazers_count` when showing stars.

These list views preserve only provider-backed filters/pagination they can represent faithfully and reject unknown UI query parameters rather than silently changing the selected list.

GitHub routes that do not yet have a native reader continue through the normal fallback chain. Security pages are intentionally outside native GitHub handling.

## Direct markdown path

For direct markdown-style URLs, webctx checks whether a `.md` document is available. If the given URL does not end in `.md`, it tries the same URL with `.md` appended.

The HEAD response must look like markdown or plain text and have enough content length to be useful. Then webctx fetches the markdown directly and derives the title from the first `#` heading when possible.

## Firecrawl fallback

When no native/direct-markdown path handles the URL, webctx uses Firecrawl:

```text
endpoint: https://api.firecrawl.dev/v2/scrape
formats: markdown
onlyMainContent: true
skipTlsVerification: true
blockAds: true
removeBase64Images: true
maxAge: 600000
```

The request excludes common non-content tags such as scripts, styles, navigation, footers, headers, asides, SVGs, images, and ad selectors.

## PDF handling

If the URL ends in `.pdf`, webctx asks Firecrawl to use the PDF parser.

## Rate limiting

Firecrawl scrape requests pass through a process-local queue with a token bucket limiter. It starts with 10 tokens and refills one token every six seconds. The queue keeps scrape calls serialized so agent workflows do not burst into Firecrawl.
