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
| 1 | Native GitHub foundation, repository roots, blobs, trees, and selectors | pending | — | Start here: canonical target/client/error/ref-resolver plus root/frontmatter/5k README and source/tree selectors. |
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

- **Last completed phase:** none; planning only.
- **Earliest incomplete phase:** Phase 1.
- **Exact phase title:** `Native GitHub foundation, repository roots, blobs, trees, and selectors`.
- **Observable boundary:** `webctx read-link` should first gain native repository-root/frontmatter/README previews, full/source-selected blob reads, tree listings, optional GitHub auth/error/pagination infrastructure, and one semantic GitHub routing path while generic fallback remains intact.
- **Current blockers:** none known. Planning-time `go test ./...` on the Zodex host entered uninterruptible I/O and produced no test result; this is an environment observation to re-check, not a product blocker or failing-test claim.
- **Plan Amendments affecting Phase 1:** none.
- **Prompt to use:** [`first-agent-prompt.md`](./first-agent-prompt.md).

## Progress entries

No implementation phases have started yet.

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
