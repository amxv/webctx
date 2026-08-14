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
| 3 | Bounded PR root and complete selector discoverability | pending | — | PR root becomes a compact map to body/comments/reviews/threads/files/commits/checks. |
| 4 | PR Files and Checks bounded subviews | pending | — | Large patch/check fan-out becomes index-first; focused selectors remain exact. |
| 5 | Actions overview/run/job context safety and raw-log navigation | pending | — | Runs/jobs stop injecting huge child/log output and expose raw log navigation. |
| 6 | Commit and compare overview/raw split plus commit selectors | pending | — | Plain commit/compare become bounded; raw diff/patch and exact commit selectors remain deep paths. |
| 7 | Releases, trees, statistics, and deployment fan-out hardening | pending | — | Remaining high-fan-out repo containers use the shared overview contract. |
| 8 | Discussions and Gists bounded navigation plus exact comment selectors | pending | — | Large conversations/multi-file Gists become index-first with proven canonical selectors. |
| 9 | Cross-family route audit, hints, docs, and release-facing contract | pending | — | Audit every target kind and document the final overview/exact/raw mental model. |
| 10 | Independent acceptance and live stress matrix | pending | — | Re-review all 143 criteria against current code and live/deterministic evidence. |

## Current handoff

- **Last completed phase:** Phase 2 — `Native Pull Request lists and PR-qualified search correctness`.
- **Earliest incomplete phase:** Phase 3 — `Bounded PR root and complete selector discoverability`.
- **Observable boundary:** repository `/pulls` pages now use the Pull Requests REST list directly with compact stable rows, validated filter/sort/page semantics and provider-derived web navigation; PR-search `q=` URLs use Search Issues with explicit `is:pr`, including copied `/issues?q=is:pr...` forms. PR roots themselves still render the existing complete conversation/reviews/threads transcript and lack the Phase 3 stable bounded map/selectable full-description contract.
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
