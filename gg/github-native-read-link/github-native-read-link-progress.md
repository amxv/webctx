# GitHub-Native `read-link` Progress

## Workstream

- **Repository:** `amxv/webctx`
- **Checkout:** `/workspace/repos/webctx`
- **Approved branch:** `main`
- **Workstream:** `github-native-read-link`
- **Sweep:** [`github-native-read-link-sweep-2026-08-14.md`](./github-native-read-link-sweep-2026-08-14.md)
- **Implementation plan:** [`github-native-read-link-implementation-plan-2026-08-14.md`](./github-native-read-link-implementation-plan-2026-08-14.md)
- **First implementation prompt:** [`first-agent-prompt.md`](./first-agent-prompt.md)
- **Subsequent implementation prompt:** [`subsequent-agent-prompt.md`](./subsequent-agent-prompt.md)

> Treat current source, tests, generated contracts, and docs as truth. Progress claims are evidence to verify, never proof.

## Standing execution rules

- Fetch first and fast-forward safely on `main`; current remote code is authoritative.
- Never hard-reset away, clean, stage, revert, or overwrite another agent's work.
- Never force-push or rewrite shared history.
- Work from the current phase's scoped reading list rather than rereading every planning artifact.
- Finish an `in_progress` phase before starting a later one.
- A phase is complete only after its observable capability, positive evidence, regression evidence, applicable live evidence, docs/generated-contract work, progress update, coherent commit, push, and remote verification are complete.
- Use live public GitHub evidence where safe and available, plus deterministic fixtures for auth/rate-limit/pagination/cap/error behavior.
- No GitHub response cache is allowed. Do not store or commit tokens, private payloads, private logs, or real private repository content.
- Add an implementation-plan Amendment immediately if current reality disproves a load-bearing assumption or phase instruction. Never weaken an acceptance criterion through an Amendment.
- GitHub security pages are outside this workstream.

## Phase table

| Phase | Capability | Status | Completion commit | Evidence / next boundary |
| ---: | --- | --- | --- | --- |
| 1 | Native GitHub foundation, repository roots, blobs, trees, and selectors | complete | `5c234ce31e6cafb055b3703255f85cb64793d9ea` | Native repository/blob/tree routing, GitHub client/error/auth/pagination boundary, ref resolver, selectors, docs, deterministic/live evidence complete. |
| 2 | Issues, conversations, selectors, lists, labels, and relationships | complete | `7bad0c3ef652560af1dcd7636dfbf0c6b5cc75bb` | Native Issue conversations/comment selectors plus bounded Issue/search/label/milestone views and current relationships are complete. |
| 3 | Pull Request conversation, reviews, threads, and exact anchors | complete | `e953217b9dceeeba0c7208579a5b8c5f16615e68` | Native PR conversations, formal reviews, REST-grouped review threads, three exact anchor classes, and optional GraphQL thread-state enrichment are complete. |
| 4 | Pull Request Files Changed, commits, checks, and diff selectors | complete | pending Phase 4 commit | Native files/diff selectors, commits, checks/statuses/annotations, and raw diff/patch semantics are complete. |
| 5 | Commits, comparisons, path history, and blame | pending | — | Reuse canonical ref resolver/diff model; GraphQL blame is auth-only. |
| 6 | GitHub Actions runs, jobs, logs, artifacts, and check annotations | pending | — | Run pages bounded; job URLs target one job/log. Verify step selectors before advertising them. |
| 7 | Branches, tags, releases, and stable repository navigation lists | pending | — | Add compact ref/release list/detail surfaces. |
| 8 | Discussions and Gists | pending | — | Discussions require GraphQL auth; Gists use public REST and raw fallback for truncation. |
| 9 | GitHub search and public User/Organization navigation | pending | — | Preserve GitHub search quota/query truth and provider-resolve profile type. |
| 10 | Repository activity, metrics, social lists, and deployments | pending | — | Keep provider delay/cache/status-retention truth explicit. |
| 11 | Packages, Projects v2, and long-tail native route closure | pending | — | Fresh route/API audit; stable read-only native or explicit fallback reason. |
| 12 | Independent acceptance, output hardening, docs/package verification, and route-fidelity audit | pending | — | Map all 127 acceptance criteria to current evidence and close gaps. |

## Current handoff

- **Last completed phase:** Phase 4 — `Pull Request Files Changed, commits, checks, and diff selectors`.
- **Earliest incomplete phase:** Phase 5.
- **Exact phase title:** `Commits, comparisons, path history, and blame`.
- **Observable boundary:** reuse the native GitHub client/ref resolver/diff rendering primitives for commit detail/comments, compare ranges, path-scoped history, and auth-only GraphQL blame, without weakening PR view separation or the source ref guarantees.
- **Current blockers:** none known. The Zodex host's anonymous GitHub core quota was exhausted during late Phase 4 live checks, so webctx checks and REST custom-media raw diff/patch were observed returning the correct native rate-limit error rather than live success. Earlier live files/commits succeeded; UI selector/raw-media provider semantics were separately re-probed over github.com. Deterministic fixtures cover the quota-blocked implementation paths.
- **Plan Amendments affecting Phase 5:** none.
- **Prompt to use:** [`subsequent-agent-prompt.md`](./subsequent-agent-prompt.md).

## Progress entries

### 2026-08-14 — Phase 1 — `complete`

- **Agent/session:** GPT-5.6 Sol implementation session on the Zodex checkout.
- **Starting state:** `main` was clean, fetched, and fast-forwarded to `71e9e2a3da8c08d04129d01cb7a69397e9a8b019`; the only newly fetched work was unrelated Vercel docs-build filtering and was preserved unchanged.
- **Ending commit(s):** `5c234ce31e6cafb055b3703255f85cb64793d9ea` — Phase 1 implementation, tests, public docs, and initial durable handoff.
- **Outcome:** `webctx read-link` now has one native GitHub boundary for repository roots, blobs, and trees. Repository roots render compact frontmatter plus bounded human-view README previews and native URL hints; direct blobs preserve full source by default with line/range and Markdown-heading selectors; tree URLs return one-level listings and bounded directory READMEs. Recognized native failures stay authoritative while unsupported GitHub routes retain direct-Markdown/Firecrawl fallback.
- **Files/areas changed:** new `internal/app/github.go` canonical target/client/error/ref-resolution/rendering path and deterministic `github_test.go`; `tools.go` routes through it; the old narrow parser/raw reader was removed from `scrape.go`; GitHub token keys joined the existing credential loader; README/docs/landing/environment template were updated for delivered behavior only.
- **Positive evidence:** deterministic tests cover root/blob/tree classification and unsupported/security routes; REST version/Accept/User-Agent/auth origin and `GH_TOKEN`→`GITHUB_TOKEN` precedence; Link parsing; 401/403/404/429/rate/provider-limit classification and secret non-leakage; slash refs and overlapping-ref ambiguity; root metadata/~5k safe README/full-link/hints; source comments, line/range and heading selectors; binary/100 MB handling; authenticated/private fallback; one-level trees, directory README, and the 1,000-entry provider ceiling. `go test ./...` and `go vet ./...` pass.
- **Regression evidence:** deterministic tests re-prove direct Markdown fallback and unsupported GitHub→Firecrawl fallback with the existing Firecrawl scrape settings unchanged; `make check` passes search/map-site compile/tests plus npm shim/postinstall checks. The old public raw blob path remains zero-REST in its deterministic test.
- **Live evidence:** forced-anonymous public reads succeeded for `https://github.com/amxv/webctx`, its `README.md#install`, `README.md#L1-L12`, and `tree/main/internal/app`; `https://github.com/cli/cli/blob/andyfeller/test/README.md` succeeded with a slash-containing ref, and `https://github.com/cli/cli/tree/andyfeller/test` provider-resolved exactly `ref: "andyfeller/test"` and returned the branch root listing. GitHub's current first-party docs were re-checked for REST versioning, auth/404 behavior, rate-limit headers/status, Link pagination, Contents 1,000-entry ceiling, and 100 MB source limits.
- **Documentation:** README plus read-link, credentials, architecture, agent-workflows, CLI reference, quickstart, troubleshooting, `.env.local.example`, and landing-page copy now describe only Phase 1 native GitHub behavior. `npm run docs:check` completed with zero errors (seven pre-existing Astro `z` deprecation hints), `npm run docs:build` passed, and the current Vercel docs-ignore tests passed 6/6.
- **Decisions made:** public blobs keep the cheapest raw-host read over the complete unresolved tail; provider-backed ref/path resolution is used where identity is required (tree and authenticated/private fallback), and multiple provider-valid splits fail as ambiguous. Human-view README sanitization preserves fenced code while direct blob source is never comment-sanitized. Root hints advertise only native capabilities actually implemented on the Phase 1 branch, so Issues/PRs are intentionally deferred to Phase 2.
- **Amendments:** none; no load-bearing Phase 1 plan assumption was disproved. The planned A1 heading-fragment caveat remains: the implemented deterministic GitHub-compatible resolver supports ordinary ATX headings and duplicate suffixes and fails unknown selectors rather than guessing.
- **Known defects/risks:** no known Phase 1 correctness blocker. Exotic Markdown heading constructs outside the proven A1 subset remain intentionally unsupported rather than approximately selected. GitHub provider caps/rate limits still apply and are surfaced instead of cached or hidden.
- **Next handoff:** Phase 2 should inspect `internal/app/github.go` first, especially `GitHubTarget`, `GitHubClient`, `GitHubNativeResult`, `ParseGitHubLinkHeader`, error classification, hint rendering, and the source ref resolver; then implement the Phase 2 Issue/list/conversation responsibilities without broadening native classification beyond faithfully supported routes.

### 2026-08-14 — Phase 2 — `complete`

- **Agent/session:** GPT-5.6 Sol continuation session on the Zodex checkout.
- **Starting state:** `main` was clean and exactly matched `origin/main` at `b471277d203d920d88f21f54a08cee5455079626`; Phase 1 was re-read from current code rather than trusted from the ledger.
- **Ending commit(s):** `7bad0c3ef652560af1dcd7636dfbf0c6b5cc75bb` — Phase 2 implementation, tests, public docs, and initial durable handoff.
- **Outcome:** repository Issue detail URLs now return native compact metadata, human-visible bodies, every substantive paginated timeline/comment event, current parent/sub-issue/dependency/field relationships, and contextual navigation. `#issuecomment-<id>` performs one targeted provider read. Repository Issue/search, label, and milestone routes are native bounded views that preserve useful filters/page state and exclude Pull Requests returned through GitHub's Issues/Search APIs.
- **Files/areas changed:** `internal/app/github.go` gained Issue/list/label/milestone target kinds plus shared provider-Link pagination and HTTP 410 classification; new `internal/app/github_issues.go` owns Phase 2 Issue semantics; new `internal/app/github_issues_test.go` supplies deterministic fixtures; Phase 1 fallback tests were retargeted from now-native Issues to an unsupported Wiki route; README/docs/landing copy was synchronized to delivered behavior.
- **Positive evidence:** deterministic tests prove multi-page Issue timeline completion through provider `Link`, bot/non-maintainer retention, invisible human-body comment stripping, empty/deleted/minimized/pinned states, state reason/type/labels/assignees/milestone, parent/sub-issue/blocked-by/blocking/field values, cross-reference and timeline state projection, one-request exact comment selection/ownership validation, PR exclusion from repository and Search issue lists, bounded UI navigation, `incomplete_results`, label/milestone detail/list boundedness, search-specific rate-resource reporting, optional relationship 404/410 handling, native 403/404/429 failures, pagination-cycle rejection, and no private/token leakage. `go test ./...`, `go vet ./...`, formatting, and `git diff --check` pass.
- **Regression evidence:** existing Phase 1 root/blob/tree/selectors remain green deterministically and live; the unsupported-GitHub→Firecrawl regression now uses a Wiki route and still asserts the existing Firecrawl scrape settings unchanged. `make check` passes search/map-site and npm shim/postinstall validation.
- **Live evidence:** forced-anonymous reads succeeded for `amxv/webctx#6`, `/issues?state=all&page=1`, `/issues?q=is:issue`, `/labels`, and `/milestones`; the Issue list correctly reduced the provider's mixed Issue/PR response to only Issues #6 and #3. `cli/cli#14134` rendered its substantive bot comment in the full conversation, and `#issuecomment-5261879950` returned that exact comment directly. Live optional-relationship probes on `amxv/webctx#6` showed parent/issue-field-values absent as 404 while sub-issue and both dependency lists returned empty 200 arrays, matching the implemented optional-boundary behavior.
- **Documentation:** README plus read-link, credentials, quickstart, CLI reference, troubleshooting, architecture, agent-workflows, and landing copy now describe native Issues/comment selectors and bounded Issue/search/label/milestone views only. `npm run docs:check` completed with zero errors (the same seven existing Astro `z` deprecation hints), `npm run docs:build` passed, and Vercel docs-ignore tests passed 6/6.
- **Decisions made:** the Issue timeline endpoint is the canonical selected-conversation stream so normal comments are not separately fetched and duplicated; exact comment anchors use the one-comment endpoint. Optional relationship endpoints suppress only provider 404/410 absence, while auth/rate/provider failures remain authoritative. Search URLs inject repository and `is:issue` qualifiers server-side while preserving the user's original GitHub UI query in output/navigation. Unknown timeline event kinds are retained generically unless they are an explicit bookkeeping-noise allowlist.
- **Amendments:** none; current first-party GitHub behavior supported the Phase 2 plan assumptions.
- **Known defects/risks:** GitHub only exposes newer Issue relationship/type/field features when the repository/account has them; absent optional relationship endpoints are omitted rather than fabricated. Exact minimized/deleted state availability depends on what REST exposes for that public/auth context. No Phase 2 correctness blocker is known.
- **Next handoff:** Phase 3 should inspect `internal/app/github.go` target/client/result pagination, `internal/app/github_issues.go` shared Issue identity/comment/timeline types and sanitizer behavior, then add Pull Request conversation/review/thread selectors without consuming `/pull/<n>/files`, `/commits`, or `/checks`, which belong to Phase 4.

### 2026-08-14 — Phase 3 — `complete`

- **Agent/session:** GPT-5.6 Sol continuation session on the Zodex checkout.
- **Starting state:** `main` was clean and exactly matched `origin/main` at `858a0dbf7fa4bddb3d476587e415139fca23ea03`; Phase 2's Issue model/client/result boundary was inspected before adding PR-specific responsibilities.
- **Ending commit(s):** `e953217b9dceeeba0c7208579a5b8c5f16615e68` — Phase 3 implementation, tests, public docs, and initial durable handoff.
- **Outcome:** `/pull/<n>` now returns a native PR conversation with compact PR/base/head/change metadata, human-visible body, normal Issue-style comments and meaningful timeline transitions, complete submitted reviews, and inline REST review comments grouped into coherent root/reply threads. Exact `#issuecomment-`, `#discussion_r`, and `#pullrequestreview-` selectors narrow to normal comments, one inline thread, or one formal review. Public PRs remain anonymous-capable; configured GitHub auth optionally enriches threads with GraphQL resolved/outdated truth without making GraphQL mandatory.
- **Files/areas changed:** `internal/app/github.go` gained PR conversation routing plus a reusable authenticated GraphQL request boundary; `internal/app/github_issues.go` extended shared timeline/comment identity for PR event fields and PR-aware normal-comment rendering; new `internal/app/github_pulls.go` owns Phase 3 PR/review/thread/selector semantics; new `github_pulls_test.go` supplies deterministic REST/GraphQL fixtures; README/docs/landing/root hints were synchronized while focused PR tabs remain separate for Phase 4.
- **Positive evidence:** deterministic fixtures prove multi-page PR timeline completion, review/timeline de-duplication, empty/full formal reviews, bot/non-member preservation, cross-review reply chains grouped under their root, multiple paths/line/range coordinates, force-push/base-change/commit/ready-for-review/user-and-team-review-request state events, invisible body/comment sanitization, PR draft/open/closed/merged metadata, all three exact anchor classes, selector ownership validation, GraphQL auth-only semantics, provider-safe GraphQL errors, GraphQL pagination and resolved/outdated/resolver state enrichment, and graceful GraphQL failure with complete REST content preserved. `go test ./...`, `go vet ./...`, formatting, and `git diff --check` pass.
- **Regression evidence:** Phase 1 root/source/tree tests and Phase 2 Issue/list/comment tests remain green; the root hint test was advanced from its old Phase 1 prohibition to require the now-native Issue/PR targets while retaining a five-hint orientation budget. Focused `/pull/<n>/files`, `/commits`, and `/checks` routes remain explicitly unsupported by native classification until Phase 4. `make check` passes search/map-site plus npm shim/postinstall validation.
- **Live evidence:** forced-anonymous `cli/cli#13250` returned 19 submitted review headings and grouped 28 REST inline review comments into 13 threads while preserving bot reviews and normal Issue comments. Live timeline data exercised force pushes, base-ref changes, commits, ready-for-review, individual/team review requests, and rename behavior. `#discussion_r3118513169` returned that exact thread with all observed replies; `#pullrequestreview-4148860648` returned the selected approval and its review comments; `#issuecomment-4447874096` returned the selected normal PR comment through shared Issue-comment identity. Current GitHub first-party REST docs were rechecked for review/comment relationships, and GraphQL docs were rechecked for `reviewThreads`, `isResolved`, `isOutdated`, and `fullDatabaseId`. No live authenticated GraphQL claim is made because the live validation was deliberately forced anonymous.
- **Documentation:** README plus read-link, credentials, quickstart, CLI reference, troubleshooting, architecture, agent-workflows, and landing copy describe native PR conversations/exact anchors and optional thread-state enrichment. PR `/files`, `/commits`, and `/checks` are described only as focused navigation targets, not as native Phase 3 reads. `npm run docs:check` completed with zero errors (the same seven existing Astro `z` deprecation hints), `npm run docs:build` passed, and Vercel docs-ignore tests passed 6/6.
- **Decisions made:** REST `in_reply_to_id` is the anonymous content-authority for grouping threads; timeline `reviewed` events are intentionally skipped because the complete reviews endpoint owns formal-review text. Exact `discussion_r` reads fetch the selected comment plus the paginated PR review-comment universe only, then render just its thread; exact review reads use the selected review plus that review's comments. GraphQL uses `fullDatabaseId` to map provider thread state back to REST root-comment IDs and never prints provider GraphQL error bodies.
- **Amendments:** none; current first-party GitHub behavior supported the Phase 3 plan assumptions.
- **Known defects/risks:** resolved/outdated thread state remains unavailable on anonymous reads by provider design; the output gives at most one optional-auth hint. Exact formal-review output includes comments submitted with that review, while later replies belonging to other reviews are available through their `discussion_r` thread or the full PR conversation. No Phase 3 correctness blocker is known.
- **Next handoff:** Phase 4 should inspect `GitHubTargetPull`, the PR-specific URL hints and selector parser, `githubPullRequest`/review-comment types, provider-Link pagination, and the existing raw/REST content primitives before adding files/commits/checks ownership. Do not re-expand focused tabs into the Phase 3 conversation renderer.

### 2026-08-14 — Phase 4 — `complete`

- **Agent/session:** GPT-5.6 Sol continuation session on the Zodex checkout.
- **Starting state:** `main` was clean and exactly matched `origin/main` at `8226c20c8be31fc71e3035029074877be5d26f2a`; Phase 3 PR conversation ownership and current GitHub Files Changed/check URL semantics were inspected before claiming focused tabs.
- **Ending commit(s):** pending Phase 4 commit; replace after commit creation.
- **Outcome:** `/pull/<n>/files`, `/commits`, and `/checks` are now distinct native resource views. Files Changed returns paginated provider patches with rename/patch-omission/provider-cap truth and supports GitHub's current SHA-256 file plus L/R line/range fragments. Commits returns only the PR commit series. Checks keeps Check Runs separate from commit statuses, follows pagination for both, supports `check_run_id` targeting with full annotation pagination and head-SHA ownership validation, and explicitly avoids manufacturing a branch-protection verdict. `/pull/<n>.diff` and `.patch` preserve raw provider media.
- **Files/areas changed:** `internal/app/github.go` gained focused PR and raw-media target kinds/routing; new `internal/app/github_pull_views.go` owns Phase 4 files/commits/checks/raw-media semantics; new `github_pull_views_test.go` supplies deterministic fixtures; Phase 3 parser tests advanced to require focused native targets; README/read-link/agent-workflows/architecture/landing copy was synchronized.
- **Positive evidence:** deterministic tests cover current SHA-256 path hashing, hash-only and L/R line/range selector parsing, side-mixing/reversed/stale/out-of-range failures, unified-diff old/new line mapping, multi-page files, text patches, rename metadata, provider-omitted/binary-style patches, explicit 3,000-file incomplete truth, commit pagination without diff/conversation expansion, multi-page Check Runs, paginated combined commit statuses, Check Run/status separation, selected `check_run_id` ownership, multi-page annotations, unrelated-check exclusion, and exact raw diff/patch media/Accept preservation. Recognized focused-route 404s remain native failures with no private response leakage. `go test ./...`, `go vet ./...`, formatting, and `git diff --check` pass.
- **Regression evidence:** the full Phase 1–3 deterministic suite remains green, including `/pull/<n>` conversation de-duplication and `#issuecomment-`/`#discussion_r`/`#pullrequestreview-` ownership. `make check` passes search/map-site and npm shim/postinstall validation. Focused view output tests explicitly prove Files Changed does not include PR conversation text and commits do not dump patches.
- **Live evidence:** forced-anonymous `cli/cli#13250/files` returned all 10 reported files as a native Files Changed view; the known `internal/ghcmd/cmd.go` selector hash `553490f999984ba28c4af0d7ffa919d10b5419f04a73f00141ee0b5a51c142e6` still appeared in current GitHub UI markup and narrowed webctx to that file. Current UI markup also exposed both `L24` and `R24` suffixes for that same hash. `/pull/13250/commits` returned all 3 reported commits natively. Current `/checks` HTML exposed `check_run_id=75849937564`, and direct github.com `.diff`/`.patch` returned their distinct raw forms. After these probes, the host's anonymous core quota was exhausted; webctx `/checks`, `.diff`, and `.patch` API-backed invocations therefore returned the correct native core-rate-limit error with reset/auth-capacity guidance rather than Firecrawl. No GitHub token was configured, so those quota-blocked webctx invocations are not mislabeled as live success; deterministic injected tests are the implementation evidence for them.
- **Documentation:** README plus read-link, agent-workflows, architecture, and landing copy now describe native files/commits/checks, diff selectors, check targeting, provider caps, and raw diff/patch behavior. `npm run docs:check` completed with zero errors (the same seven existing Astro `z` deprecation hints), `npm run docs:build` passed, Vercel docs-ignore tests passed 6/6, and npm dry-run packaging includes the new focused-view source/tests.
- **Decisions made:** provider file identity uses exactly SHA-256 of the filename, matching current GitHub UI anchors; side-specific selectors map unified patch hunks by old (`L`) versus new (`R`) line counters and fail stale lines instead of approximating. A PR reporting/reaching 3,000 changed files is conservatively marked incomplete at GitHub's documented ceiling. Missing `patch` is described as provider-unavailable (binary/oversized/omitted examples) rather than guessed as definitely binary. Combined commit statuses are themselves Link-paginated because GitHub documents `per_page`/`page`. Direct diff/patch uses the Pull Request REST custom media types so private/auth-capable reads share the native error/auth boundary while preserving bytes as text.
- **Amendments:** none; current first-party GitHub behavior supported the Phase 4 plan assumptions.
- **Known defects/risks:** GitHub diff and `check_run_id` URL conventions are UI compatibility facts rather than stable REST schemas, so deterministic compatibility tests and live probes remain important. Anonymous rate exhaustion prevented late live webctx checks/raw-media success evidence in this session; the failure behavior itself was live-proven and deterministic success paths are covered. No Phase 4 correctness blocker is known.
- **Next handoff:** Phase 5 should inspect `github_pull_views.go` diff/raw-media primitives and `github.go` ref/client/pagination/error boundaries, then add commit/compare/path-history/blame responsibilities without coupling them back into PR-focused routes.

When a phase is completed or blocked, append an entry in this exact shape:

```markdown
### YYYY-MM-DD — Phase N — `<status>`

- **Agent/session:** ...
- **Starting state:** branch/worktree actually inspected; SHA may be included as a coordinate.
- **Ending commit(s):** full pushed SHA(s).
- **Outcome:** what became observably true.
- **Files/areas changed:** concise paths and major boundaries.
- **Positive evidence:** phase-specific proof.
- **Regression evidence:** existing behavior re-proven.
- **Live evidence:** real-system evidence, or a concrete reason it was not applicable.
- **Documentation:** help/docs/generated-contract work and validation, or a concrete not-applicable reason.
- **Decisions made:** local implementation judgments that do not amend the plan.
- **Amendments:** plan Amendment link/reference or `none`.
- **Known defects/risks:** exact remaining problems.
- **Next handoff:** earliest incomplete boundary and what the next agent should inspect first.
```
