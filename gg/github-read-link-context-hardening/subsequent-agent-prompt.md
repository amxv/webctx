# Subsequent Implementation Agent Prompt — GitHub Read-Link Context Hardening

Continue the existing workstream in:

```text
/workspace/repos/webctx
```

Use the current approved `main` branch.

## Start from current reality

Before editing:

1. read `/workspace/repos/webctx/AGENTS.md`;
2. inspect `git status`;
3. fetch `origin` and fast-forward safely;
4. preserve newer work;
5. never force-push, hard reset, rewrite history, clean unrelated files, or overwrite another agent's work.

Current source/tests/docs are truth. Ledger completion claims are evidence to verify, never proof. Reopen a supposedly complete phase if current code contradicts its acceptance contract.

## Load only the relevant handoff context

Do **not** browse neighboring/historical `gg/` workstreams.

Read:

- `gg/github-read-link-context-hardening/github-read-link-context-hardening-implementation-plan-2026-08-14.md`
  - `State of Ideal System`
  - `Decisions and Assumptions`
  - `Acceptance Criteria`
  - `Amendments`
  - the earliest genuinely incomplete phase
- `gg/github-read-link-context-hardening/github-read-link-context-hardening-progress.md`
  - rules
  - Phase table
  - Current handoff
  - latest entries relevant to the active phase
- only the Sweep sections explicitly named by the active phase;
- the exact **current** source/tests/docs in that phase's `Files to read before starting` list.

If the earliest phase is `in_progress`, finish it before moving later.

## Execution behavior

Start at the earliest genuinely incomplete phase and continue across as many **contiguous phases** as the session can finish thoroughly. Do not stop merely because one phase completed, but do not skip an unfinished phase.

For every phase:

1. reconcile its plan anchors with current code;
2. preserve the cross-phase output vocabulary: overview/container vs exact semantic selector vs explicit raw/bulk resource;
3. keep URL-native navigation—no GitHub-specific `--full`, `--logs`, `--comments`, or similar flags;
4. keep overview/container output roughly around the 5k-character/rune target using safe Markdown boundaries;
5. preserve exact selected human content while preventing subordinate machine logs/summaries from becoming accidental context bombs;
6. preserve explicit raw blob/diff/patch/download paths as the bulk opt-in mechanism;
7. use deterministic preview/state ordering only—no LLM summaries, no bot suppression, no maintainer/inferred relevance ranking;
8. distinguish provider incompleteness/ceilings from local output omission;
9. avoid blind provider pagination when the current overview contract does not require complete enumeration;
10. preserve native-failure authority, auth/rate-limit truth, token non-leak behavior, slash-ref resolution and non-GitHub fallback behavior;
11. add phase-specific positive tests/evidence whose outcome was impossible before the phase;
12. retain relevant regression tests;
13. use live GitHub fixtures where safe and current, plus deterministic pathological fixtures for output scale/schema drift/provider edge cases;
14. update user docs only at coherent shipped public boundaries;
15. update the progress ledger immediately after every completed or blocked phase;
16. add a plan Amendment immediately if a load-bearing assumption proves false.

The user has explicitly authorized broader correctness/optimization fixes that belong to this same defect class. If current code reveals another GitHub **container fan-out, selector-discoverability, schema-robustness, or truthful-boundedness** defect, fix it at the correct invariant boundary and add acceptance/evidence coverage. Do not use this as permission for unrelated refactors or new security/admin GitHub surfaces.

## Live environment

The checkout may contain a local `.env.local` with search/Firecrawl/GitHub credentials for live tests. Use it through the product's credential loader. Never print, log, commit, quote, or copy secret values into prompts, tests, progress entries, artifacts, or chat.

Public live fixtures can move or disappear. A missing live fixture is not permission to weaken deterministic acceptance coverage.

## Phase completion and handoff

A phase is complete only when:

- its observable behavior is present through real `webctx read-link` paths;
- positive evidence specifically proves that phase;
- relevant existing behavior is re-proven;
- applicable live evidence is recorded;
- docs/generated/public contracts are current;
- plan Amendments are current;
- the ledger row, Current handoff and detailed progress entry are updated;
- coherent changes are committed and pushed;
- a post-push fetch confirms local full HEAD equals `origin/main`;
- the worktree is clean except for deliberately preserved unrelated pre-existing work.

If interrupted mid-phase, leave it `in_progress`, record the exact completed slice/evidence/next action, and push only a coherent safe slice when repository policy allows.

Return exact pushed SHA(s), completed phases, validation/live evidence, Amendments, known risks, and the earliest remaining phase/boundary.
