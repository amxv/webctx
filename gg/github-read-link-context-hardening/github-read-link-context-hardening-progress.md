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
| 1 | Shared bounded-context contract and resilient Issues | pending | — | Start here. Prove bounded Issue behavior plus polymorphic timeline decoding and `#issue-*`. |
| 2 | Native Pull Request lists and PR-qualified search correctness | pending | — | `/pulls` and `/issues?q=is:pr...` become truthful native PR lists. |
| 3 | Bounded PR root and complete selector discoverability | pending | — | PR root becomes a compact map to body/comments/reviews/threads/files/commits/checks. |
| 4 | PR Files and Checks bounded subviews | pending | — | Large patch/check fan-out becomes index-first; focused selectors remain exact. |
| 5 | Actions overview/run/job context safety and raw-log navigation | pending | — | Runs/jobs stop injecting huge child/log output and expose raw log navigation. |
| 6 | Commit and compare overview/raw split plus commit selectors | pending | — | Plain commit/compare become bounded; raw diff/patch and exact commit selectors remain deep paths. |
| 7 | Releases, trees, statistics, and deployment fan-out hardening | pending | — | Remaining high-fan-out repo containers use the shared overview contract. |
| 8 | Discussions and Gists bounded navigation plus exact comment selectors | pending | — | Large conversations/multi-file Gists become index-first with proven canonical selectors. |
| 9 | Cross-family route audit, hints, docs, and release-facing contract | pending | — | Audit every target kind and document the final overview/exact/raw mental model. |
| 10 | Independent acceptance and live stress matrix | pending | — | Re-review all 143 criteria against current code and live/deterministic evidence. |

## Current handoff

- **Last completed phase:** none.
- **Earliest incomplete phase:** Phase 1 — `Shared bounded-context contract and resilient Issues`.
- **Observable boundary:** current `v0.2.0` native GitHub readers are feature-rich but some container/detail URLs still expand subordinate text without a context budget; old Issues can also fail on object-shaped `minimized` timeline data.
- **Current blockers:** none known. GitHub/provider behavior listed as assumptions in the plan must be verified at the phase that depends on it.
- **Plan Amendments affecting next phase:** none.
- **Prompt to use now:** `first-agent-prompt.md`.

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
