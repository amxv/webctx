# First Implementation Agent Prompt — GitHub Read-Link Context Hardening

Work in the existing checkout:

```text
/workspace/repos/webctx
```

Work on the repository's current approved `main` branch. Before editing:

1. read `/workspace/repos/webctx/AGENTS.md`;
2. inspect `git status`;
3. fetch `origin` and fast-forward safely;
4. preserve all newer remote work;
5. never force-push, hard reset, rewrite history, clean unrelated files, or overwrite another agent's work.

Current remote source, tests, docs, and generated/public contracts are authoritative. The planning SHA is provenance only.

## Read only the active workstream context

Do **not** browse neighboring/historical `gg/` workstreams.

Read:

1. `gg/github-read-link-context-hardening/github-read-link-context-hardening-implementation-plan-2026-08-14.md`
   - `Planning Basis`
   - `State of Current System`
   - `State of Ideal System`
   - `Decisions and Assumptions`
   - `Acceptance Criteria`
   - **Phase 1 only**
   - `Amendments`
2. `gg/github-read-link-context-hardening/github-read-link-context-hardening-progress.md`
3. In the Sweep, only the Phase 1-named sections:
   - `Architecture map`
   - `Shared GitHub client behavior`
   - `Shared truncation primitive`
   - `Issue detail behavior`
   - `Landmines` 1–6
   - `Existing patterns worth copying`
4. The exact current source/tests/docs named in Phase 1's `Files to read before starting` section.

Do not assume the planning baseline still matches current code. Re-open every named symbol in the current branch before changing it.

## Implement Phase 1 only

Implement **Phase 1 — Shared bounded-context contract and resilient Issues** completely.

The hard product constraints are:

- `read-link` stays URL-driven; add no GitHub-specific CLI flags.
- overview/container output targets roughly 5k Unicode characters/runes using safe Markdown boundaries, not a tokenizer or hard byte cut;
- exact semantic selectors remain the opt-in path to one full selected human-content object;
- raw/blob/diff/patch representations remain explicit bulk paths;
- no LLM/model summarization enters `read-link`;
- no bot/maintainer relevance heuristic is allowed;
- provider-incomplete and locally omitted content must remain distinct;
- native GitHub failures remain authoritative; do not hide failures by scraping GitHub UI;
- optional GitHub auth stays optional and secrets must never be printed;
- security/admin/settings pages remain out of scope.

For Phase 1 specifically:

- establish reusable GitHub overview/index/preview/omission helpers only as wide as the Issue vertical slice needs;
- tolerate the live object-shaped Issue timeline `minimized` representation and nearby optional shape drift without erasing the primary Issue;
- preserve small Issue full-conversation readability;
- make large Issue roots deterministic bounded overviews;
- add and validate exact `#issue-<Issue id>` body selection;
- keep `#issuecomment-*` narrow;
- preserve relationships, pinned comments, event filtering, provider ceilings, and error semantics.

Do not implement `/pulls` or later PR/Actions/compare phases early unless a tiny shared change is strictly required to keep the Phase 1 architecture coherent. Leave later public behavior unchanged.

## Evidence required before Phase 1 is complete

Positive evidence must include:

- deterministic boolean/null/object-shaped `minimized` fixtures, including `{"reason":"spam"}`;
- a synthetic large Issue proving bounded output + safe body/comment previews + exact selector URLs + truthful omission;
- a small Issue proving the existing readable conversation form remains;
- `#issue-*` valid/mismatch selector tests;
- live native read of `https://github.com/cli/cli/issues/326` or an equivalent still exposing the polymorphic shape;
- actual output-size observation for at least one large synthetic/live Issue.

Regression evidence must include the repository's relevant existing Go tests, `go vet`, and any canonical validation current code requires. Re-prove repository root/source selectors and non-GitHub fallback seams if shared helpers touched them.

Use the live `.env.local` only through existing credential loading. Never print token/key values.

## Documentation and plan maintenance

Update user-facing docs in this phase only if Issue/body-selector/bounded-overview behavior is now public and the current docs would otherwise lie.

If implementation reality invalidates a load-bearing assumption or phase instruction, add an `Amendments` entry to the implementation plan immediately. An Amendment may change the implementation approach; it may not weaken the user's goal or silently remove acceptance criteria.

## Handoff discipline

When Phase 1 is fully complete:

1. update the Phase 1 row in `github-read-link-context-hardening-progress.md`;
2. update `Current handoff` to Phase 2;
3. append the required detailed progress entry;
4. stage only coherent intended files;
5. verify the diff contains no secrets/private live data;
6. commit coherently;
7. push normally to `main`;
8. fetch again;
9. verify local full HEAD equals `origin/main` full SHA;
10. verify a clean worktree except for any pre-existing unrelated work you deliberately preserved.

Return the exact pushed SHA(s), positive/regression/live evidence, any plan Amendments, and the Phase 2 handoff.
