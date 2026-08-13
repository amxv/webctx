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
| 1 | Native GitHub foundation, repository roots, blobs, trees, and selectors | complete | pending Phase 1 commit | Native repository/blob/tree routing, GitHub client/error/auth/pagination boundary, ref resolver, selectors, docs, deterministic/live evidence complete. |
| 2 | Issues, conversations, selectors, lists, labels, and relationships | pending | — | Requires Phase 1 native routing/client/pagination/hints. |
| 3 | Pull Request conversation, reviews, threads, and exact anchors | pending | — | Reuse Issue conversation identity and add PR review/thread truth. |
| 4 | Pull Request Files Changed, commits, checks, and diff selectors | pending | — | Add PR view-specific semantics and current diff/check selectors. |
| 5 | Commits, comparisons, path history, and blame | pending | — | Reuse canonical ref resolver/diff model; GraphQL blame is auth-only. |
| 6 | GitHub Actions runs, jobs, logs, artifacts, and check annotations | pending | — | Run pages bounded; job URLs target one job/log. Verify step selectors before advertising them. |
| 7 | Branches, tags, releases, and stable repository navigation lists | pending | — | Add compact ref/release list/detail surfaces. |
| 8 | Discussions and Gists | pending | — | Discussions require GraphQL auth; Gists use public REST and raw fallback for truncation. |
| 9 | GitHub search and public User/Organization navigation | pending | — | Preserve GitHub search quota/query truth and provider-resolve profile type. |
| 10 | Repository activity, metrics, social lists, and deployments | pending | — | Keep provider delay/cache/status-retention truth explicit. |
| 11 | Packages, Projects v2, and long-tail native route closure | pending | — | Fresh route/API audit; stable read-only native or explicit fallback reason. |
| 12 | Independent acceptance, output hardening, docs/package verification, and route-fidelity audit | pending | — | Map all 127 acceptance criteria to current evidence and close gaps. |

## Current handoff

- **Last completed phase:** Phase 1 — `Native GitHub foundation, repository roots, blobs, trees, and selectors`.
- **Earliest incomplete phase:** Phase 2.
- **Exact phase title:** `Issues, conversations, selectors, lists, labels, and relationships`.
- **Observable boundary:** extend the existing `GitHubTarget` / `GitHubClient` / native success-error-unsupported path to Issue detail/conversation/comment selectors and bounded Issue/list/label/milestone relationships without weakening the Phase 1 source/tree guarantees or generic fallback boundary.
- **Current blockers:** none known. The planning-time Go test hang did not recur; `make check`, full Go tests/vet, docs checks/build, packaging smoke, cross-platform builds, and public live GitHub reads all completed successfully in Phase 1.
- **Plan Amendments affecting Phase 2:** none.
- **Prompt to use:** [`subsequent-agent-prompt.md`](./subsequent-agent-prompt.md).

## Progress entries

### 2026-08-14 — Phase 1 — `complete`

- **Agent/session:** GPT-5.6 Sol implementation session on the Zodex checkout.
- **Starting state:** `main` was clean, fetched, and fast-forwarded to `71e9e2a3da8c08d04129d01cb7a69397e9a8b019`; the only newly fetched work was unrelated Vercel docs-build filtering and was preserved unchanged.
- **Ending commit(s):** pending Phase 1 commit; replace after commit creation.
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
