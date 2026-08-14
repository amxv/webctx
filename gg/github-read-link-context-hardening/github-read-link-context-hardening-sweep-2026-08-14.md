# GitHub Read-Link Context Hardening Sweep

## Coordinates and scope

- **Repository:** `amxv/webctx`
- **Checkout:** `/workspace/repos/webctx`
- **Branch inspected:** `main`
- **Planning baseline:** `d541d1a24622d9b92fd1fbd87b5af5cc69370389` (`v0.2.0`)
- **Research date:** 2026-08-14
- **Mode:** read-only current-state research. This document describes the implementation and observed behavior that exist at the planning baseline; it does not prescribe the future implementation.
- **User-visible surface in scope:** `webctx read-link` behavior for GitHub URLs, especially context size, selector discoverability, URL fidelity, pagination, failure semantics, and live-provider correctness.
- **Adjacent surfaces checked at the seam:** generic `ReadLink` fallback, GitHub credential/error handling, Firecrawl package fallback, user-facing read-link/credential/troubleshooting docs, and package/release validation.
- **Deliberately outside this Sweep:** GitHub security/admin/settings pages, search-provider ranking behavior, `map-site`, unrelated docs-site styling, and implementation history not needed to explain current code.

The repository was fast-forward synchronized before research. The worktree was clean before and after the read-only inspection and live probes.

## Evidence basis

The Sweep used four evidence classes:

1. **Current source and tests** under `internal/app/`.
2. **Current user-facing docs** under `src/content/docs/`.
3. **The built `v0.2.0` executable** exercised against live public GitHub resources with the repository's `.env.local` credentials available.
4. **Current GitHub provider contracts** from GitHub's official REST/GraphQL documentation and live API responses where the exact response shape mattered.

The live test token was used only through the existing environment. Secret values were never printed or copied into artifacts.

## Architecture map

`read-link` currently enters through:

```text
cmd/webctx/main.go
  -> internal/app/app.go Run(...)
     -> internal/app/tools.go ReadLink(rawURL)
        -> internal/app/github.go readGitHubNative(rawURL)
           -> parseGitHubTarget(rawURL)
           -> 30-second native GitHub context
           -> readGitHubNativeWithClient(...)
              -> family-specific reader
        -> direct .md path when native is unsupported
        -> Firecrawl fallback when native/direct markdown are unsupported
```

`ReadLink` distinguishes three native outcomes:

- `GitHubNativeUnsupported`: the URL is not a native GitHub target, so normal direct-markdown/Firecrawl fallback may run.
- `GitHubNativeSuccess`: native output is authoritative and returned directly.
- `GitHubNativeFailure`: the native family was recognized but failed, so the error is returned rather than silently scraping unrelated GitHub UI. The one explicit exception is the existing best-effort public Package fallback for specific auth/permission failures.

This boundary is load-bearing. A recognized GitHub Issue/PR/Actions/commit URL does not quietly turn into a Firecrawl scrape on rate limits, private access failures, or decode failures.

### Native GitHub modules

Current native functionality is already decomposed by resource family:

```text
internal/app/github.go
  semantic URL parsing; shared GitHub client; repo/blob/tree reads;
  ref/path resolution; Markdown selectors; shared truncation/helpers

internal/app/github_issues.go
  Issue detail, timeline, relationships, comments, Issue lists/search,
  labels, milestones

internal/app/github_pulls.go
  PR conversation, reviews, inline review threads, comment/review selectors

internal/app/github_pull_views.go
  PR files, commits, checks, focused check selection, raw PR diff/patch

internal/app/github_actions.go
  Actions overview, workflows, runs, jobs, artifacts, logs

internal/app/github_commits.go
  commit detail, commit comments, compare, history, blame, raw diff/patch

internal/app/github_refs.go
  branches, tags, releases, release assets, forks, stargazers, watchers

internal/app/github_discussions_gists.go
  Discussions, Discussion comments/replies, Gists, Gist files/comments

internal/app/github_search_profiles.go
  GitHub Search, users/orgs, profile tabs

internal/app/github_activity_deployments.go
  repository activity/statistics, deployments/environments/statuses

internal/app/github_packages_projects.go
  Packages and Projects v2
```

There are 154 `Test...` functions across the GitHub-focused test files at the planning baseline.

## Shared GitHub client behavior

`GitHubClient` is the canonical provider boundary. It owns:

- `api.github.com` and raw-content origins;
- optional `GH_TOKEN` / `GITHUB_TOKEN` selection (`GH_TOKEN` wins);
- `User-Agent`;
- REST API version `2026-03-10`;
- response status, headers, body, final URL, and too-large state;
- typed GitHub error classification;
- `Link` header parsing;
- REST pagination;
- GraphQL requests for capability-specific reads;
- anonymous retry for selected public REST endpoints when a narrow configured token is rejected.

### `RESTPages` is complete-by-default

`GitHubClient.RESTPages` follows `rel=next` until GitHub stops returning a next link. It also detects pagination cycles.

That completeness primitive is useful for exact complete resources, but several current landing/overview readers use it even when the provider child collection is huge. This is a major source of latency and context growth in current behavior.

Examples:

- Issue timeline: every page.
- PR reviews/review comments: every page.
- PR checks: every check-run page.
- Actions run: every jobs page and every artifacts page.
- compare: every comparison commit page.
- commit detail: every file page.
- release detail: every asset page.
- Gist root: every comment page.
- deployment environment: every status page for each selected deployment.

### Native timeout

`readGitHubNative` wraps every native read in one 30-second context. A reader that performs many sequential pagination requests therefore competes with the same deadline as a narrow selector read.

A live large compare reached page 31 and then failed with `context deadline exceeded` rather than returning a bounded overview.

## Current output vocabulary

The native readers consistently use compact Markdown plus YAML-like frontmatter. This is already a strong agent-oriented convention.

Typical output shape:

```text
---
repository: "owner/repo"
<resource facts>
---

# Human title

<resource-specific sections>

## Navigation / Useful GitHub URLs
```

Selectors and child URLs are generally represented as canonical GitHub web URLs rather than custom `webctx` syntax.

## Repository root behavior

`readGitHubRepository` fetches repository metadata plus GitHub's canonical README resource.

`renderGitHubRepository` emits a lightweight frontmatter block with current facts including, when available:

- repository full name;
- description;
- language;
- default branch;
- license;
- stars;
- topics;
- fork/archive/private visibility state.

README content is stripped of invisible HTML comments and passed through `truncateMarkdownSafe` with `githubRootPreviewRunes = 5000`.

If truncated, the output provides the README's canonical `html_url` as the full source.

The root also gives native-URL hints for:

- full README source;
- source line selection;
- one-level tree listing;
- Issues;
- a PR URL shape.

### Live repository-root result

`https://github.com/vercel/next.js` produced approximately 3,953 characters / 69 lines and remained useful and bounded.

This is the strongest current example of the desired “overview first, canonical deeper URLs next” behavior.

## Blob/source behavior

Public blobs first try `raw.githubusercontent.com` because that is the cheapest faithful path and naturally handles slash-containing refs without REST ref/path disambiguation.

With a configured token, a raw 404 can fall through to authenticated ref/path resolution for private content.

### Blob selectors

Current selectors:

- `#L20`
- `#L20-L40`
- Markdown heading fragments such as `#install`
- duplicate Markdown heading fragments such as `#install-1`

Line selectors return only the selected source range and total line coordinates.

Markdown heading selection parses ATX headings outside fenced code blocks and returns the selected heading through the next heading of equal-or-higher level.

### Live source-range result

A Next.js source URL with `#L100-L140` returned about 2,299 characters / 46 lines.

### Case-sensitive copied blob URLs

A black-box probe using `.../blob/canary/README.md#getting-started` returned 404. Provider inspection showed this was not a heading-parser failure: Next.js stores the file as lowercase `readme.md`, and GitHub's own web/raw/Contents URLs for uppercase `README.md` also return 404. GitHub's `/repos/vercel/next.js/readme` endpoint returns the canonical lowercase path.

The current heading selector unit tests pass on valid file paths. The important current behavior is therefore GitHub-compatible path case sensitivity, not case-insensitive blob resolution.

## Tree behavior

Tree URLs resolve slash-containing refs against the Contents API, sort entries, emit a one-level listing, and optionally include a directory README preview using the same ~5,000-rune safe truncation helper.

GitHub's 1,000-entry Contents ceiling is surfaced truthfully as `complete: false`.

Live examples for `vercel/next.js/tree/canary/packages` and `/test` were about 2.0–2.5k characters.

A directory with close to the 1,000-item provider ceiling can still create a much larger terminal response because every returned child is currently rendered.

## Ref/path resolution

`resolveGitHubRefPath` tries provider-valid splits of the unresolved tail rather than assuming the first path segment is the ref.

This is required because branch/tag names can contain `/`.

Multiple provider-valid ref/path splits produce an explicit ambiguity error rather than silently choosing one.

This logic is load-bearing for tree/blob/history/blame correctness and should not be duplicated by later hardening work.

## Issue detail behavior

`readGitHubIssue` currently supports exact `#issuecomment-<id>` selection.

Without a fragment it:

1. fetches `/issues/{number}`;
2. rejects PRs that arrived through an Issue detail URL with a canonical PR hint;
3. fetches the entire Issue timeline through `RESTPages`;
4. fetches parent/sub-issues/dependency/field-value relationships;
5. renders the full body, relationships, pinned comment, comments, and substantive timeline events.

The current renderer deliberately drops invisible HTML comments and non-substantive noise events.

### Current Issue timeline data shape

`githubTimelineEvent` currently declares:

```text
Minimized       bool
MinimizedReason string
```

Live GitHub timeline data is polymorphic. `cli/cli#326` returned a legitimate `commented` event whose `minimized` value was an object:

```json
{
  "event": "commented",
  "id": 858596858,
  "minimized": {"reason": "spam"},
  "minimized_reason": null
}
```

This causes the entire native Issue read to fail during JSON unmarshal:

```text
decode GitHub Issue timeline: json: cannot unmarshal object into Go struct field githubTimelineEvent.minimized of type bool
```

The same failure reproduced on `cli/cli#9139`.

This is a provider-shape robustness defect, not a rate/auth problem.

### Live normal Issue result

A current Next.js Issue (`#97337`) returned about 6,112 characters / 123 lines and read cleanly as a human conversation.

Its exact comment permalink returned about 1,941 characters.

This demonstrates that full Issue-detail rendering is pleasant when the content is naturally small; the problem is unbounded expansion, not the basic human-readable format.

## Issue body identity

The REST Issue object exposes an integer `id` that maps to GitHub's canonical body fragment:

```text
/issues/<number>#issue-<issue-id>
```

Live example:

```text
vercel/next.js#97337 -> Issue id 5146379036
body fragment          #issue-5146379036
```

The current parser rejects `#issue-*`; only `#issuecomment-*` is implemented.

## Issue lists and search

Bare `/issues` uses GitHub's repository Issues REST endpoint with a default `per_page=30`, removes PR-shaped items, and emits a structured compact list plus `Previous`/`Next` web URLs derived from provider pagination.

A live bare Next.js Issues page was only about 757 characters.

Search-style `/issues?q=...` uses `/search/issues`.

### PR qualifier correctness bug

`readGitHubIssueSearch` currently appends `is:issue` whenever the query does not already contain `is:issue`, even if it already contains `is:pr`.

It then filters PR items from the returned results.

Live URL:

```text
https://github.com/vercel/next.js/issues?q=is%3Apr+is%3Aopen
```

Current native output reports:

```text
view: issues
query: "is:pr is:open"
total_matches: 2117
```

but renders actual Issues, not PRs.

This is especially dangerous because the output is structured and looks authoritative while contradicting the copied GitHub query.

## Pull Request list behavior

`parseGitHubTarget` has no `GitHubTargetPullList` and no `/pulls` case.

Therefore:

```text
https://github.com/<owner>/<repo>/pulls
```

falls through native GitHub handling and is scraped by the generic page path.

### Live `/pulls` results

Next.js:

- bare `/pulls`: ~18,587 chars / 410 lines;
- filtered `/pulls?q=...`: ~20,778 chars / 510 lines;
- `/pulls?page=2`: ~17,549 chars / 417 lines.

The output contains browser/UI artifacts including:

- `Uh oh!` loading errors;
- account-tab reload notices;
- login-only notification controls;
- loading placeholders;
- repeated Open/Closed counts;
- UI filter menus;
- ProTips;
- duplicate pagination/navigation.

This is qualitatively different from the current native `/issues` list.

GitHub's REST “List pull requests” endpoint supports state/head/base/sort/direction/per_page/page. Query-style PR searches can use Search Issues with `is:pr`.

## Pull Request detail behavior

`readGitHubPullRequest` currently recognizes:

- `#issuecomment-<id>`;
- `#discussion_r<id>`;
- `#pullrequestreview-<id>`.

The exact selectors already fetch narrowly scoped resources rather than reconstructing the whole PR.

Without a selector, PR detail currently:

1. fetches PR identity;
2. fetches the complete Issue-style timeline;
3. fetches all reviews;
4. fetches all inline review comments;
5. groups review comments into threads;
6. optionally enriches thread resolved/outdated state through GraphQL;
7. renders the full body, full conversation comments/events, all review bodies, all thread bodies/replies, and navigation to files/commits/checks.

### Live PR context growth

`vercel/next.js#97343`:

- total: ~46,331 chars / 998 lines;
- metadata/header: ~431 chars;
- body: ~73 chars;
- **Conversation: ~44,773 chars**;
- Reviews: ~37 chars;
- Threads: ~55 chars;
- navigation: ~217 chars.

Two automation comments containing CI/test and statistics reports dominate the response.

`cli/cli#13250`:

- total: ~21,868 chars / 405 lines;
- conversation: ~2,603 chars;
- **Reviews: ~7,629 chars**;
- **Inline review threads: ~10,463 chars**.

A body-heavy Next.js PR (`#96552`) returned about 31,455 chars. Its current rendered Body section was ~7,713 chars and Conversation ~22,485 chars.

These examples prove independent expansion risks in PR body, ordinary conversation, review bodies, and review threads.

### Exact PR selectors are already excellent

Live scoped results:

- `#issuecomment-...`: ~718 chars for a selected PR conversation comment;
- `#pullrequestreview-...`: ~2,453 chars for a selected review and its review comments;
- `#discussion_r...`: ~1,026 chars for one review thread.

The selector implementation is already the strongest escape hatch from large PR context.

### Current selector discoverability

Review renderers include review IDs and `#pullrequestreview-*` URLs.

Review-comment/thread metadata include `#discussion_r*` URLs.

Ordinary conversation comments are headed as `Comment by @user — timestamp`; the root output does not consistently expose the comment ID/canonical `#issuecomment-*` URL in an index-friendly form.

The current root navigation only points to `/files`, `/commits`, and `/checks`.

## PR description body identity

A PR also has an Issue-side REST representation. The Issue-side `id` matches GitHub's canonical `#issue-<id>` body fragment.

Live proof:

```text
GET /repos/vercel/next.js/issues/96552
id: 5053699136
GitHub body fragment: #issue-5053699136
```

The current PR parser rejects `#issue-*`.

This is a first-party structured way to construct a canonical “selected PR description” URL without scraping GitHub HTML.

## PR Files view

`/pull/<n>/files` fetches all provider pages up to GitHub's 3,000-file ceiling and renders each file including its patch when provided.

A GitHub diff fragment is parsed as a SHA-256-of-path selector with optional left/right line coordinates. A selected file/hunk returns a narrow patch.

Live Next.js PR #97343 Files output was about 3,469 characters for two changed files and was clean/useful.

For genuinely large PRs, the root `/files` view can still be enormous because it renders every returned patch. The explicit diff-file/hunk selector already provides a better narrow path once discovered.

## PR Commits view

`/pull/<n>/commits` returns a compact commit list after complete pagination.

Next.js PR #97343 produced only ~535 chars for one commit.

This surface is currently well-shaped; only very large PR commit lists could approach the same list-size concerns as other 30/100-row pages.

## PR Checks view

Without `check_run_id`, `readGitHubPullChecks`:

- fetches every check-run page;
- fetches combined commit status pages;
- sorts all runs by name/id;
- renders every check run;
- includes each check's output title and full `Output.Summary` when present;
- emits a focused `?check_run_id=` URL only when the check has annotations.

### Live Checks result

Next.js PR #97343 had 132 check runs:

- total output: ~30,536 chars / 870 lines;
- check-run section: ~29,444 chars;
- status section: ~241 chars.

The focused failing check:

```text
/pull/97343/checks?check_run_id=94639361056
```

returned only ~692 chars.

The provider already gives status/conclusion, app, annotations count, Details URL, and check ID sufficient for a compact index/rollup.

### Focused check behavior

A selected check fetches the check identity plus **all** annotation pages and renders:

- check summary;
- output title/summary;
- every annotation message/raw-details line.

A single machine-generated check can therefore still create a large selected response.

## Actions overview and workflow views

`/actions` combines:

- up to 30 workflows;
- up to 30 recent runs;
- run pagination.

Live Next.js `/actions` was ~9,921 chars / 80 lines.

A selected workflow page with 30 runs was ~5,774 chars / 49 lines.

These are structurally clean list outputs but the root combines two independent lists and can exceed the repository-root-style budget.

## Actions run behavior

A run detail currently fetches:

- run identity;
- **every jobs page**;
- **every artifacts page**;
- no job logs.

It then renders every job link/status and every artifact name/size/expiry/archive API URL.

### Live large run

Next.js run `31757053478`:

- 102 jobs;
- 133 artifacts;
- total output ~40,056 chars / 261 lines;
- Jobs section ~14,375 chars;
- Artifacts section ~24,091 chars.

The run is a landing/container URL but current behavior expands every child identity.

## Actions job behavior

An Actions job URL fetches:

1. the selected job identity and structured steps;
2. `/actions/jobs/{job_id}/logs`;
3. the redirected log body/archive;
4. up to the global 100 MB GitHub body limit;
5. the entire decoded log into output inside a Markdown code fence.

GitHub's official endpoint is a 302 redirect to a plain-text log URL that expires after one minute. Public repositories may use it without auth; fine-grained tokens require Actions read permission for private/authenticated access.

### Live job output variance

- one failing job: ~3,447 chars;
- successful job `94635412218`: **~216,981 chars / 3,162 lines**;
- build job `94635017814`: **~152,657 chars / 2,213 lines**.

A semantically specific job URL is therefore not predictably safe for an agent context window.

The job's structured step list already identifies failed/successful steps independently of the raw log.

## Commit detail behavior

Commit detail currently:

- follows every commit-response file page;
- accumulates up to GitHub's documented 3,000-file ceiling;
- fetches all commit comment pages;
- renders the full commit message;
- renders **every changed file with patch content** using the same file renderer as PR Files;
- renders full commit comment bodies.

### Live commit result

A Next.js release commit returned ~17,196 chars / 545 lines, mostly from changed-file patches.

Raw commit `.diff` and `.patch` URLs already exist as explicit native resources.

### Commit comment selectors

GitHub commit comments expose canonical HTML URLs of the form:

```text
/commit/<sha>#commitcomment-<id>
```

GitHub REST also exposes `GET /repos/{owner}/{repo}/comments/{comment_id}` for one comment.

The current commit reader rejects all fragments, so the exact selector path is not yet used.

## Compare behavior

Plain compare currently calls:

```text
RESTPages(/compare/{base}...{head}?per_page=100)
```

It:

- follows **every commit page**;
- keeps changed files from the first page (GitHub exposes compare files only there, up to 300);
- renders every commit;
- renders every changed file including full patch text.

### Live compare results

A single canary-version comparison:

```text
v16.3.1-canary.15...v16.3.1-canary.16
```

returned approximately **868,699 chars / 20,888 lines**.

A much larger `v15.5.0...canary` comparison chased pages until page 31 and failed at the 30-second native context deadline.

### Raw compare escape hatches

Current native URLs already support:

```text
/compare/base...head.diff
/compare/base...head.patch
```

The tested raw representations were approximately 1.92 MB and 2.03 MB respectively. Their size is deliberate because the URL explicitly requests the raw representation.

GitHub documents that paginated compare responses paginate commits while changed files are only present on the first page, capped at 300 files.

## Commit history and blame

History uses a stable `page=` web URL and a default 30-row page. A Next.js path history page was ~6.4k characters.

Blame uses structured GraphQL ranges and requires auth.

These are already bounded list/selector-style surfaces.

## Release list behavior

`/releases` is a compact 30-row list and does not expand release bodies. Next.js produced ~3,872 chars.

This is another good current example of “index first, detail later.”

## Release detail behavior

Release detail currently:

- emits the full release note body;
- follows every release-asset page;
- renders every asset with metadata/download URL.

A live Next.js `v15.5.0` release returned ~29,719 chars / 486 lines.

`TestReleaseDetailPreservesLongBodyAndPaginatesAssets` explicitly requires a 500-line body to remain present and all asset pages to be fetched. This test accurately protects the current behavior but encodes an output contract that is no longer context-safe for agents.

There is no currently implemented release-note fragment selector analogous to Issue/PR comments. Asset browser-download URLs are already exact deeper resources.

## Branches/tags/forks/social lists

These list views use `boundedListQuery` with `per_page=30` and stable `page=` navigation.

Live outputs:

- branches: ~5.3k;
- tags: ~3.1k;
- forks: ~3.2k.

Stargazers/watchers use `RESTPublic`, meaning a rejected narrow token is retried anonymously for public reads.

Live GitHub currently returned authentication-required responses anonymously for the tested Next.js stargazer/subscriber endpoints, so the resulting auth/permission error is provider truth rather than a missed anonymous retry.

## Discussions list behavior

Discussions require GraphQL authentication.

The list intentionally fetches only the first 30 updated Discussions and does not pretend a GraphQL cursor is a stable copied GitHub page URL.

A live Next.js list was ~7.1k and truthfully noted that more Discussions exist upstream.

## Discussion detail behavior

Discussion detail currently:

- paginates **all** top-level comments through GraphQL;
- paginates **all** replies for each comment;
- renders the full Discussion body;
- renders every comment/reply body.

A live Next.js Discussion returned ~16,942 chars / 360 lines.

Canonical comment/reply URLs include `#discussioncomment-<number>` fragments.

GitHub GraphQL `DiscussionComment` exposes both the canonical `url` and an integer `databaseId`, so the current provider can represent those selectors without inventing a custom flag.

Current `readGitHubDiscussion` rejects all fragments.

## Gist behavior

A Gist root currently:

- fetches Gist metadata/files;
- follows every Gist comment page;
- fetches full raw content for API-truncated files where possible;
- renders every file's full content;
- renders every Gist comment body;
- renders revision history.

Existing Gist file selectors already support exact file and line-range URLs based on GitHub's `#file-...` fragment shape.

GitHub REST exposes `GET /gists/{gist_id}/comments/{comment_id}` for an exact Gist comment, but the current root does not expose/use a Gist-comment selector path.

Large multi-file Gists can therefore expand multiple independent full sources into one root response even though exact file selectors already exist.

## Search/profile/package/project behavior

GitHub Search and user/org profile tabs generally return 30-row bounded pages with stable page navigation where the UI URL supports it.

Live profile/org root and tab samples were roughly 0.3–2.5k.

Projects v2 deliberately return only the first 50 items and truthfully note `More Project items exist upstream`; item bodies are not expanded. This is already aligned with bounded-container semantics.

Packages use 30-version pages and have a narrowly scoped best-effort Firecrawl fallback for public Package pages when the structured API rejects specific auth/permission states.

These surfaces are not current context-bomb priorities.

## Repository statistics

Contributor statistics fetch GitHub's whole statistics array and render every contributor.

Live Next.js contributors returned ~18,616 chars / 509 lines for 499 contributors.

The current renderer preserves provider order and computes additions/deletions across all weeks per contributor.

Commit-activity/code-frequency statistics produce fixed weekly datasets and are much smaller.

GitHub may return 202 while statistics are being computed; current code truthfully renders a retry-later state.

## Deployment environment fan-out

A deployment environment page intentionally limits deployments to 10 per page, but for each deployment it calls `RESTPages` for **all** deployment statuses and renders every returned status.

This is a bounded parent multiplied by an unbounded child connection. It has the same structural fan-out shape as other identified context risks, although the live stress suite did not find a pathological public example.

## Shared truncation primitive

`truncateMarkdownSafe(markdown, maxRunes)` already exists and:

- counts Unicode runes rather than bytes;
- prefers safe line/paragraph boundaries;
- tracks fenced code blocks;
- avoids invalid UTF-8 cuts;
- reports whether truncation occurred.

It is currently used for repository and directory README previews only.

There is no shared concept of:

- overview/container output budget;
- child index item preview;
- omitted-child count;
- exact-selector hint generation;
- machine-generated text preview;
- raw/full escape-hatch link.

Each family renderer currently decides independently how much provider text to append.

## Exact selector paths already proven useful

The following existing native selectors produce compact, high-value output:

```text
/blob/<ref>/<path>#L20-L40
/blob/<ref>/<path>#heading
/pull/<n>#issuecomment-<id>
/pull/<n>#pullrequestreview-<id>
/pull/<n>#discussion_r<id>
/pull/<n>/files#diff-<sha256(path)>...
/pull/<n>/checks?check_run_id=<id>
/gist...#file-...-Lx-Ly
```

Raw explicit representations already exist for:

```text
/pull/<n>.diff
/pull/<n>.patch
/commit/<sha>.diff
/commit/<sha>.patch
/compare/base...head.diff
/compare/base...head.patch
raw/blob URLs
```

These URLs are a core current strength. The main gap is that several landing pages do not expose enough of them before expanding child content.

## Current tests and what they prove

At the baseline:

- `go test ./...` passes;
- `go vet ./...` passes;
- `npm test` passes;
- the current binary builds normally again (the prior machine I/O slowdown was not present during this run).

The tests provide strong coverage for:

- semantic URL classification;
- slash-containing ref preservation;
- GitHub REST version/auth headers;
- anonymous public retry behavior;
- pagination and cycle rejection;
- repository README preview truncation;
- source line/Markdown-heading selectors;
- Issue complete timeline/relationships;
- exact Issue comment selection;
- PR comment/review/thread selection;
- PR GraphQL thread-state enrichment;
- PR diff selectors;
- focused check selection/annotations;
- Actions run/jobs/artifacts pagination;
- selected Actions job/log retrieval and redirect-token safety;
- commit/compare pagination/provider ceilings;
- raw diff/patch media types;
- release list/detail/assets;
- Discussion/Gist pagination;
- long-tail list/query validation.

### Tests that currently encode unbounded output

Several tests protect completeness assumptions that conflict with the newly observed agent-context problem:

- PR conversation test expects full body, bot comment bodies, full review bodies, and full inline-thread bodies in the root PR.
- Actions run test expects all paginated jobs and all paginated artifacts to appear.
- Actions job test expects the selected job's complete log text to be emitted.
- compare test expects all commit pages and changed-file patches in the plain compare output.
- release detail test explicitly expects a very long release body to remain untruncated and all asset pages to be expanded.

These are not accidental bad tests; they accurately describe the current product contract. Any bounded-output change must deliberately update the observable contract and replace these assertions with boundedness + navigation/selectability evidence, rather than merely deleting coverage.

## Current user-facing docs

`src/content/docs/read-link.md` accurately documents the current mental model:

```text
supported structured URL -> native/direct provider read
clean markdown available -> direct markdown
otherwise                -> Firecrawl
```

It teaches:

- repository overview;
- tree/blob/line/heading reads;
- Issue conversation and `#issuecomment-*`;
- PR root/review-thread/files/commits/checks;
- Actions run then selected job;
- commit/compare/history/blame;
- releases/activity/deployments/Discussions/Gists/search/profiles/projects;
- optional GitHub auth.

The current Actions section says the selected job returns its substantive log. The compare/Issue/PR sections imply detail URLs can return full relevant content. These docs must be reconciled with any new bounded-container contract.

`credentials.md` and `troubleshooting.md` already explain optional GitHub auth, fine-grained-token limitations, anonymous public retry, rate-limit truth, and unavailable Actions logs.

## Relevant history

The current native GitHub implementation landed as a sequence of resource-family additions on 2026-08-13, including:

- GitHub read-link foundation;
- native Issues;
- PR conversations;
- PR focused views;
- commits/history;
- Actions;
- refs/releases;
- Discussions/Gists;
- later live-auth reconciliation.

The current source is internally consistent around one semantic parser/client/renderer architecture; there is no competing older native GitHub subsystem to migrate away from.

## Landmines

### 1. Completeness and context safety are currently coupled

Many readers achieve “complete” by fetching every page and then rendering every child body. Replacing only the renderer can still leave severe latency/API amplification. Replacing only pagination can silently lose provider truth. The two concerns must be separated deliberately.

### 2. `RESTPages` is a powerful primitive, not a universal default

It should remain for exact complete resources that need provider pagination. Container/overview readers using it blindly can hit the shared 30-second timeout or do dozens of unnecessary calls before emitting any useful context.

### 3. Exact selectors must stay exact

`#issuecomment-*`, `#pullrequestreview-*`, `#discussion_r*`, `check_run_id`, diff selectors, source line selectors, and Gist file selectors already avoid unrelated reads. Hardening must not route them back through a full parent fetch.

### 4. Raw representations must remain explicit escape hatches

`.diff`, `.patch`, raw/blob and provider log-download URLs can be huge. Their size is intentional because the URL itself asks for raw bulk content. A global hard cap applied indiscriminately would destroy this useful distinction.

### 5. A specific semantic URL can still be unsafe

Actions jobs prove that “specific” does not automatically mean “small”: a single selected job produced >200k chars because the root representation includes its whole machine log. Machine-generated subordinate text needs a separate boundedness rule from human atomic comments.

### 6. GitHub timeline JSON is polymorphic

The live `minimized` object proves that strict static assumptions about optional timeline enrichment can take down an otherwise readable Issue. Future unknown timeline fields/events must not cause the primary body/comments to become unavailable.

### 7. PRs have two database identities

The PR REST object and Issue-side REST object have distinct IDs. GitHub's `#issue-<id>` description permalink uses the Issue-side ID. Building the selector from the PR REST ID would be wrong.

### 8. `/issues?q=is:pr...` is a real GitHub URL form

PR search links are not confined to `/pulls`. A native `/pulls` implementation that leaves Issue-search PR qualifiers misclassified would still produce incorrect results from common copied GitHub links.

### 9. Query vocabulary is part of URL semantics

The current native readers intentionally allow only understood query parameters. PR-list/search hardening must preserve GitHub filter/sort intent it can map and reject unsupported semantics rather than silently dropping filters.

### 10. Review-thread state is optional GraphQL enrichment

REST contains the substantive thread content. GraphQL adds resolved/outdated state. Failure of that enrichment must remain non-fatal for the selected PR/thread content.

### 11. Discussion cursors are not stable copied web URLs

The current Discussion list deliberately does not invent a cursor navigation URL. Bounded Discussion detail cannot pretend GraphQL cursors are user-facing selectors either.

### 12. Provider list ceilings differ by family

GitHub has resource-specific ceilings (e.g. Contents directory 1,000, compare files 300, PR/commit changed files up to 3,000). “Omitted because webctx budgeted output” and “missing because provider did not expose more” must remain distinguishable.

### 13. Current tests intentionally protect old completeness

Changing output without replacing those tests with explicit new acceptance assertions risks either accidental regression or a misleading green suite.

### 14. Firecrawl fallback must stay narrow

Recognized GitHub native failures are intentionally authoritative. Output hardening must not turn decode/pagination/provider errors into silent scraped GitHub UI, recreating the exact `/pulls` noise problem through a different path.

### 15. Security pages remain outside native scope

The target parser deliberately leaves `security` and admin/settings namespaces unsupported. This workstream should not widen into credential/alert/security rendering.

### 16. Source path case follows GitHub

The live uppercase Next.js README URL was itself invalid on github.com. A “fix” that makes arbitrary blob paths case-insensitive would diverge from GitHub URL semantics and can be ambiguous on case-sensitive trees.

### 17. Statistics ordering can be low-signal

Contributor statistics currently render all provider rows. Even if output is capped, selecting the first rows without a deterministic useful ordering can hide the highest-contribution entries. Bounded statistical summaries need an explicit ordering rule.

### 18. Deployment status expansion is an N+1 seam

Ten deployments can each cause multi-page status retrieval. The parent page is bounded while subordinate provider work is not.

## Existing patterns worth copying

1. **Repository root preview + full-source link.** The best current bounded-overview pattern.
2. **`truncateMarkdownSafe`.** Existing UTF-8/Markdown-aware preview helper.
3. **Exact PR selectors.** Narrow provider calls with canonical GitHub fragment URLs.
4. **Focused check query.** `?check_run_id=` proves query semantics can select one child without new flags.
5. **Release list / Issue list.** Compact structured lists with provider-derived web pagination.
6. **Project v2 first-page truthfulness.** Explicitly states more upstream content exists instead of expanding it all.
7. **Provider ceilings in frontmatter/warnings.** Existing complete/incomplete vocabulary.
8. **`RESTPublic`.** Token fallback stays centralized rather than resource-specific retry hacks.
9. **Native failure authority.** Prevents a structured failure from silently becoming noisy scraped HTML.

## Black-box stress matrix

Representative live measurements at the planning baseline:

| URL family | Example result | Current assessment |
| --- | ---: | --- |
| repository root | ~3.95k chars | good bounded overview |
| bare Issues | ~0.76k | excellent list |
| Issue detail (small) | ~6.1k | good human-readable detail |
| old Issue detail | hard decode failure | correctness defect |
| `/pulls` | ~18.6k scraped UI | missing native list |
| filtered `/pulls` | ~20.8k scraped UI | missing native search/list |
| `/issues?q=is:pr...` | structured but wrong resource type | correctness defect |
| PR root, bot-heavy | ~46.3k | context bomb |
| PR root, review-heavy | ~21.9k | context bomb |
| selected PR comment | ~0.72k | excellent selector |
| selected PR review | ~2.45k | excellent selector |
| selected review thread | ~1.03k | excellent selector |
| PR files (2 files) | ~3.47k | good |
| PR commits (1 commit) | ~0.54k | good |
| PR checks (132 runs) | ~30.5k | context bomb |
| selected check | ~0.69k | excellent selector |
| Actions root | ~9.9k | structurally clean, large |
| workflow | ~5.8k | acceptable/borderline |
| Actions run | ~40.1k | context bomb |
| selected failing job | ~3.45k | good by chance |
| selected successful job | ~217k | severe context bomb |
| compare, adjacent canary tags | ~869k | severe context bomb |
| large compare | 30s timeout | latency/completeness failure |
| commit detail | ~17.2k | patch expansion |
| release detail | ~29.7k | body/assets expansion |
| releases list | ~3.9k | good |
| Discussion detail | ~16.9k | conversation expansion |
| Discussions list | ~7.1k | bounded but large |
| contributor statistics | ~18.6k | full fan-out |
| tree examples | ~2–2.5k | good |
| source line selector | ~2.3k | excellent selector |
| profile/org tabs | ~0.3–2.5k | good |

These measurements are workload examples, not universal thresholds. They establish the defect class: current output size is sometimes proportional to total subordinate provider content rather than to the semantic role of the copied URL.

## Coverage statement

Mapped deeply:

- native routing and provider client behavior;
- repo/blob/tree shared output/truncation/selectors;
- Issue detail/list/search and live timeline schema failure;
- PR root/selectors/subviews;
- checks/focused checks;
- Actions overview/run/job/log behavior;
- commit/compare/history boundaries;
- releases/assets;
- Discussions/Gists;
- statistics/deployment fan-out;
- tests that encode current completeness;
- user-facing read-link/auth/troubleshooting docs;
- live output size/latency across representative large public resources.

Checked at the seam:

- Search/profile/package/project readers because their current list/item shapes are already bounded;
- generic Firecrawl fallback to ensure native failures remain authoritative;
- current release/package validation.

Deliberately not mapped deeply:

- GitHub security/admin/settings surfaces;
- write APIs;
- search-provider internals unrelated to GitHub native reads;
- docs-site presentation internals.

## Factual gaps / live proof still needed

1. **Discussion exact-comment lookup cost.** GitHub GraphQL exposes `DiscussionComment.databaseId` and canonical URL, but selecting by copied numeric fragment may require paging the containing Discussion's comment connection until the matching database ID is found; implementation should prove the cheapest truthful query available at execution time.
2. **Actions raw-log link presentation.** The REST log endpoint returns a short-lived redirect URL. The implementation must decide whether to print the expiring final URL, print the stable GitHub API log endpoint, or both, while ensuring the token is never embedded/leaked and the UX remains useful after the one-minute redirect expires.
3. **Release-note full-body escape hatch.** No stable GitHub fragment equivalent to `#issue-*` was established for release notes. The current release page itself is the canonical human URL; the bounded default must be truthful about this limitation rather than inventing a synthetic selector.
4. **Very large Gist and deployment-environment examples.** The code clearly contains fan-out potential, but no public fixture as pathological as the PR/Actions/compare cases was needed to establish the architectural risk. Phase-specific tests should synthesize deterministic large states even if live fixtures remain small.
5. **Provider evolution.** GitHub API schemas/permissions can change. Flexible decoding and live compatibility tests must distinguish optional enrichment drift from core-resource failure.
