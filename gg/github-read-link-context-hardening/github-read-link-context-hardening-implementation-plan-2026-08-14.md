# GitHub Read-Link Context Hardening Implementation Plan

## Planning Basis

- **Repository:** `amxv/webctx`
- **Checkout:** `/workspace/repos/webctx`
- **Branch inspected:** `main`
- **Planning date:** 2026-08-14
- **Planning coordinate:** `d541d1a24622d9b92fd1fbd87b5af5cc69370389` (`v0.2.0`). This SHA is provenance only; implementation agents must treat current remote `main` as truth.
- **Current-state research:** [GitHub Read-Link Context Hardening Sweep](./github-read-link-context-hardening-sweep-2026-08-14.md)
- **Specification basis:** the complete product discussion for this workstream plus the current source, tests, docs, live `v0.2.0` stress results, and current GitHub provider contracts.
- **No product implementation occurred during planning.**

### Authoritative provider references

Use GitHub's current first-party documentation for unstable API facts. Relevant planning-time references include:

- REST API versioning: `https://docs.github.com/en/rest/about-the-rest-api/api-versions`
- Issues/timeline: `https://docs.github.com/en/rest/issues/timeline`
- Pull Requests: `https://docs.github.com/en/rest/pulls/pulls`
- Workflow jobs/logs: `https://docs.github.com/en/rest/actions/workflow-jobs`
- Workflow runs/logs: `https://docs.github.com/en/rest/actions/workflow-runs`
- Commits/compare: `https://docs.github.com/en/rest/commits/commits`
- Commit comments: `https://docs.github.com/en/rest/commits/comments`
- Gist comments: `https://docs.github.com/en/rest/gists/comments`
- Discussions GraphQL guide: `https://docs.github.com/en/graphql/guides/using-the-graphql-api-for-discussions`

Current source already pins REST requests to `2026-03-10`; do not loosen that versioning behavior while implementing this workstream.

### Live acceptance fixtures established during planning

These public URLs exposed the current defect classes and should remain useful live probes when GitHub still serves them:

```text
https://github.com/vercel/next.js
https://github.com/vercel/next.js/issues
https://github.com/vercel/next.js/pulls
https://github.com/vercel/next.js/issues?q=is%3Apr+is%3Aopen
https://github.com/vercel/next.js/pull/97343
https://github.com/cli/cli/pull/13250
https://github.com/cli/cli/pull/13250#discussion_r3118513169
https://github.com/cli/cli/issues/326
https://github.com/vercel/next.js/pull/97343/checks
https://github.com/vercel/next.js/pull/97343/checks?check_run_id=94639361056
https://github.com/vercel/next.js/actions/runs/31757053478
https://github.com/vercel/next.js/actions/runs/31757053478/job/94635412218
https://github.com/vercel/next.js/compare/v16.3.1-canary.15...v16.3.1-canary.16
https://github.com/vercel/next.js/commit/c77d3f45a5b99f554d37be15cc12b96e269b4326
https://github.com/vercel/next.js/releases/tag/v15.5.0
https://github.com/vercel/next.js/discussions/96973
https://github.com/vercel/next.js/graphs/contributors
```

Live public state is mutable. Deterministic tests remain required even when a fixture changes or disappears.

## State of Current System

`read-link` already has a mature native GitHub subsystem rather than a generic crawler special case. The semantic parser recognizes repository/source/Issue/PR/Actions/commit/release/Discussion/Gist/search/profile/activity/deployment/package/project families; one shared GitHub client owns auth/version/error/pagination behavior; family-specific modules render compact Markdown/frontmatter; exact selectors already narrow many PR/source/check/Gist reads.

The detailed current-state map is in the Sweep. The load-bearing current architecture is:

```text
ReadLink
  -> semantic GitHub target parser
  -> one native GitHub client
  -> family-specific reader
  -> deterministic Markdown/frontmatter
  -> authoritative native failure
  -> generic markdown/Firecrawl only when the URL is genuinely unsupported
```

The current defect class is not lack of GitHub integration. It is that some native **landing/container URLs still equate completeness with expanding every subordinate body/patch/log**, while nearby exact selectors already prove a much more context-efficient model.

Representative current failures:

- `/pulls` is not native and falls back to noisy GitHub UI scraping.
- `/issues?q=is:pr...` reports a PR query but returns Issues.
- PR roots can emit 20–46k+ characters because they expand all comments/reviews/threads.
- Checks can emit 30k+ because they expand every check summary.
- Actions runs can emit 40k+ by listing every job/artifact.
- one Actions job emitted >216k because the full raw log is printed.
- one ordinary compare emitted ~869k and a larger compare timed out while paging commits.
- commit/release/Discussion/statistics detail views can emit 17–30k+.
- old real Issues can fail entirely on a polymorphic timeline `minimized` field.

Current exact selectors such as `#issuecomment-*`, `#pullrequestreview-*`, `#discussion_r*`, `?check_run_id=`, source line ranges, and PR diff selectors already return excellent narrow results. This plan extends that successful pattern rather than replacing it.

## State of Ideal System

After all phases land, `webctx read-link` treats a GitHub URL as both a **resource identity** and an **intent signal**.

### 1. Three output classes

Every supported GitHub URL belongs to one of three behavioral classes.

#### A. Overview / container

Examples:

```text
repository root
Issue root when its natural conversation is large
PR root
/pulls
/checks
/actions
Actions workflow
Actions run
commit root
compare root
release detail
Discussion root
multi-file Gist root
large tree/statistics/deployment environment
```

The default overview target is **roughly 5,000 Unicode characters/runes**, using safe Markdown boundaries rather than byte slicing. A renderer may modestly cross the target only to finish the current safe block and include mandatory truth/navigation. Output size does not grow linearly with total subordinate provider bodies.

An overview prioritizes:

1. authoritative metadata/frontmatter;
2. a bounded primary-body preview where the resource has one;
3. compact rollups/counts/state;
4. compact child indexes with stable identities;
5. exact canonical GitHub URLs for deeper reads;
6. truthful omitted/incomplete/provider-limit notes.

#### B. Exact semantic selector

Examples:

```text
#issue-<id>
#issuecomment-<id>
#pullrequestreview-<id>
#discussion_r<id>
#commitcomment-<id>
#discussioncomment-<id>
Gist comment selector when GitHub supplies one
?check_run_id=<id>
#diff-... selected file/hunk
#L20-L40
#markdown-heading
Gist selected file/range
```

These URLs intentionally select one semantic item. They return that selected human/content object faithfully without expanding unrelated siblings. Existing exact-selector efficiency is preserved.

A selected machine-generated object may still preview subordinate bulk text when GitHub supplies a separate raw/details resource; selecting a check or Actions job is not implicit permission to dump hundreds of kilobytes of logs/summaries.

#### C. Explicit raw / bulk resource

Examples:

```text
blob/raw file URL
/pull/<n>.diff
/pull/<n>.patch
/commit/<sha>.diff
/commit/<sha>.patch
/compare/base...head.diff
/compare/base...head.patch
GitHub's raw Actions log download resource
release asset download URL
```

The URL itself explicitly requests bulk bytes. These paths are not forced through the overview budget merely to make all output sizes uniform. Existing provider safety ceilings still apply.

### 2. One shared bounded-context vocabulary

Cross-family concepts use one consistent representation:

- **preview:** deterministic source text shortened only for the overview;
- **indexed item:** kind/identity + author/actor when meaningful + timestamp + state/coordinate + short preview + exact canonical URL;
- **reported:** provider-reported total where available;
- **returned/indexed:** what this overview actually emitted;
- **more available / omitted:** explicit when the overview did not enumerate everything;
- **provider incomplete:** distinct from local output budgeting;
- **raw/details URL:** canonical deeper provider URL when one exists.

Do not call a locally budgeted index “complete” merely because the provider response itself was complete.

### 3. URL-native navigation, not flags

No `--full`, `--limit`, `--comments`, `--logs`, or similar GitHub-specific CLI flags are added.

The navigation mechanism remains copied GitHub URLs and their real query/fragment semantics. Hints teach only capabilities that materially avoid unnecessary provider/output work. They do not teach `grep`, `sed`, `awk`, `head`, `tail`, or other transformations an agent can already apply after fetching.

### 4. Predictable PR landing experience

A PR root is always a compact structured overview rather than a complete transcript dump.

It contains:

- PR state/identity/branch/stats metadata;
- bounded description preview;
- full-description `#issue-<Issue-side database id>` URL;
- compact substantive event/comment index;
- compact review index;
- compact inline-thread index;
- child counts/omission notes;
- `/files`, `/commits`, `/checks` URLs.

Each indexed ordinary comment/review/thread includes its author, timestamp, stable ID, short preview, and exact selector URL when GitHub exposes them.

### 5. Adaptive Issue landing experience

Small Issues preserve the pleasant current human-readable body/conversation when the complete rendered result fits the overview budget.

Large Issues switch deterministically to a bounded overview/index containing body preview, pinned/relationship facts, compact timeline/comment entries, omitted counts/notes, and exact `#issue-*` / `#issuecomment-*` links.

The decision is based on deterministic rendered size/content, not author importance, bot heuristics, or model summarization.

### 6. Provider schema drift cannot erase the primary resource

Optional timeline/enrichment fields use tolerant decoding where GitHub's schema is polymorphic or evolves. A new/minimized metadata shape does not make an otherwise readable Issue/PR body unavailable.

Unknown substantive events may be omitted or rendered generically only when their stable fields are understood; decode failure of an optional enrichment is not allowed to corrupt invented facts.

### 7. Machine-generated fan-out is summarized before bodies

Checks/Actions/deployments/statistics render state rollups and child indexes before any verbose generated text.

Failures, in-progress/queued states, and other non-success states are ordered ahead of routine successful children when the overview budget cannot include every child. This is a deterministic state priority, not semantic/model summarization.

### 8. Existing direct/raw GitHub behavior stays intact

Source blobs/line ranges, raw diff/patch media, auth/error semantics, slash-ref resolution, provider ceilings, token non-leak guarantees, Firecrawl's narrowly scoped Package fallback, and non-GitHub read-link behavior remain load-bearing.

## Decisions and Assumptions

### Decision 1. Default overview budget is a soft ~5,000-rune target, not an arbitrary token counter

**Reversible.**

- **A. Reuse rune/Markdown-safe budgeting around ~5,000 characters. — Recommended**
- **B. Add a tokenizer/model-specific token budget.** Adds dependencies/model coupling for little product benefit.
- **C. Hard byte cut at exactly 5,000.** Breaks Markdown/UTF-8 and mandatory navigation.

**Why A:** the repository already has `truncateMarkdownSafe` and the user explicitly accepts a rough character budget. The goal is context safety/predictability, not model-specific billing precision.

**Selected:** A.

### Decision 2. PR roots are always structured overviews; Issue roots are adaptive

**Reversible, public-output change.**

- **A. Always summarize/index PR roots; keep small Issues fully human-readable and switch large Issues to overview mode. — Recommended**
- **B. Only truncate when either root exceeds budget.** Produces less predictable PR structure and makes selector discovery conditional.
- **C. Always index both Issues and PRs.** Needlessly degrades the excellent small-Issue reading experience.

**Why A:** PRs have multiple independent child surfaces (conversation, reviews, inline threads, files, commits, checks) and benefit from a stable map. Issues have one primary conversation and are currently pleasant at modest size.

**Selected:** A.

### Decision 3. No new GitHub-specific CLI flags

**Public-contract one-way door for this workstream.**

Navigation depth comes from canonical GitHub URL paths, queries, fragments, raw resources, and copied child links. No `--full`, `--logs`, `--page-size`, or custom selector syntax is introduced.

**Selected:** URL semantics only.

### Decision 4. Exact human-content selectors remain faithful, even when the selected body itself is long

**Reversible.**

- **A. Full selected atomic human object; no siblings. — Recommended**
- **B. Apply the overview budget to the selected comment/review/body too.** Leaves no native deeper URL for some human objects and defeats explicit opt-in selection.

**Why A:** the user explicitly opted into that exact comment/review/body URL. The context-bomb defect is accidental expansion of siblings/subordinate machine data, not a deliberate exact text selection.

**Selected:** A, subject to existing provider/network safety ceilings.

### Decision 5. Selected machine-generated resources can still be bounded

**Reversible.**

A selected check or Actions job returns its identity/steps/annotations/summary in a bounded representation when subordinate generated text is large. GitHub's Details/raw-log endpoints are the deeper opt-in resources.

**Selected:** bounded semantic machine detail + explicit raw/details link.

### Decision 6. Container reads stop provider pagination when completeness is not needed for the overview contract

**Architecture-shaping.**

- **A. Fetch only the metadata/pages required for truthful rollups and the bounded child index. — Recommended**
- **B. Always fetch every provider page, then truncate rendering.** Fixes context size but preserves avoidable 30-second timeouts/API amplification.

**Why A:** the compare failure proves network completeness itself can be harmful. The output must distinguish `provider_more_available` from a provider ceiling and from locally omitted children.

**Selected:** A.

Exact selectors and explicitly complete bounded lists may continue using `RESTPages` where completeness is part of the selected resource contract.

### Decision 7. PR/Issue body selectors use GitHub's real `#issue-<Issue-side id>` identity

**Stable URL-contract choice.**

The Issue REST representation supplies the database ID used by GitHub's canonical body fragment, including for PRs. The PR REST `id` must not be substituted.

**Selected:** `#issue-<Issue-side id>`.

### Decision 8. PR list supports both `/pulls` and PR-qualified `/issues?q=...`

**Public behavior change.**

A copied GitHub PR search must remain a PR search even when GitHub's UI URL uses the `/issues` route. Token-aware qualifier parsing must not append `is:issue` to an explicit PR query.

Conflicting explicit `is:issue` and `is:pr` qualifiers are preserved/truthfully rejected or represented as a conflicting provider query; webctx must not silently choose one resource type and claim the other.

**Selected:** semantic qualifier preservation.

### Decision 9. Child index prioritization is deterministic state ordering, not heuristic relevance

**Reversible.**

When an index cannot fit every child:

1. failure/error/cancelled/timed-out or equivalent actionable states;
2. in-progress/queued/pending states;
3. remaining children in stable provider/chronological order.

For human conversations without machine state, preserve chronological order and make omissions explicit. Do not rank by “maintainer,” bot/human, reactions, or inferred importance.

**Selected:** deterministic state/chronology only.

### Decision 10. Existing raw diff/patch/blob paths remain intentionally large

**Public contract.**

Overview changes do not truncate `.diff`, `.patch`, raw blob, or explicit raw download representations merely because they exceed 5k. They are the opt-in bulk paths.

**Selected:** preserve.

### Decision 11. Long-tail container hardening is included where current code has the same fan-out class

**Scope decision.**

Included in this workstream:

- large tree listings;
- commit root and commit comments;
- release detail/assets;
- contributor statistics;
- deployment-environment statuses;
- Discussion detail;
- Gist root/comments;
- selected check machine text;
- Actions roots/runs/jobs;
- PR files when many patches would exceed the overview budget.

This is class-level scope, not unrelated feature expansion.

### Decision 12. Security/admin/settings pages stay out of native scope

**Hard exclusion.**

No security-alert/secret/code-scanning/settings/admin/billing native readers are added by this workstream.

### Assumption 1. Discussion numeric comment selectors remain recoverable from GraphQL

**Working assumption:** copied `#discussioncomment-<databaseId>` URLs can be matched against `DiscussionComment.databaseId` while preserving canonical `url` truth.

**Why reasonable:** GitHub's current GraphQL DiscussionComment schema exposes `databaseId`, and live Discussion URLs use `#discussioncomment-*`.

**If false:** Phase 8 must keep the root bounded and expose canonical returned comment URLs but mark exact selector reads unsupported rather than inventing a lookup.

**Proof boundary:** Phase 8 provider fixture + live Discussion comment probe.

### Assumption 2. Gist exact comment selectors are usable only when GitHub returns a canonical comment URL/ID pairing

Do not invent a `#gistcomment-*` syntax merely because the REST endpoint accepts a numeric ID. Phase 8 must prove the copied GitHub web fragment from live/provider data before registering it.

### Assumption 3. Actions raw-log UX should expose a stable endpoint as well as any short-lived redirect only when safe

GitHub documents the downloaded job log as a 302 to a one-minute URL. A printed expiring storage URL alone is not durable navigation.

Phase 5 must prove whether the best user-facing hint is the stable GitHub API log endpoint, the returned temporary URL, or a pair with explicit expiry semantics. It must never expose the token or imply a short-lived URL is durable.

### Assumption 4. Release notes have no proven exact full-body fragment

The plan therefore does not invent one. A bounded release detail can link the canonical release page and all exact asset downloads while truthfully noting when notes are previewed.

## Acceptance Criteria

1. `webctx read-link` continues to select the native GitHub path before generic page crawling for every currently supported native family.
2. Recognized native GitHub failures remain authoritative except for the already-supported narrow Package best-effort fallback; this workstream does not silently scrape Issue/PR/Actions/commit/etc. failures.
3. No GitHub-specific `read-link` flags are added.
4. The shared native GitHub output model distinguishes overview/container, exact semantic selector, and explicit raw/bulk resources.
5. Overview/container output targets roughly 5,000 Unicode runes and does not grow linearly with subordinate body/log/patch size.
6. Overview truncation preserves valid UTF-8 and safe Markdown boundaries.
7. Mandatory metadata, truthfulness notes, and navigation are retained even when a primary preview is truncated.
8. Locally omitted/index-budgeted children are explicitly distinguished from provider-incomplete/provider-ceiling results.
9. Child index entries use deterministic previews; no model/LLM summarization is introduced into `read-link`.
10. Child index hints teach only useful GitHub URL selectors/subresources, not generic shell filtering commands.
11. Existing source line selectors remain exact and narrow.
12. Existing Markdown heading selectors remain exact on valid GitHub blob paths.
13. Existing slash-containing ref/path resolution and ambiguity errors remain correct.
14. Repository-root frontmatter, README preview, and useful URL hints remain compatible with the current clean behavior.
15. Full source/blob reads remain available as explicit content reads and are not silently replaced by overview snippets.
16. Raw PR/commit/compare `.diff` and `.patch` reads remain available and preserve provider bytes within existing safety limits.
17. `GH_TOKEN` remains preferred over `GITHUB_TOKEN` and auth remains optional where GitHub permits anonymous public access.
18. Rate-limit errors continue to surface provider resource/reset/retry data and only suggest adding a token when no token is configured.
19. Fine-grained-token public retry behavior remains centralized and does not leak Authorization to unrelated hosts.
20. Live and deterministic tests never print token values.
21. The Issue timeline decoder accepts GitHub's observed object-shaped `minimized: {"reason": ...}` representation without failing the Issue read.
22. The Issue timeline decoder continues to support boolean/null/non-minimized comment states already covered by current behavior.
23. Optional/unknown timeline enrichment fields cannot cause the primary Issue body to become unreadable solely because a nonessential field changed shape.
24. A small Issue whose complete human-readable body/conversation fits the overview budget retains the current readable full-conversation style.
25. A large Issue switches to deterministic bounded overview/index form rather than dumping every full comment body.
26. Large Issue overview includes Issue metadata, body preview, relationships/pinned facts where present, timeline/comment counts or truthful availability, and exact child URLs for indexed comments.
27. Large Issue comments indexed in the overview include comment ID, author, timestamp, short preview, and `#issuecomment-<id>` URL.
28. Large Issue body preview includes a canonical `#issue-<Issue id>` full-body selector URL.
29. `read-link` on a valid Issue `#issue-<id>` selector returns the exact Issue description body without expanding timeline/comments.
30. `read-link` on an Issue `#issuecomment-<id>` remains a narrow one-comment read.
31. A mismatched Issue body selector ID is rejected truthfully rather than returning the wrong Issue body.
32. `cli/cli#326` or an equivalent fixture with object-shaped minimized data is readable through the native Issue path.
33. Bare `/issues` structured list behavior and stable UI page navigation remain intact.
34. `/pulls` is recognized as a native GitHub target and no longer falls through to Firecrawl/GitHub UI scraping.
35. Bare `/pulls` returns structured PR rows with repository/view/page metadata and canonical PR URLs.
36. Native PR-list rows include at least PR number/title/state or draft state/author and updated or created time when GitHub returns them.
37. PR-list labels/review metadata are included only when available without N+1 expansion; the list remains compact.
38. `/pulls?page=N` preserves the selected page and emits correct Previous/Next GitHub web URLs where provider pagination supplies them.
39. Supported `/pulls` state/head/base/sort/direction query intent is preserved against GitHub's List Pull Requests endpoint.
40. Search-style `/pulls?q=...` uses GitHub Search semantics scoped to the repository and forces/preserves `is:pr` rather than scraping UI filters.
41. `/issues?q=...is:pr...` returns PR results, not Issues.
42. `/issues?q=...is:issue...` continues to return Issue results.
43. Queries with no explicit PR qualifier continue to follow the Issue-list/search contract.
44. Conflicting explicit PR/Issue qualifiers are never silently rewritten into a different resource type.
45. Unsupported PR-list/query parameters fail before provider calls or fall through only when the native parser deliberately does not claim the URL; understood filters are never silently dropped.
46. A PR root always returns the structured PR-overview format rather than the complete transcript format.
47. PR frontmatter preserves state/draft/merged/title/author/base/head/timestamps/commit/file/addition/deletion/comment counts already available today.
48. PR description is previewed safely within the overview budget.
49. PR overview exposes the exact full-description `#issue-<Issue-side database id>` URL.
50. `read-link` on a valid PR `#issue-<id>` returns the full PR description without expanding conversation/reviews/threads.
51. A mismatched PR description selector is rejected truthfully.
52. Ordinary PR conversation comments are indexed with author, timestamp, stable comment ID, short preview, and exact `#issuecomment-<id>` URL.
53. PR reviews are indexed with state, reviewer, submitted time, review ID, short preview when a body exists, and exact `#pullrequestreview-<id>` URL.
54. Inline review threads are indexed with root comment ID, file/line coordinate when available, resolved/outdated state when available, author/time, short preview, and exact `#discussion_r<id>` URL.
55. PR overview does not embed complete bodies for every conversation comment, review, or inline thread.
56. PR overview makes omitted child counts/availability explicit when all indexes do not fit the overview budget.
57. PR overview always exposes `/files`, `/commits`, and `/checks` child URLs.
58. Existing exact PR comment/review/thread selectors remain narrow and return the selected semantic content without unrelated siblings.
59. REST substantive thread content remains available even when optional GraphQL resolved/outdated enrichment fails.
60. Bot comments are not categorically dropped; they receive the same deterministic indexed treatment as human comments.
61. PR output never ranks comments by inferred importance, maintainer status, or model heuristics.
62. PR `/files` remains a distinct child view and preserves exact `#diff-...` file/hunk selectors.
63. A small PR Files view that fits the overview budget may retain its current useful patch presentation.
64. A large PR Files view switches to a bounded file index/patch preview rather than expanding every returned patch.
65. A file-index entry exposes filename/status/change counts and the exact GitHub diff selector/blob URLs needed to drill into that file when available.
66. Exact PR diff-file/hunk selectors continue to return the selected patch/hunk faithfully.
67. PR `/commits` remains a compact commit list with current completeness truth.
68. PR `/checks` frontmatter still reports head SHA and provider-reported check/status counts.
69. PR `/checks` renders an aggregate status/conclusion rollup before individual runs.
70. Failed/error/cancelled/timed-out checks are indexed before successful routine checks when the overview cannot show every run.
71. In-progress/queued/pending checks are indexed ahead of routine successes after hard failures.
72. Every indexed check run exposes its ID and exact `?check_run_id=<id>` URL, not only runs with annotations.
73. PR `/checks` does not embed every check's full output summary when many runs exist.
74. A focused check remains bound to the selected PR head SHA and rejects ownership mismatch.
75. A focused check safely previews oversized output summary/annotation raw details rather than injecting unbounded machine-generated text.
76. Focused check output exposes GitHub Details URLs or other canonical deeper resources when returned by the provider.
77. Actions root remains a structured native view and no longer needs to enumerate both 30 workflows and 30 runs when that exceeds the overview budget.
78. Actions root exposes direct `/actions/workflows` and recent-run navigation sufficient to reach the omitted collection.
79. Actions workflow detail remains a bounded run list with stable page navigation.
80. Actions run frontmatter preserves run ID/number/attempt/status/conclusion/event/head/actor/timestamps plus truthful job/artifact counts.
81. Actions run does not emit every job and every artifact merely because provider pagination is available.
82. Actions run provides a deterministic job-state rollup and indexes failure/non-success jobs before routine successes.
83. Indexed Actions jobs expose exact `/actions/runs/<run>/job/<job>` URLs.
84. Actions run summarizes artifacts with counts/size/expiry facts and only a bounded subset of individual artifact links when necessary.
85. Actions run avoids unnecessary provider pagination when later pages are not required for its truthful bounded overview.
86. A selected Actions job always returns its structured job/step metadata even when logs are unavailable or expired.
87. A selected Actions job no longer injects an arbitrarily large full log by default.
88. A selected Actions job shows a bounded log preview chosen deterministically from the selected job, with failure/terminal context favored when structured step/log boundaries permit it.
89. The selected job clearly states when the log preview was truncated.
90. Selected job output exposes a safe canonical raw/full-log route or download mechanism based on GitHub's documented job-log endpoint without leaking credentials.
91. Any printed temporary Actions storage URL is labeled with its short-lived nature; a one-minute redirect URL is never presented as durable navigation.
92. Existing token non-forwarding across Actions log redirects remains covered.
93. A commit root preserves identity/message/author/verification/parent/stats facts while becoming a bounded changed-file/comment overview.
94. Plain commit detail does not render every changed-file patch by default when the output would exceed the overview budget.
95. Commit changed-file entries include status/change counts and exact blob/diff navigation where available.
96. GitHub commit `#diff-<sha256(path)>` file/hunk fragments are supported when the copied commit page supplies the same proven selector shape as PR Files.
97. Commit `#commitcomment-<id>` selects one commit comment through GitHub's single-comment REST endpoint and verifies it belongs to the selected commit/repository.
98. Raw commit `.diff` / `.patch` remain full explicit bulk reads.
99. Plain compare fetches enough provider data for metadata plus a bounded commit/file index without following arbitrary numbers of commit pages merely to print them all.
100. Plain compare does not embed every changed-file patch.
101. Compare frontmatter truthfully distinguishes total commits, indexed/returned commits, first-page file availability, GitHub's 300-file ceiling, and local omission.
102. Plain compare always hints the exact `.diff` and `.patch` URLs as the full bulk representations.
103. The adjacent-canary Next.js compare used during planning no longer produces hundreds of thousands of characters from the plain compare URL.
104. A large compare that currently times out while following dozens of pages returns a useful bounded overview within the native timeout when GitHub's first required provider calls succeed.
105. Existing slash-containing compare ref handling remains correct.
106. Release lists remain compact and do not expand release notes.
107. Release detail preserves release metadata but previews oversized release notes within the shared overview budget.
108. Release detail renders a bounded asset index with exact browser download URLs and truthful omitted/total information when many assets exist.
109. Release detail never invents a nonexistent full-notes selector; it links the canonical GitHub release page and labels the notes as previewed when truncated.
110. Contributor statistics order contributors deterministically by a useful metric (commit total descending, stable tie-break) before applying the overview budget.
111. Contributor statistics report returned/omitted counts when not every contributor row is rendered.
112. Large tree listings remain one-level native views but use the shared output budget and truthfully state omitted entries/provider 1,000-entry ceiling separately.
113. Deployment-environment detail does not fetch/render unbounded status history for every deployment when the overview only needs current/latest state.
114. Deployment output preserves latest state, deployment identity, logs/environment URLs, and truthful indication that older statuses may exist upstream.
115. Discussion lists retain the current first-30/no-fake-cursor truthfulness.
116. Discussion detail uses the shared bounded overview for large bodies/comments/replies instead of expanding the complete conversation.
117. Discussion overview indexes comments/replies with author, timestamp, numeric database ID when available, short preview, and canonical `#discussioncomment-*` URL.
118. `#discussioncomment-*` selects the exact Discussion comment/reply when GitHub's current GraphQL identity permits it, without expanding unrelated conversation.
119. If a copied Discussion comment selector cannot be resolved truthfully under the current provider contract, webctx returns a precise unsupported/not-found result rather than the wrong comment.
120. A Gist root remains structured but does not expand every full file plus every comment when that exceeds the overview budget.
121. Gist root indexes files with filename/language/size and exact existing `#file-...` selector/raw URL.
122. Existing exact Gist file/line selectors remain faithful.
123. Gist comments in a root overview include stable ID/author/time/short preview and canonical comment URL when GitHub returns one.
124. Exact Gist-comment selection is added only for a provider-proven copied GitHub fragment shape and uses the single-comment REST endpoint; no synthetic fragment is invented.
125. Projects, Packages, Search, profile tabs, activity, branches, tags, forks, history and other already-bounded list surfaces retain their stable query/page semantics.
126. Security/admin/settings pages remain outside native GitHub scope.
127. Current Firecrawl behavior for non-GitHub pages remains unchanged by GitHub output budgeting.
128. Current best-effort GitHub Package crawl behavior remains narrowly scoped to its existing auth/permission cases.
129. Current GitHub REST API version pin remains `2026-03-10` unless current provider documentation at implementation time requires a deliberate update.
130. User-facing docs describe the overview/exact/raw URL mental model without exposing phase/history/internal implementation detail.
131. User-facing docs teach `/pulls`, `#issue-*`, PR comment/review/thread selectors, focused checks, Actions job/raw-log navigation, commit comments, Discussion/Gist selectors only where actually implemented.
132. Docs do not teach shell filtering as a webctx feature; hints focus on URL-native narrowing that avoids fetching/emitting unrelated content.
133. Credential docs continue to describe GitHub auth as optional and `GH_TOKEN` precedence correctly.
134. Troubleshooting docs distinguish provider rate limits, permission failures, provider ceilings, local bounded previews, and unavailable raw logs.
135. Existing release/package metadata and npm/native binary behavior remain unchanged except for intentional documentation/version notes made when this feature is released.
136. Deterministic tests include pathological large bodies/comments/reviews/patches/check summaries/logs/artifact lists/discussion replies and prove output growth is bounded.
137. Deterministic tests include both locally omitted content and provider-incomplete states and assert they are not conflated.
138. Deterministic tests prove every newly indexed selector URL maps back to the intended exact native resource.
139. Live validation uses public GitHub fixtures for `/pulls`, PR root, checks, Actions run/job, compare, old Issue timeline shape, commit/release/Discussion, and selector drill-down where the fixtures still exist.
140. Live validation records actual output size/shape without storing secrets or private response bodies in the repository.
141. `go test ./...`, `go vet ./...`, npm shim checks, applicable docs checks, and build/package smoke all pass at the final acceptance boundary.
142. The final route audit finds no supported container family that still eagerly renders unbounded subordinate provider bodies without either a documented intentional exception or an exact/raw opt-in URL.
143. No temporary compatibility path, duplicate GitHub router, or permanent legacy renderer remains after the workstream completes.

## Plan Phases

## Phase 1 — Shared bounded-context contract and resilient Issues

### Files to read before starting

**Orientation and shared contracts**

- `gg/github-read-link-context-hardening/github-read-link-context-hardening-sweep-2026-08-14.md` — read **Architecture map**, **Shared GitHub client behavior**, **Shared truncation primitive**, **Issue detail behavior**, **Landmines 1–6**, and **Existing patterns worth copying**.
- `internal/app/github.go` — `GitHubTarget`, `GitHubClient`, `RESTPages`, `readGitHubNative*`, `truncateMarkdownSafe`, shared render/navigation helpers; this is the cross-family seam for output-budget vocabulary.
- `internal/app/tools.go` — `ReadLink` native outcome handling; ensure native failures remain authoritative.

**First vertical slice**

- `internal/app/github_issues.go` — `githubTimelineEvent`, `readGitHubIssue`, `fetchGitHubIssueTimeline`, `renderGitHubIssue`, `renderTimelineComment`, Issue selector/list helpers.
- `internal/app/github_issues_test.go` — complete timeline/relationships tests, minimized comment tests, selector/list/search tests.
- `internal/app/github_test.go` — repository truncation helper tests and client/auth/pagination tests worth copying.
- `src/content/docs/read-link.md` — current Issue/body/selector user contract; update only behavior actually landed in this phase.

### What to do

Establish the shared **overview budget / bounded preview / compact indexed child / omitted-content truth** concepts at the GitHub-rendering layer without building a generic document framework for non-GitHub pages.

Reuse the existing Markdown-safe truncation behavior rather than adding tokenization. The shared helpers should let later family renderers reserve metadata/navigation, build deterministic previews, and distinguish locally omitted content from provider incompleteness.

Use Issues as the first real vertical proof:

- normalize GitHub's polymorphic `minimized` timeline shape, including the observed object form, without losing current minimized-body truthfulness;
- make optional timeline metadata decode robust enough that a shape change cannot erase the primary Issue body;
- add exact `#issue-<Issue id>` body selection and identity verification;
- preserve the current full human-readable rendering for a naturally small Issue;
- when the complete Issue would exceed the overview budget, render metadata + body preview + relationship/pinned facts + compact chronological timeline/comment index instead of every full body;
- every indexed comment must expose its existing `#issuecomment-*` URL and stable identity;
- make omission/local-budget state explicit.

Do not add flags. Do not change bare `/issues` list/search behavior in this phase except where shared helpers require regression-safe adaptation. Do not solve PR rendering yet.

### Validation strategy

**Positive evidence**

- Add deterministic fixtures for boolean/null/object-shaped `minimized`, including the exact `{"reason":"spam"}` shape observed live.
- Add a synthetic large Issue whose many/large comments would have produced a very large current response; assert bounded overview size, body/comment previews, omitted state, selector URLs, and safe Markdown.
- Add a small Issue fixture proving the readable full conversation remains.
- Add `#issue-*` body selector tests for valid/mismatched identities.
- Live-read `https://github.com/cli/cli/issues/326` (or another issue still exposing the object shape) and show that the previous decode failure is gone.

**Regression evidence**

- Existing `#issuecomment-*`, relationship, pinned-comment, label/milestone, list/search, client/error, repository README and source-selector tests remain green.
- `go test ./...` and `go vet ./...` pass.

### What must not break

- native failure authority versus Firecrawl;
- exact Issue comment ownership validation;
- current meaningful/noise timeline-event filtering;
- pinned comment non-duplication;
- provider ceilings vs local omission semantics;
- repository-root README preview behavior;
- generic non-GitHub `read-link`.

## Phase 2 — Native Pull Request lists and PR-qualified search correctness

### Files to read before starting

- Sweep: **Issue lists and search**, **Pull Request list behavior**, **Landmines 8–9**.
- `internal/app/github.go` — `GitHubTargetKind` and `parseGitHubTarget`; add the list target without disturbing existing `/pull/<n>` parsing.
- `internal/app/github_issues.go` — current Issue list/search provider query construction and UI page-navigation helper usage.
- `internal/app/github_search_profiles.go` — existing `pullrequests` Search mapping and query semantics; pattern for native PR search results.
- `internal/app/github_pulls.go` — PR data types usable for list rows.
- `internal/app/github_test.go`, `github_issues_test.go`, `github_search_profiles_test.go`, `github_pulls_test.go` — parser/list/query invariants.
- GitHub Pull Requests REST documentation — verify current filter/sort/page contract before coding.

### What to do

Add a first-class native PR-list target for `/owner/repo/pulls`.

Bare/native-list mode should use GitHub's List Pull Requests resource and preserve understood `state`, `head`, `base`, `sort`, `direction`, and `page` semantics without N+1 enrichment.

Search/filter mode should handle copied GitHub `q=` PR URLs through Search Issues with repository scope and explicit PR semantics. Reuse the existing compact Issue/Search list vocabulary but label the resource truthfully as Pull Requests.

Fix `/issues?q=...` classification so explicit `is:pr` queries do not get rewritten with `is:issue` and then stripped of PRs. Preserve explicit Issue searches. Handle conflicting explicit qualifiers truthfully rather than silently changing intent.

The output should be the PR analogue of the current clean `/issues` list: compact frontmatter, one row per result, stable PR URL, and provider-derived web pagination.

Do not scrape GitHub `/pulls` UI. Do not reproduce GitHub's filter menus/ProTips/loading state.

### Validation strategy

**Positive evidence**

- Parser tests for bare `/pulls`, page/query/filter forms and malformed siblings.
- Deterministic REST list tests for open/closed/draft PR identity, sort/filter forwarding, and Previous/Next URL reconstruction.
- Deterministic search tests for `/pulls?q=...is:pr...` and `/issues?q=is:pr...` returning PRs.
- Explicit Issue-search regression fixture proving `is:issue` still returns Issues.
- Live compare `vercel/next.js/pulls` output against the current scraped-UI failure: no `Uh oh!`, loading/login chrome, ProTips, duplicated pagination, or Firecrawl dependency.

**Regression evidence**

- Existing `/issues`, global/repo GitHub Search, PR detail and selector tests remain green.
- Unsupported query intent is rejected before silent semantic loss.

### What must not break

- `/pull/<n>` detail/subview parsing;
- Search provider rate-limit/error truth;
- stable `page=` web URL navigation;
- Issue list PR filtering when the resource is genuinely Issues;
- generic GitHub Search `type=pullrequests` behavior.

## Phase 3 — Bounded PR root and complete selector discoverability

### Files to read before starting

- Sweep: **Pull Request detail behavior**, **Live PR context growth**, **Exact PR selectors**, **PR description body identity**, **Landmines 3, 7, 10, 13**.
- `internal/app/github_pulls.go` — `readGitHubPullRequest`, review/comment/thread fetches, `renderGitHubPullRequest`, existing exact selector readers/renderers.
- `internal/app/github_issues.go` — shared Issue-side identity/comment rendering and the Phase 1 body selector pattern.
- `internal/app/github.go` — shared overview/index helpers from Phase 1.
- `internal/app/github_pulls_test.go` — current complete-transcript assertions that must be deliberately replaced, plus exact-selector/GraphQL tests that must stay.
- `src/content/docs/read-link.md` — PR navigation docs.

### What to do

Change the PR root from “complete transcript” to a stable structured overview.

Fetch the Issue-side PR representation needed to obtain the canonical `#issue-<id>` body selector. Preserve PR REST metadata as the PR truth for branches/stats/state.

Render:

- existing PR frontmatter facts;
- bounded description preview + exact full-description selector;
- compact substantive event/comment index;
- compact review index;
- compact inline-thread index;
- omission/availability facts when the complete indexes do not fit;
- `/files`, `/commits`, `/checks` navigation.

Each ordinary comment/review/thread index item must expose stable identity, actor/author, timestamp, short deterministic preview, state/coordinate where relevant, and exact existing GitHub selector URL.

Preserve chronological order for human conversation. Do not suppress bots as a class or rank maintainers above other participants. Keep REST thread content authoritative when GraphQL state enrichment is unavailable.

Extend PR fragments with `#issue-<Issue-side id>` body selection. Existing `#issuecomment-*`, `#pullrequestreview-*`, and `#discussion_r*` remain exact/full semantic reads.

### Validation strategy

**Positive evidence**

- Rewrite the current complete-PR fixture into a bounded-root fixture while retaining all current semantic metadata/events as index entries.
- Add synthetic very large body/comment/review/thread fixtures and prove output remains bounded while every emitted item has the correct selector URL.
- Assert PR body selector uses Issue-side ID, not PR REST ID.
- Live-read both `vercel/next.js#97343` and `cli/cli#13250`; record bounded size and verify their large child bodies are now opt-in via printed selectors.
- Follow printed comment/review/thread/body selectors and prove the exact content remains accessible.

**Regression evidence**

- Exact selector tests continue to prove narrow request counts and ownership.
- GraphQL state enrichment fallback remains non-fatal.
- PR state/draft/merged/base/head/stats and raw `/files` `/commits` `/checks` URLs remain correct.

### What must not break

- selected review includes its own review comments;
- selected thread includes the selected reply/root relationship;
- invisible GitHub HTML comment stripping;
- bot content availability through exact selectors;
- thread resolved/outdated enrichment semantics;
- no GraphQL requirement for anonymous substantive PR reads.

## Phase 4 — PR Files and Checks bounded subviews

### Files to read before starting

- Sweep: **PR Files view**, **PR Checks view**, **Focused check behavior**, black-box Checks metrics.
- `internal/app/github_pull_views.go` — file pagination/rendering, diff selector parsing, checks fetch/sort/render, focused check/annotations.
- `internal/app/github_pull_views_test.go` — file selector/provider-cap tests and checks/focused-check tests.
- `internal/app/github.go` — Phase 1 bounded-index helpers.
- GitHub Checks/Actions provider docs for current check-run/annotation limits as needed.

### What to do

Keep `/files` and `/checks` as distinct PR child URLs, but prevent those container views from becoming a second context bomb.

For Files:

- preserve the current full patch presentation when a small file set naturally fits;
- otherwise render a compact file index with status/addition/deletion/change facts, patch preview only as budget permits, exact GitHub diff selector, blob/raw links already available, and provider/local completeness truth;
- preserve exact `#diff-...` file/hunk selection as the opt-in full semantic diff slice.

For Checks:

- calculate compact conclusion/status counts;
- order failed/error/cancelled/timed-out states first, then active/pending, then routine successes;
- include a `?check_run_id=` URL for every indexed run, not only annotated runs;
- do not embed every run's full `Output.Summary` on the container page.

For focused checks:

- keep PR-head ownership verification;
- preserve annotation coordinates/messages;
- bound oversized provider-generated Summary/RawDetails/annotation collections and link the provider Details URL when supplied.

### Validation strategy

**Positive evidence**

- Synthetic 100+/3000-file PR fixtures prove Files output growth is bounded while provider-cap/local-omission state remains distinct.
- Existing diff-selector fixture still yields exact patch/hunk.
- Synthetic 100+ check-run fixture proves rollup, state priority, focused URL on every indexed row and bounded size.
- Oversized focused-check Summary/annotations fixture proves bounded machine text + Details URL.
- Live Next.js `/pull/97343/checks` drops from the ~30k baseline to a compact actionable index while the focused failing check remains readable.

**Regression evidence**

- PR Files 3,000-file provider ceiling remains truthful.
- combined commit statuses remain separately represented and are not conflated with branch-protection decisions.
- focused check ownership mismatch still fails.

### What must not break

- SHA-256(path) diff identity;
- left/right line selector semantics;
- raw PR `.diff`/`.patch` behavior;
- check annotations' source coordinates;
- PR `/commits` compact view.

## Phase 5 — Actions overview/run/job context safety and raw-log navigation

### Files to read before starting

- Sweep: **Actions overview**, **Actions run behavior**, **Actions job behavior**, **Landmines 5 and 6**.
- `internal/app/github_actions.go` — overview/workflow/run/job fetch and render paths, log redirect/decode behavior.
- `internal/app/github_actions_test.go` — run pagination, selected-job full-log expectations, unavailable log and redirect-token safety.
- `internal/app/github.go` — shared budget/provider client; do not bypass its auth/origin safeguards.
- `src/content/docs/read-link.md`, `credentials.md`, `troubleshooting.md` — current Actions/log wording.
- GitHub Workflow Jobs and Workflow Runs docs — verify redirect expiry/auth semantics at implementation time.

### What to do

Apply the overview contract at three Actions levels.

**Actions root:** compact the combined workflows/recent-runs surface and explicitly link `/actions/workflows` plus run-page navigation rather than spending the whole budget duplicating both lists.

**Workflow:** keep the current bounded run list/page behavior; adopt shared budget helpers if long names/rows make it oversized.

**Run:** render run identity + job/artifact totals + state rollups + prioritized bounded job index. Do not fetch/render every artifact page simply for completeness; summarize artifacts and expose only as many exact download/API identities as fit. Provider-more/local-omission state must be explicit.

**Job:** always render structured step state. Replace unconditional entire-log emission with a bounded deterministic log preview. When failure context can be identified from structured steps/log markers, prioritize the failed/terminal portion plus enough surrounding context to diagnose it; otherwise use a deterministic head/tail or equivalent bounded strategy and label it.

Expose the stable GitHub job-log endpoint and, if useful/safe, the returned short-lived download location with explicit expiry semantics. Do not print or forward credentials to storage hosts.

No new `--log` or `--full` flag.

### Validation strategy

**Positive evidence**

- Rewrite the current Actions-run pagination fixture so it proves truthful counts/rollups/index omission rather than every artifact body row.
- Synthetic 100+ jobs/artifacts fixture remains bounded and orders failures/active states ahead of successes.
- Rewrite selected-job tests with a very large log and prove bounded preview + truncation note + raw/stable log navigation.
- Failure-log fixture proves failed-step/terminal context is retained deterministically.
- Preserve 410/expired log behavior.
- Live Next.js run `31757053478` and jobs `94635412218` / `94635017814` no longer inject 40k/150k/216k into ordinary reads.

**Regression evidence**

- job/run ownership remains validated;
- redirect token is never forwarded to storage host;
- malformed ZIP/non-text log errors remain truthful;
- workflow/root query/page semantics remain intact.

### What must not break

- Actions auth permissions and public-read behavior;
- run attempt/latest-job semantics;
- log retention truth;
- no local cache;
- exact job URLs;
- artifact expiry/download metadata accuracy.

## Phase 6 — Commit and compare overview/raw split plus commit selectors

### Files to read before starting

- Sweep: **Commit detail behavior**, **Compare behavior**, **Commit comment selectors**, compare latency evidence, **Landmines 1–4, 12**.
- `internal/app/github_commits.go` — commit pagination/rendering, comments, compare pagination/rendering, raw diff/patch, history/blame.
- `internal/app/github_pull_views.go` — existing diff selector/hash parser/patch selection to reuse for commit-page diff anchors.
- `internal/app/github_commits_test.go`, `github_pull_views_test.go` — provider caps, slash refs, raw media, diff selector patterns.
- GitHub commit comments and compare REST docs.

### What to do

Turn plain commit and compare URLs into bounded overviews while preserving raw `.diff`/`.patch` as explicit bulk resources.

**Commit root:** preserve commit identity/message/verification/parent/stats, but index changed files and commit comments without expanding every patch/body. Do not page through thousands of changed files simply to print them all when the overview is already full. Reuse GitHub's proven commit-page `#diff-<sha256(path)>` anchors to select one file/hunk using the existing PR diff-selector machinery.

Add `#commitcomment-<id>` exact comment selection using GitHub's single-comment REST endpoint and validate the selected comment's commit/repository identity.

**Compare root:** fetch only provider data needed for a truthful overview. Keep first-page changed-file metadata/provider 300-file semantics, compact commit index, no full patches. Do not follow arbitrary commit pages solely to expand the overview. Always print `.diff` and `.patch` URLs as the explicit full bulk forms.

Do not register a compare diff fragment unless GitHub's copied web URL shape is proven; planning did not establish a stable compare `#diff-*` anchor.

History/blame remain their current bounded list/range contracts.

### Validation strategy

**Positive evidence**

- Synthetic multi-thousand-file commit proves bounded output and no unnecessary full file pagination.
- Commit `#diff-*` fixture copied from GitHub's real path-hash scheme returns exact file/hunk.
- Commit `#commitcomment-*` fixture uses only the single-comment endpoint and rejects mismatched commit identity.
- Rewrite compare pagination test to prove first-page/overview truth and no blind full pagination.
- Plain compare fixture with huge patches stays bounded; `.diff`/`.patch` still preserve raw bytes.
- Live adjacent-canary Next.js compare no longer emits ~869k; `v15.5.0...canary` no longer hits the 30-second deadline merely to enumerate all commits.

**Regression evidence**

- slash-containing compare refs;
- provider 300-file/3,000-file ceilings;
- history page navigation;
- blame GraphQL/auth behavior;
- raw diff/patch media types.

### What must not break

- commit verification/parent identity;
- provider incomplete vs local omission distinction;
- commit message fidelity;
- raw representations;
- current 100 MB provider body safety ceiling.

## Phase 7 — Releases, trees, statistics, and deployment fan-out hardening

### Files to read before starting

- Sweep: **Release detail behavior**, **Tree behavior**, **Repository statistics**, **Deployment environment fan-out**, relevant live measurements.
- `internal/app/github_refs.go` — releases/assets plus bounded-list patterns.
- `internal/app/github.go` — tree rendering and directory README preview.
- `internal/app/github_activity_deployments.go` — stats/deployment status loops.
- `internal/app/github_refs_test.go`, `github_test.go`, `github_activity_deployments_test.go` — current long-body/assets/tree/provider-status expectations.
- `src/content/docs/read-link.md` — release/activity/deployment user paths.

### What to do

Apply the same bounded-container rule to remaining native readers that currently fan out subordinate rows.

**Release detail:** preview long notes, retain release metadata, bound the asset index, retain exact browser download URLs and provider/local omission facts. Do not invent a full-notes selector; the canonical GitHub release page remains the human source.

**Tree:** preserve one-level listing and directory README preview, but prevent a 1,000-entry directory from generating a huge response. Prefer directories before files only if that deterministic ordering is explicitly chosen and documented; otherwise preserve sorted names and truncate with omitted count. Keep the provider 1,000-entry warning separate.

**Contributor statistics:** sort deterministically by commit total descending (stable tie-break), render the most useful bounded top set, and report omitted count. Do not imply GitHub's cached statistics are live just because webctx made a live request.

**Deployment environment:** avoid fetching/rendering complete status history for every deployment when latest/current state is enough for the bounded overview. Preserve direct log/environment URLs and a truthful statement that older statuses may exist.

Review adjacent fixed-size stats/list surfaces for accidental budget overruns, but do not churn already-compact branches/tags/forks/activity/history just to hit an exact character number.

### Validation strategy

**Positive evidence**

- Rewrite the long-release test to assert safe preview + exact assets + bounded omission instead of full 500-line notes.
- Synthetic hundreds-of-assets/tree-entries/contributors/deployment-statuses fixtures prove bounded growth.
- Contributor ordering test proves highest totals appear first and tie behavior is deterministic.
- Deployment request-count test proves later status pages are not fetched when not needed for the overview.
- Live Next.js `v15.5.0` release and contributors page fall to useful bounded output.

**Regression evidence**

- release list remains body-free and paginated;
- release tag names containing `/` still resolve;
- tree slash-ref/ambiguity semantics and README preview remain correct;
- statistics 202/computing state remains truthful;
- branch/tag/fork/social-list behavior remains stable.

### What must not break

- release asset download URLs;
- provider cached-statistics warning;
- tree 1,000-entry ceiling truth;
- deployment latest status identity;
- stargazer/watcher auth truth;
- no local caching.

## Phase 8 — Discussions and Gists bounded navigation plus exact comment selectors

### Files to read before starting

- Sweep: **Discussions list/detail**, **Gist behavior**, **Factual gaps 1–2**.
- `internal/app/github_discussions_gists.go` — GraphQL pagination/data model/rendering, Gist comments/file selectors/raw retrieval.
- `internal/app/github_discussions_gists_test.go` — current complete pagination/file/comment expectations.
- `internal/app/github.go` — target fragment plumbing/shared budget helpers.
- GitHub Discussions GraphQL guide — `DiscussionComment.databaseId`, `url`, reply connections.
- GitHub Gist comments REST docs — single-comment endpoint and current response identity.

### What to do

Keep Discussions list behavior truthful about its non-web-addressable GraphQL cursor, while hardening detail pages.

**Discussion detail:** preview the main body, compact comments/replies into an indexed conversation, preserve accepted-answer identity, expose canonical `#discussioncomment-*` URLs/database IDs, and add exact comment/reply selection if current GraphQL can resolve the copied numeric identity truthfully. Avoid fetching every reply page simply to emit a bounded root when unnecessary.

**Gist root:** for multi-file/large Gists, render metadata + bounded file index/content previews + comment index + revision summary rather than every full file/comment. Existing `#file-*` selectors remain the explicit full file/range path. Add an exact Gist-comment read only when a real GitHub canonical fragment is proven from provider/live data; use `GET /gists/{gist_id}/comments/{comment_id}` and validate ownership. If no canonical web selector is proven, still surface returned canonical comment URLs but do not invent syntax.

### Validation strategy

**Positive evidence**

- Synthetic long Discussion body + hundreds of comments/replies remains bounded, preserves accepted answer/index truth, and avoids unnecessary deep pagination.
- Prove `#discussioncomment-*` numeric lookup against a provider fixture and one live public Discussion comment when available.
- Synthetic multi-file Gist with large files/comments stays bounded; exact file selector remains full/narrow.
- Provider/live Gist comment identity test decides the selector assumption before registering it.
- Live Next.js Discussion `96973` drops from ~17k to bounded overview while its printed comment URLs remain useful.

**Regression evidence**

- Discussions still require auth truthfully;
- no fake cursor URL is introduced;
- Gist truncated-file raw retrieval and file-line selectors remain correct;
- Gist revision metadata remains truthful.

### What must not break

- accepted-answer marker;
- Discussion reply parentage;
- canonical provider URLs;
- Gist raw-host token safety;
- exact selected-file fidelity.

## Phase 9 — Cross-family route audit, hints, docs, and release-facing contract

### Files to read before starting

- Full current implementation plan including Amendments.
- Sweep: **Black-box stress matrix**, **Current output vocabulary**, **Exact selector paths already proven useful**, **Current user-facing docs**, **Landmines 14–18**.
- `internal/app/github*.go` — sample every supported target dispatch and identify any remaining overview/container that expands subordinate bodies.
- `internal/app/tools.go` — ensure native/fallback boundary remains unchanged.
- `src/content/docs/read-link.md`, `architecture.md`, `agent-workflows.md`, `credentials.md`, `troubleshooting.md`, `cli-reference.md`, `changelog.md` — update only shipped behavior.
- `README.md` and landing-page read-link copy only where public descriptions are now materially inaccurate.

### What to do

Run a systematic target-kind audit after the family phases rather than assuming the named stress cases are exhaustive.

Classify every `GitHubTargetKind` as overview, exact selector, raw/bulk, or intentionally unsupported. For each overview, inspect whether current provider work or rendered child content can still grow without a bound. Close remaining class-level gaps using the shared helpers; do not refactor compact surfaces gratuitously.

Normalize useful hint vocabulary so overview outputs surface the best next GitHub URLs without repeating obvious shell capabilities. Hints should be concise and context-specific, not a generic manual appended to every response.

Update user docs around the final mental model:

```text
landing/container URL -> compact overview + exact next URLs
exact selector        -> selected semantic content
raw/bulk URL          -> explicit full representation
```

Document `/pulls`, PR-qualified `/issues?q=...`, `#issue-*`, existing PR selectors, focused checks, bounded Actions job logs + raw endpoint, commit comments/diff selectors, Discussion/Gist selectors only if implemented, and provider/local truncation truth.

Do not expose phase numbers, planning artifacts, fixture IDs, or implementation diary text in user docs.

### Validation strategy

**Positive evidence**

- Build a table in test/audit output mapping every current target kind to its output class and representative boundedness/selector contract.
- Stress synthetic long string/list data through every overview renderer not already covered by earlier phase tests.
- User docs contain runnable canonical examples and no stale claims that a job/root necessarily emits the entire log/transcript.
- Docs checks/build pass using current repository tooling.

**Regression evidence**

- all existing search/map/non-GitHub read-link tests;
- all native URL parser families;
- npm packaging/build smoke;
- credential/error docs remain consistent with code.

### What must not break

- unsupported security/admin namespace boundary;
- Package best-effort fallback warning;
- non-GitHub Firecrawl settings;
- existing release asset/npm binary naming;
- current search ranking behavior.

## Phase 10 — Independent acceptance and live stress matrix

### Files to read before starting

- The full current implementation plan including every Amendment.
- The current progress ledger and latest phase entries.
- The entire current affected GitHub reader/test set, not stale diffs alone.
- User-facing GitHub/read-link docs and release-facing metadata.
- Sweep **Black-box stress matrix**, **Landmines**, and **Factual gaps** as a regression checklist.

### What to do

Perform an independent acceptance pass against **current code**, treating progress claims as evidence to verify rather than proof.

Map all 143 acceptance criteria to actual tests/live evidence. Re-run the live stress matrix where GitHub fixtures remain available, including:

- native `/pulls` + PR filters;
- `/issues?q=is:pr...` correctness;
- large PR root + each printed selector;
- old Issue polymorphic minimized shape;
- large checks + focused check;
- Actions root/run/large successful job;
- adjacent-tag compare + `.diff/.patch` escape hatch;
- large commit/release/Discussion/statistics;
- representative tree/Gist/deployment/profile/search paths.

Record output character counts for overview examples and verify they remain roughly within the target without treating a one-character threshold as correctness. Investigate any output that grows proportional to subordinate body/log/patch size.

Review provider request counts for the biggest containers to ensure rendering became bounded **and** expensive blind pagination was removed where the new contract does not require completeness.

Verify raw/exact URLs still retrieve the selected/full intentional representation.

Close any missing regression, docs mismatch, selector ownership issue, credential leakage risk, or stale compatibility path before declaring complete.

### Validation strategy

**Positive evidence**

- Acceptance matrix covers all criteria with deterministic/live evidence.
- Live measurements demonstrate orders-of-magnitude reduction for the PR/Actions/compare context bombs identified in the Sweep.
- Exact selectors printed by parent indexes are followed and verified end-to-end.
- Request-count evidence demonstrates large compare/container reads do not blindly traverse unneeded pages.

**Regression evidence**

- broad Go tests/vet;
- npm shim checks;
- docs checks/build;
- native binary build + `--version` smoke;
- search/read-link/map-site live smoke with configured provider credentials;
- package/release surface smoke as applicable.

### What must not break

- any criterion already proven in earlier phases;
- clean git history/worktree discipline;
- secrets/private live data hygiene;
- current branch compatibility;
- raw/bulk escape hatches;
- native failure authority and fallback boundaries.

## Amendments

None yet.
