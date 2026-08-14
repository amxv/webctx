# GitHub Read-Link Context Hardening Progress

## Workstream

- **Plan:** [github-read-link-context-hardening-implementation-plan-2026-08-14.md](./github-read-link-context-hardening-implementation-plan-2026-08-14.md)
- **Sweep:** [github-read-link-context-hardening-sweep-2026-08-14.md](./github-read-link-context-hardening-sweep-2026-08-14.md)
- **First implementation prompt:** [first-agent-prompt.md](./first-agent-prompt.md)
- **Continuation prompt:** [subsequent-agent-prompt.md](./subsequent-agent-prompt.md)

Treat current source, tests, generated contracts, and docs as truth. Progress claims are evidence to verify, never proof.

This ledger belongs only to the `github-read-link-context-hardening` workstream. Do not merge unrelated repository work or historical planning into it.

## Status vocabulary

- `pending`
- `in_progress`
- `blocked`
- `complete`

A phase is `complete` only after its observable outcome, phase-specific positive evidence, relevant regression evidence, applicable live evidence, docs/generated-contract work, coherent commit, push, remote verification, and ledger update are complete.

## Phase table

| Phase | Capability | Status | Completion commit | Evidence / next boundary |
| ---: | --- | --- | --- | --- |
| 1 | Shared bounded-context contract and resilient Issues | complete | `68db77888bc136f97086aad62d4b844bb4d25159` | Large Issue roots are bounded/adaptive, `minimized` shape drift is tolerant, and exact `#issue-*` body reads are proven live. |
| 2 | Native Pull Request lists and PR-qualified search correctness | complete | `8d85e3cdf5629fb36bc1f8eb48031282e2e8bf2b` | `/pulls` is native and bounded; PR-qualified `/issues?q=...` preserves PR semantics instead of being rewritten as Issues. |
| 3 | Bounded PR root and complete selector discoverability | complete | `b34af6b9d61affb1ed6f0364a61f38cf443891a8` | PR roots are bounded maps with Issue-side body selectors and exact comment/review/thread drill-downs. |
| 4 | PR Files and Checks bounded subviews | complete | `714da077dac892b9eda358f31e1ef2637813d1d2` | Large patch/check fan-out is index-first; focused diff/check selectors remain scoped. |
| 5 | Actions overview/run/job context safety and raw-log navigation | complete | `44048c280b49554888b02d3252178194a55009f2` | Actions/run/job views are bounded; failures are prioritized and full log access is explicit rather than inline. |
| 6 | Commit and compare overview/raw split plus commit selectors | complete | `d793eadd2dbbe1f4596819d690b1084f911bf237` | Plain commit/compare are bounded indexes; raw diff/patch and exact commit file/comment selectors remain explicit deep paths. |
| 7 | Releases, trees, statistics, and deployment fan-out hardening | complete | `cbe885227270cf38bd22c532a9c8f960508cd8fd` | Release detail, large trees, contributor statistics and deployment environments now use bounded/index-first output without chasing subordinate provider history. |
| 8 | Discussions and Gists bounded navigation plus exact comment selectors | complete | `3d7bd224fcdc7849e1c7a7db4dd7245745227e4a` | Discussion/Gist roots are index-first and bounded; copied provider comment/file selectors are exact reads. |
| 9 | Cross-family route audit, hints, docs, and release-facing contract | pending | — | Audit every target kind and document the final overview/exact/raw mental model. |
| 10 | Independent acceptance and live stress matrix | pending | — | Re-review all 143 criteria against current code and live/deterministic evidence. |

## Current handoff

- **Last completed phase:** Phase 8 — `Discussions and Gists bounded navigation plus exact comment selectors`.
- **Earliest incomplete phase:** Phase 9 — `Cross-family route audit, hints, docs, and release-facing contract`.
- **Observable boundary:** Discussion roots make one bounded GraphQL request for the main body plus the first 30 top-level comments / first 5 replies per returned comment, preserve accepted-answer identity and canonical `#discussioncomment-<databaseId>` links, and stop instead of traversing the whole conversation; an exact copied Discussion comment/reply selector may page only as needed to find that one identity. Gist roots make one bounded comment-page request and render file/comment/revision indexes without raw-file fan-out; exact `#file-*` reads remain full/narrow and proven `#gistcomment-<id>` selectors use GitHub's single-comment endpoint. Ownerless copied hash-ID Gist URLs are recognized without reclassifying ordinary Gist profile paths.
- **Current blockers:** none known. GitHub/provider behavior listed as assumptions in the plan must be verified at the phase that depends on it.
- **Plan Amendments affecting next phase:** none.
- **Prompt to use now:** `subsequent-agent-prompt.md`.

## Planning baseline evidence

Planning inspected `main` at `d541d1a24622d9b92fd1fbd87b5af5cc69370389` (`v0.2.0`). This is a coordinate only; implementation must fetch and use current remote code.

At planning time:

- `go test ./...` passed;
- `go vet ./...` passed;
- `npm test` passed;
- the native binary built successfully;
- live provider credentials in the local `.env.local` allowed end-to-end GitHub/search/read-link/map-site probes;
- no product code was changed by planning.

Representative black-box baselines to beat are recorded in the Sweep, including approximately 46k PR root, 30k Checks, 40k Actions run, 150–217k Actions jobs, 869k plain compare, and the Issue timeline polymorphic-decode failure.

## Progress-entry format

Append one entry after every completed or blocked phase:

```markdown
### YYYY-MM-DD — Phase N — `<status>`

- **Agent/session:** ...
- **Starting state:** branch/worktree actually inspected; SHA may be included as a coordinate.
- **Ending commit(s):** full pushed SHA(s).
- **Outcome:** what became observably true.
- **Files/areas changed:** concise paths and major boundaries.
- **Positive evidence:** phase-specific proof whose result changed because of the phase.
- **Regression evidence:** existing behavior re-proven.
- **Live evidence:** real GitHub/provider evidence, or a concrete reason it was not applicable.
- **Documentation:** help/docs/generated-contract work and validation, or a concrete not-applicable reason.
- **Decisions made:** local implementation judgments that do not amend the plan.
- **Amendments:** plan Amendment link/reference or `none`.
- **Known defects/risks:** exact remaining problems.
- **Next handoff:** earliest incomplete boundary and what the next agent should inspect first.
```

Never place credentials, private live content, tokens, raw private provider payloads, or local secret values in this ledger.

### 2026-08-14 — Phase 1 — `complete`

- **Agent/session:** ChatGPT Atlas implementation session on the existing `/workspace/repos/webctx` checkout.
- **Starting state:** clean `main` at `7f3a294f1aef96c7c65070c932fce77ac3972297`, already fast-forwarded to `origin/main` before edits.
- **Ending commit(s):** `68db77888bc136f97086aad62d4b844bb4d25159` (implementation and user-facing Issue docs); this ledger update follows as the administrative handoff commit.
- **Outcome:** GitHub Issue timeline decoding now tolerates boolean, null, and object-shaped `minimized` data; exact `#issue-<Issue id>` reads verify identity and return only the complete selected description; small Issue roots retain the prior readable conversation while large roots switch to a deterministic ~5k-rune overview with body/comment previews, relationship/pinned facts, exact selector URLs, and distinct local-omission/provider-incomplete truth.
- **Files/areas changed:** shared GitHub overview helpers in `internal/app/github.go`; Issue decode/select/render behavior and fixtures in `internal/app/github_issues.go` / `internal/app/github_issues_test.go`; public Issue navigation guidance in `src/content/docs/read-link.md`.
- **Positive evidence:** deterministic `minimized` fixtures cover false/null/true and object `{"reason":"spam"}` plus nearby unknown shapes; exact Issue-body tests prove one-provider-read selection and mismatch rejection; the synthetic pathological Issue renders at 4,134 Unicode runes with safe body/comment previews, selectors, pinned/relationship facts, and truthful omission; the existing small complete-conversation fixture remains in full form.
- **Regression evidence:** `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check` all passed; existing Issue comment ownership, relationship/pinned/noise filtering, repository/source, client/error, and non-GitHub fallback tests remain green as part of the repository suite.
- **Live evidence:** built `webctx` successfully read `https://github.com/cli/cli/issues/326` in 4,315 runes instead of failing decode; the current public timeline still exposed 34 null and 8 object-shaped minimized values with object reason `spam`; the returned overview exposed `#issue-561414098` and exact comment selectors; reading `#issue-561414098` returned only the description in 1,379 runes with no timeline expansion.
- **Documentation:** `src/content/docs/read-link.md` now documents adaptive Issue roots and the exact `#issue-<issue-id>` description path; no later-phase behavior was documented early.
- **Decisions made:** kept Issue timeline pagination complete in this first vertical slice to preserve the existing Issue conversation/provider contract; boundedness is applied at rendering while later phases address high-amplification container pagination where the plan explicitly requires it.
- **Amendments:** none.
- **Known defects/risks:** `/pulls` is still non-native and PR-qualified `/issues?q=...` is still handled as an Issue search; those are intentionally Phase 2. Other PR/Actions/compare context growth remains in later phases.
- **Next handoff:** Phase 2 — read `subsequent-agent-prompt.md`, reconcile current remote state, then inspect only the Phase 2 plan/sweep/source boundaries for native `/pulls` and PR-qualified search correctness.

### 2026-08-14 — Phase 2 — `complete`

- **Agent/session:** continuation in the same ChatGPT Atlas implementation session on `/workspace/repos/webctx`.
- **Starting state:** clean synchronized `main` at `5bd3fb8d0a382dcf22ddc5a3b1c319a2513a3ce9` after the Phase 1 remote verification.
- **Ending commit(s):** `8d85e3cdf5629fb36bc1f8eb48031282e2e8bf2b` (implementation/tests/public docs); this ledger handoff follows as a separate administrative commit.
- **Outcome:** `/owner/repo/pulls` is now a first-class native target backed by the List Pull Requests REST endpoint; supported state/head/base/sort/direction/page intent is validated and preserved, PR rows expose state/draft/author/time/canonical URLs without N+1 enrichment, and provider Link pagination becomes copied GitHub web navigation. Query-mode `/pulls?q=...` and `/issues?q=is:pr...` now use repository-scoped Search Issues with truthful `is:pr` semantics instead of silently appending `is:issue` and filtering the intended results away.
- **Files/areas changed:** GitHub target/dispatch in `internal/app/github.go`; token-aware Issue-vs-PR search classification in `internal/app/github_issues.go`; native PR list/search/query validation/rendering in `internal/app/github_pulls.go`; Phase 2 parser/list/search/regression coverage in the corresponding tests plus the route-fidelity acceptance matrix; public examples in `src/content/docs/read-link.md`.
- **Positive evidence:** deterministic PR-list fixtures prove one provider call, open/draft/closed identity, exact filter forwarding and Previous/Next reconstruction; deterministic search fixtures prove both `/pulls?q=...` and `/issues?q=is:pr...` generate repository-scoped `is:pr` provider queries and render canonical PR URLs; token-aware qualifier fixtures do not misclassify quoted/textual `is:pr`; explicit `is:issue` remains an Issue search; conflicting resource qualifiers and unsupported PR query parameters/values fail before any provider call.
- **Regression evidence:** existing PR detail/selectors, generic GitHub Search `type=pullrequests`, Issue lists/search, native route exclusions, provider error behavior and repository-wide tests remain green. Final validation passed `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check`.
- **Live evidence:** built `webctx` read `vercel/next.js/pulls` natively in 4,961 runes / 43 lines and page 2 in 5,059 runes / 44 lines; the copied PR search `vercel/next.js/issues?q=is:pr+is:open` rendered `view: pull_requests` in 5,233 runes. No `Uh oh!`, ProTip, sign-in, or loading-placeholder UI chrome appeared; text containing the word “loading” in an earlier probe was confirmed to be legitimate current PR titles/branch names rather than scrape artifacts.
- **Documentation:** `read-link` now teaches `/pulls`, query-mode PR lists, and the fact that copied `/issues?q=is:pr...` links retain PR semantics; no Phase 3 PR-root selector promises were documented early.
- **Decisions made:** native PR containers own a fixed compact provider page size of 30 rather than accepting UI `per_page` overrides, preventing a copied page-size knob from defeating the shared bounded-container goal; official GitHub REST state/head/base/sort/direction/page behavior remains forwarded. Search result ceilings are surfaced separately from current-page size.
- **Amendments:** none; GitHub's current first-party Pull Requests REST documentation still supports the planned list filters and pagination contract.
- **Known defects/risks:** PR root still expands complete conversation, reviews and inline threads and does not yet expose an exact `#issue-<Issue-side id>` description selector; that is the intentional Phase 3 boundary. Later PR Files/Checks/Actions/compare fan-out remains untouched.
- **Next handoff:** Phase 3 — start from `subsequent-agent-prompt.md`, reconcile remote state, then read only the Phase 3 plan/sweep/source surfaces for the always-bounded PR root and selector map.

### 2026-08-14 — Phase 3 — `complete`

- **Agent/session:** continuation in the same ChatGPT Atlas implementation session on `/workspace/repos/webctx`.
- **Starting state:** clean synchronized `main` at `3110c39f2ff0b1ea5539d62ab3205b32153765fd` after the Phase 2 remote verification.
- **Ending commit(s):** `b34af6b9d61affb1ed6f0364a61f38cf443891a8` (implementation/tests/public docs); this ledger handoff follows as a separate administrative commit.
- **Outcome:** plain PR roots are no longer complete transcripts. They use PR REST metadata as the state/branch/stat authority, fetch the Issue-side PR object to obtain the distinct canonical `#issue-<Issue-side id>` description anchor, fetch only one REST provider page of timeline/reviews/review-comments for the overview, and render deterministic description/comment/review/thread previews plus exact selectors and `/files` `/commits` `/checks` navigation. Exact `#issue-*`, `#issuecomment-*`, `#pullrequestreview-*`, and `#discussion_r*` reads remain full scoped semantic reads.
- **Files/areas changed:** PR identity/fetch/render/selector logic in `internal/app/github_pulls.go`; converted bounded-root and new pathological/Issue-side selector coverage in `internal/app/github_pulls_test.go`; public PR overview/navigation wording in `src/content/docs/read-link.md`.
- **Positive evidence:** the converted anonymous PR fixture proves distinct PR REST id `123456789` versus Issue-side id `987654321`, uses only the latter in `#issue-*`, preserves state/base/head/stats and all first-page substantive event semantics, exposes stable comment/review/thread IDs plus exact selectors, keeps bot content visible, omits reply bodies from the root thread index, reports provider-more truth separately, never calls GraphQL without auth, and stays under the shared 5k-rune target. A synthetic large body + 20 comments + 12 reviews + 12 threads stays <=5,000 runes, safely closes Markdown fences, locally omits child index entries deterministically, and verifies every emitted child has a matching selector URL. Exact PR-body selection uses one Issue-side provider read and rejects mismatched IDs.
- **Regression evidence:** exact selected review still includes its review comments; exact selected thread still reconstructs root/replies; exact ordinary comment remains shared with Issue semantics; GraphQL resolved/outdated enrichment remains visible when available and non-fatal when unavailable; invisible GitHub HTML comments remain stripped. Final validation passed `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check`.
- **Live evidence:** built `webctx` read `vercel/next.js#97343` in 2,469 runes / 85 lines (planning baseline ~46k) and `cli/cli#13250` in 4,583 runes / 122 lines (planning baseline ~21.8k). Next.js exposed Issue-side body id `5146963444` and two exact ordinary-comment selectors; CLI exposed Issue-side body id `4301913599` plus exact review/thread selectors. Following the printed Next.js body selector returned only the 304-rune full description; following a printed large Next.js automation-comment selector returned the selected ~30,381-rune comment only; following CLI review/thread selectors returned scoped 2,418-rune and 1,008-rune reads respectively. This proves large child bodies are opt-in rather than root-injected.
- **Documentation:** `read-link` now describes PR roots as compact overviews, explains the printed `#issue-*` full-description path, and states that copied comment/review/thread anchors remain exact scoped reads.
- **Decisions made:** root timeline/review/review-comment REST collections stop after their first 100-item provider page and surface a provider-more flag rather than eagerly chasing pagination; local index limits adapt downward until the shared output target fits. Existing authenticated GraphQL thread-state enrichment remains optional and non-fatal because it adds state only, not subordinate text.
- **Amendments:** none.
- **Known defects/risks:** `/pull/<n>/files` can still render every returned patch up to GitHub's provider ceiling and `/checks` can still expand large check-run fan-out/annotation context. Those are intentional Phase 4 targets; `/commits` remains compact enough to preserve as-is unless Phase 4 evidence disproves that assumption.
- **Next handoff:** Phase 4 — start from `subsequent-agent-prompt.md`, reconcile remote state, then inspect only the PR Files/Checks plan, sweep, source, tests, and current provider docs needed for their bounded overview/focused-selector contract.

### 2026-08-14 — Phase 4 — `complete`

- **Agent/session:** continuation in the same ChatGPT Atlas implementation session on `/workspace/repos/webctx`.
- **Starting state:** clean synchronized `main` at `1d7e2ac680bb77d7ef0c4883fb6752f5bc33fee6` after Phase 3 remote verification.
- **Ending commit(s):** `714da077dac892b9eda358f31e1ef2637813d1d2` (implementation/tests/public docs); this ledger handoff follows separately.
- **Outcome:** unselected PR Files stops after one 100-file provider page; a naturally small complete set retains full patches, while larger/incomplete sets become bounded file indexes with status/change facts, SHA-256(path) selectors, blob/raw URLs, limited patch previews, and distinct local/provider/cap truth. Exact diff selectors still traverse complete provider pages to locate and return the selected patch/hunk. Checks retain complete run metadata so failures on later pages can be prioritized, but render only rollups plus a bounded failure/active-first index with `?check_run_id=` for every indexed run. Focused checks preserve head ownership and annotation coordinates while bounding generated summary/message/raw-details/annotation output and linking provider Details URLs.
- **Files/areas changed:** `internal/app/github_pull_views.go`, Phase 4 fixtures in `internal/app/github_pull_views_test.go`, and PR Files/Checks guidance in `src/content/docs/read-link.md`.
- **Positive evidence:** deterministic one-page Files evidence proves no eager overview pagination; small complete Files preserves full patches; synthetic 100-file/provider-more and 3,000-file ceiling fixtures remain <=5k runes while distinguishing provider-more/provider-ceiling from local omission. A synthetic 130-run Checks set remains <=5k, orders a hard failure before an active run before successes, exposes a focused URL for every indexed run, keeps combined statuses separately represented, and excludes machine summaries. A synthetic focused check with 40 huge annotations / reported 50 remains <=5k while preserving coordinates/message previews and separating summary/raw/local/provider truncation truth.
- **Regression evidence:** PR `/commits`, raw `.diff`/`.patch`, diff hash identity, left/right hunk semantics, exact file selection, selected-check ownership mismatch, combined commit statuses, and small focused-check annotation rendering remain green. Current first-party GitHub Checks docs still specify up to 100 results per page for check-run/annotation lists under API version `2026-03-10`. citeturn299283search0 Final validation passed `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check`.
- **Live evidence:** built `webctx` reduced `vercel/next.js#97343/checks` from the planning ~30.5k baseline to 4,893 runes / 133 lines while retaining metadata for all 132 runs, indexing 13 and locally omitting 119; all 13 indexed runs exposed focused URLs and the first three were current failures. The prescribed failing check `94639361056` rendered in 819 runes with its annotation and provider Details links. The same PR's two-file Files view retained the complete 3,481-rune patch form. The live `cli/cli#13250` `internal/ghcmd/cmd.go` exact diff selector returned one scoped full patch in 3,550 runes with `files_returned: 10`, `files_rendered: 1`, and no overview conversion.
- **Documentation:** PR docs now explain small-vs-large Files behavior, exact diff narrowing, Checks rollups/priority, focused-run URLs, and bounded focused machine detail.
- **Decisions made:** Files overview pagination is deliberately capped at the first provider page; exact diff selection is the escape hatch that may traverse all file pages. Checks deliberately retain complete run metadata pagination because otherwise a failure on a later page could be hidden behind routine successes; only rendered child count and generated text are budgeted.
- **Amendments:** none.
- **Known defects/risks:** Actions overview/run/job still enumerate large child collections and selected jobs can emit massive full raw logs; Phase 5 owns that boundary.
- **Next handoff:** Phase 5 — reconcile remote state via `subsequent-agent-prompt.md`, then inspect only the Actions overview/run/job plan, sweep, source/tests/docs, and current provider log/redirect semantics.

### 2026-08-14 — Phase 5 — `complete`

- **Agent/session:** continuation in the same ChatGPT Atlas implementation session on `/workspace/repos/webctx`.
- **Starting state:** clean synchronized `main` at `676149d5ae5e3c93dedc8121ec0a18bfcf45e069` after Phase 4 remote verification.
- **Ending commit(s):** `44048c280b49554888b02d3252178194a55009f2` (implementation/tests/public docs); this ledger handoff follows separately.
- **Outcome:** Actions root/workflow listings are adaptive bounded indexes; the root explicitly links the full workflow collection instead of duplicating both complete workflow/run lists. Run detail fully paginates latest-attempt job metadata so failures on later pages remain discoverable, but fetches only the first artifact page, surfaces job/artifact reported/returned/indexed/local/provider truth, and prioritizes failure then active then routine job rows. Selected jobs always retain structured step state while logs become adaptive bounded previews: failed jobs favor failed-step/error context plus terminal context, other large logs use deterministic head/tail. Every job exposes the stable GitHub job-log API endpoint; webctx intentionally does not print the signed redirect location, whose provider-documented lifetime is one minute.
- **Files/areas changed:** `internal/app/github_actions.go`, Phase 5 deterministic/live regression coverage in `internal/app/github_actions_test.go`, and Actions/log guidance in `src/content/docs/read-link.md` plus `src/content/docs/troubleshooting.md`.
- **Positive evidence:** synthetic 30-workflow/30-run root and workflow lists stay <=5k while surfacing local omission; a synthetic 130-job/100-returned-of-133-artifact run stays <=5k, preserves exact job/artifact identities, and orders hard failure before active before success; the run fixture proves jobs still follow provider pagination while artifacts stop after one page. A multi-thousand-line failed job retains the failed structured step, error/exit context and terminal tail while staying <=5k and exposing raw-log navigation; a large successful log deterministically keeps head/tail. Small logs remain full, ZIP logs still decode, malformed/non-text archives still error truthfully, and 410 remains a truthful unavailable-log state.
- **Regression evidence:** job/run ownership validation, unproven step-fragment rejection, workflow/root query/page semantics, provider-native errors, and redirect Authorization stripping remain green. Current first-party GitHub Workflow Jobs documentation still defines `GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs` as a `302` redirect to a plain-text download URL that expires after one minute; public resources can use the endpoint without authentication, while private fine-grained access requires Actions read permission. Final validation passed `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check`.
- **Live evidence:** built `webctx` reduced `vercel/next.js/actions` to 3,234 runes / 50 lines while indexing 6 of 30 returned workflows and 14 of 30 returned runs and linking `/actions/workflows`. Run `31757053478` dropped from the planning ~40k baseline to 4,931 runes / 69 lines while retaining all 102 job metadata, indexing 18, fetching 100 of 133 artifacts and indexing 3; the first three indexed jobs were current failures. Previously ~217k job `94635412218` now renders in 4,953 runes and ~153k job `94635017814` in 4,904, both with all structured steps, bounded head/tail log previews, stable API log endpoints, explicit one-minute expiry semantics, and no signed storage query URL in output.
- **Documentation:** `read-link` now describes Actions run rollups/indexing, bounded selected-job logs, failure-vs-head/tail preview semantics, and stable raw-log navigation; troubleshooting distinguishes unavailable logs from intentional previewing.
- **Decisions made:** complete job metadata pagination is retained because hiding a later-page failed job would make the overview semantically wrong; artifact expansion is capped at the first provider page because artifact identity/detail fan-out is not required for failure diagnosis. Signed storage redirect URLs are deliberately never rendered, because the stable API endpoint is reproducible and the redirect itself is both ephemeral and credential-like.
- **Amendments:** none.
- **Known defects/risks:** plain commit and compare readers can still inject large patch/message/file bodies; Phase 6 owns that split. Actions log fetching still has the repository-wide 100MB direct-read ceiling before rendering, but ordinary semantic output is now bounded.
- **Next handoff:** Phase 6 — start from `subsequent-agent-prompt.md`, reconcile remote state, then inspect only commit/compare plan/sweep/source/tests/docs and current provider semantics for bounded overview versus raw diff/patch plus exact commit selectors.

### 2026-08-14 — Phase 6 — `complete`

- **Agent/session:** continuation in the same ChatGPT Atlas implementation session on `/workspace/repos/webctx`.
- **Starting state:** clean synchronized `main` at `ce18271500a3097b7cc7f4a73b2026bd82b4b85a` after Phase 5 remote verification.
- **Ending commit(s):** `d793eadd2dbbe1f4596819d690b1084f911bf237` (implementation/tests/public docs); this ledger handoff follows separately.
- **Outcome:** plain commit roots keep the complete commit message, authorship/committer/verification/stats/parents, but stop after one provider page of changed files and comments and render bounded identity-rich indexes instead of patch/comment bodies. Changed-file rows expose GitHub-compatible SHA-256(path) `#diff-*` selectors plus blob/raw links; commit comments expose `#commitcomment-*`. Exact diff selectors may paginate the commit API to find one requested path and optional left/right hunk range, while exact commit comments use the repository-scoped single-comment endpoint and verify commit ownership. Plain compare roots stop after the first provider page, index commits/files without patches, keep the distinct 300-file provider ceiling truthful, and print explicit raw `.diff`/`.patch` representations.
- **Files/areas changed:** commit/compare fetch, selector, renderer, and raw-navigation logic in `internal/app/github_commits.go`; deterministic pathological/provider/selectors coverage in `internal/app/github_commits_test.go`; commit/compare public mental model in `src/content/docs/read-link.md`.
- **Positive evidence:** deterministic commit fixtures prove plain roots do not follow file/comment pagination, preserve the full commit message, keep provider-more distinct from local omission, never inject file patch bodies, and stay <=5k for 100-file/100-comment and 3,000-file-cap cases. Exact commit-comment selection uses one provider request for canonical hex commit URLs, preserves the selected full body/location, strips invisible HTML comments, and rejects mismatched commit identity. Exact file and `R10-R10` line selectors deliberately traverse provider pages and return only the selected file/hunk. Compare fixtures prove plain overview pagination stops after page one, 300-file/provider-incomplete truth remains explicit, patches are omitted, and large compare metadata stays bounded.
- **Regression evidence:** raw commit/compare `.diff`/`.patch`, slash-containing compare refs, history/blame, PR raw diff/patch and shared diff hash/hunk semantics remain green as part of the repository suite. Current first-party GitHub docs still document commit changed-file pagination up to 3,000 files and a canonical single commit-comment endpoint. Final validation passed `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check`.
- **Live evidence:** built `webctx` read `vercel/next.js/compare/v16.3.1-canary.15...v16.3.1-canary.16` in 4,705 runes / 71 lines versus the sweep's ~869k baseline, retaining all 13 returned commits and indexing 4 of 172 changed files. `commit/v16.3.1-canary.16` resolved to `c77d3f45a5b99f554d37be15cc12b96e269b4326` and rendered in 4,584 runes with 7 of 22 files indexed and canonical exact diff selectors. Following the first selector returned only `lerna.json` with its complete provider patch in 891 runes. Following the compare's explicit raw paths produced complete 1,922,503-byte `.diff` and 2,032,190-byte `.patch` outputs, proving raw change bodies remain opt-in rather than ordinary context.
- **Documentation:** `read-link` now explains bounded commit roots, exact file/hunk/comment anchors, raw commit `.diff`/`.patch`, and the same overview/raw distinction for compares.
- **Decisions made:** exact selectors are allowed to spend provider pagination because the user has selected one resource; ordinary commit/compare containers are not. Canonical hexadecimal commit URLs can validate a selected comment's `commit_id` directly without an extra identity fetch; symbolic refs resolve the commit first before accepting ownership.
- **Amendments:** none.
- **Known defects/risks:** release bodies/assets, recursive trees, statistics, and deployment/status collections can still amplify large provider child sets; Phase 7 owns those families.
- **Next handoff:** Phase 7 — start from `subsequent-agent-prompt.md`, reconcile remote state, then inspect only Releases, trees, statistics, deployment plan/sweep/source/tests/docs plus current first-party provider semantics needed for bounded fan-out.

### 2026-08-14 — Phase 7 — `complete`

- **Agent/session:** continuation in the same ChatGPT Atlas implementation session on `/workspace/repos/webctx`.
- **Starting state:** synchronized `main` at `3a4d308` with one deliberately preserved pre-existing staged `src/content/docs/read-link.md` addition describing the intended large-repository views; current source/tests still proved Phase 7 itself was unimplemented, so the docs were reconciled with real behavior rather than treated as completion evidence.
- **Ending commit(s):** `cbe885227270cf38bd22c532a9c8f960508cd8fd` (implementation/tests/public docs); this ledger handoff follows separately.
- **Outcome:** release detail is a bounded overview with safely previewed notes, one provider asset page, exact asset downloads and separate local/provider omission truth; trees retain one-level sorted navigation and directory README context while bounding entry fan-out and separately surfacing the 1,000-entry Contents ceiling; contributor statistics prioritize commit totals with deterministic login tie-breaking and bound displayed rows; deployment environments keep deployment identity plus the latest returned status/log/environment URLs while making older status history explicit without following it.
- **Files/areas changed:** release fetch/rendering in `internal/app/github_refs.go`; one-level tree budgeting in `internal/app/github.go`; contributor-statistics ordering/budgeting and deployment latest-status fetching/rendering in `internal/app/github_activity_deployments.go`; corresponding pathological/regression fixtures; Phase 7 user guidance in `src/content/docs/read-link.md`.
- **Positive evidence:** a synthetic 500-line release with 100 returned assets plus a provider `next` page stays <=5k runes, fetches exactly one asset page, preserves canonical full-notes and exact asset-download URLs, and distinguishes local asset omission from provider-more truth. A 1,000-entry directory stays <=5k while simultaneously reporting provider ceiling and local omission. A 300-contributor fixture stays <=5k, surfaces the two 10,000-commit contributors first with deterministic `alpha` before `zeta` tie ordering, and reports omission. Ten deployments each advertising 250 status pages make exactly ten status requests (`per_page=1`, no follow-up pagination), preserve latest state/log/environment URLs and older-history truth, and remain bounded.
- **Regression evidence:** release list remains body-free; slash-containing release tags still resolve; tree slash-ref resolution, one-level behavior and directory README preview remain green; statistics `202` remains a computing/provider state; deployment list still performs no status fan-out; branch/tag/fork/social/native-failure tests remain green. Final validation passed `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check`.
- **Live evidence:** built `webctx` read `vercel/next.js/releases/tag/v15.5.0` in 2,798 characters / 64 lines versus the sweep's ~29.7k baseline, retaining the release metadata, bounded notes preview and canonical full-release URL. That release currently reports no assets, so asset fan-out is covered deterministically. `vercel/next.js/graphs/contributors` rendered in 2,876 characters / 84 lines versus the sweep's ~18.6k baseline, indexing 70 of 499 returned contributors and locally omitting 429 after descending commit-total ordering.
- **Documentation:** the preserved staged `read-link` addition was completed/reworded to match shipped behavior exactly: bounded release/tree/contributor/deployment views, provider ceilings/202 truth, and direct GitHub navigation without invented flags/selectors.
- **Decisions made:** release assets stop after the first provider page because full asset enumeration is not required for the release overview; a tree keeps case-insensitive name ordering with an exact-case tie-break rather than introducing directory-first ranking; deployment environments request `per_page=1` for each already-bounded deployment and call it the “latest returned status,” avoiding stronger ordering claims than the provider contract proves.
- **Amendments:** none; current first-party GitHub API documentation continues to expose paginated release assets/deployment statuses under API version `2026-03-10` and the Contents API still documents its 1,000-entry directory ceiling.
- **Known defects/risks:** Discussion detail still expands complete GraphQL conversation/reply pages and Gist roots can still expand every file/comment. Exact Discussion/Gist comment fragment semantics have not yet been provider-proven; Phase 8 owns those boundaries.
- **Next handoff:** Phase 8 — inspect only Discussion/Gist plan/sweep/source/tests and current first-party GraphQL/Gist-comment identity semantics; keep Discussion list cursor truth unchanged while bounding detail roots and adding exact comment selection only where canonical IDs/URLs prove it.

### 2026-08-14 — Phase 8 — `complete`

- **Agent/session:** continued in the same ChatGPT Atlas implementation session on `/workspace/repos/webctx` immediately after Phase 7; remote `main` was synchronized before edits.
- **Ending commit(s):** `3d7bd224fcdc7849e1c7a7db4dd7245745227e4a` (implementation/tests/public docs); this ledger handoff follows separately.
- **Outcome:** Discussion detail roots are now bounded overview/index responses rather than exhaustive GraphQL traversals. They preview the main post, preserve accepted-answer identity, expose numeric database IDs plus canonical copied comment URLs, and distinguish returned/local/provider-more conversation state. Exact `#discussioncomment-<id>` reads traverse top-level pages only as needed, test first-page replies before deeper reply pages, defer deep reply pagination until necessary, validate the canonical URL belongs to the requested Discussion, and return the selected full sanitized comment/reply. Gist roots now index bounded files/comments/revisions from one comment provider page without fetching every truncated raw file; exact `#file-*` selectors retain full/range fidelity and only the selected truncated file may use token-safe raw retrieval. Proven `#gistcomment-<id>` selectors call `GET /gists/{gist_id}/comments/{comment_id}` directly, validate returned API identity, and render the exact comment. GitHub's ownerless hash-ID Gist URLs are also recognized natively while ordinary one-segment username/profile paths remain unclaimed.
- **Files/areas changed:** `internal/app/github_discussions_gists.go` for bounded GraphQL/REST fetch shapes, exact selectors, adaptive rendering and identity validation; `internal/app/github.go` for narrowly guarded ownerless Gist-ID parsing; `internal/app/github_discussions_gists_test.go` for the new bounded/exact contract; `src/content/docs/read-link.md` for Discussion/Gist navigation guidance.
- **Provider proof:** current GitHub GraphQL documentation exposes `DiscussionComment.databaseId` and canonical `url`; current Gist REST documentation exposes the direct single-comment endpoint. A live public Gist/comment proved GitHub's copied `#gistcomment-3793344` fragment and ownerless hash-ID URL form. GitHub's own current Gist REST examples also publish ownerless hash-ID `html_url` / clone URLs (including 20-character IDs), supporting the narrow bare-ID parser without treating `gist.github.com/<username>` as a Gist.
- **Positive evidence:** a synthetic 500-line Discussion with 250 reported comments, an accepted answer and 80 reported replies remains <=5k runes and performs exactly one root GraphQL request (`comments(first:30)`, `replies(first:5)`) with no deep pagination. Exact top-level comment selection stops after its first matching provider page even when hundreds of replies are advertised; a selected reply beyond the first reply page triggers exactly one deferred reply-page lookup in the fixture and preserves parent comment ID. A synthetic Gist with 100 large files, 500 reported comments / 30 returned comments, and 100 revisions stays <=5k, makes one comment-page request and zero raw-file requests while reporting local/provider omission independently. Exact Gist file/range tests preserve full source and raw-host token safety; exact Gist comment tests use only the single-comment endpoint and no comment list.
- **Live evidence:** `vercel/next.js/discussions/96973` dropped from the observed 17,625-character / 369-line exhaustive baseline to 3,375 characters / 94 lines while preserving copied numeric comment URLs and reply parentage. `#discussioncomment-17960256` resolved live to the complete selected comment. Public Gist `b9c46c4b27bfe8461631f4c58e8f5d3d` renders natively from both owner-qualified and provider-supported ownerless hash-ID URLs; the ownerless root is 3,355 characters / 47 lines and `#gistcomment-3793344` is 549 characters / 15 lines, with the canonical owner-qualified URL recovered from GitHub metadata.
- **Regression evidence:** Discussion list remains a first-30 authenticated list with truthful “more upstream” cursor semantics and no invented page URL; no-token Discussion paths fail before provider calls. Gist source HTML comments remain source text while human comment HTML comments are sanitized; exact Gist file selection performs no comment fetch; truncated selected-file raw fallback still never forwards GitHub Authorization; Gist revision endpoint behavior remains green. Final validation passed `go test ./...`, `go vet ./...`, `npm test`, `make build`, and `git diff --check`.
- **Documentation:** user-facing `read-link` docs now describe compact Discussion roots, exact copied `#discussioncomment-*` reads, bounded Gist roots, existing `#file-*` exact file/range reads, exact `#gistcomment-*` reads, and ownerless hash-ID Gist support without adding webctx-specific flags.
- **Decisions made:** root Discussion fetches stop after the first bounded provider slice; exact selector cost is permitted only for the selected semantic identity. Accepted answer data is queried separately so it remains visible even if the accepted comment lies outside the returned top-level slice. Deep reply pages are deferred until top-level and first-page reply candidates have been exhausted. One-segment Gist URLs are claimed only for 20–64-character hexadecimal IDs, matching GitHub's documented/live hash-ID forms while avoiding generic user-profile capture; ambiguous two-segment ownerless revision syntax is not guessed.
- **Amendments:** none; provider/live evidence resolved the Phase 8 selector assumptions in favor of implementing both exact Discussion and exact Gist comment selectors.
- **Known defects/risks:** exact Discussion comment selection can still require provider traversal because GitHub GraphQL does not expose a direct database-ID lookup proved for this resource; this cost is explicit-selector-only and does not affect roots. Cross-family route/support hints, docs consistency and release-facing audit remain Phase 9 work.
- **Next handoff:** Phase 9 — reconcile the route matrix across all GitHub families, audit unsupported/ambiguous URLs and fallback hints, align public docs with the shipped overview/exact/raw contract, and add regression coverage for any route-boundary gaps found before final acceptance.

## Execution rules

1. Fetch first and fast-forward safely; current remote code is authoritative.
2. Read `AGENTS.md` and only the current workstream artifacts named by the active phase. Do not browse neighboring historical `gg/` packages merely because they exist.
3. Reconcile progress claims against current source/tests before trusting them.
4. Finish an `in_progress` phase before starting a later phase.
5. Keep every phase vertically exercisable through `webctx read-link`.
6. A green pre-existing suite is regression evidence, not proof of the new behavior. Add positive evidence that changes because of the phase.
7. Use live GitHub fixtures where safe and stable enough; complement them with deterministic pathological fixtures for large output/schema drift/provider failures.
8. Never expose the local `GH_TOKEN` or other `.env.local` values in logs, fixtures, commits, or the ledger.
9. Update docs only when the corresponding public behavior is actually present on the branch.
10. When a load-bearing plan assumption proves false, record an Amendment immediately; do not quietly weaken acceptance criteria.
11. Commit coherent phase work, push normally, fetch again, and verify local full SHA equals remote full SHA.
12. Never force-push, hard-reset away newer work, clean another agent's files, or rewrite shared history.
