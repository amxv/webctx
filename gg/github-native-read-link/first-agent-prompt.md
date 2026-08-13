# First Implementation Agent Prompt — GitHub-Native `read-link`

Work in `/workspace/repos/webctx` on the approved `main` branch.

Your task is to implement **Phase 1 only** of the `github-native-read-link` workstream, validate it thoroughly, update its durable handoff, commit it coherently, and push it normally.

## Repository safety first

1. Inspect the working tree and current branch before changing anything.
2. Fetch `origin` and fast-forward safely. Current remote code is authoritative; the planning SHA is only provenance.
3. If another agent has uncommitted work, do not clean, reset, stage, revert, reformat, or overwrite it. Work around it safely or stop at the conflicting boundary with an exact handoff.
4. Never force-push, hard-reset shared work, or rewrite history.

## Read only the context you need

Read these planning sections before implementation:

1. `gg/github-native-read-link/github-native-read-link-implementation-plan-2026-08-14.md`
   - `Planning Basis`
   - `State of Ideal System`
   - all `Decisions and Assumptions`
   - `Acceptance Criteria` as the overall contract, with special attention to criteria exercised by Phase 1
   - `Phase 1 — Native GitHub foundation, repository roots, blobs, trees, and selectors` in full
   - `Amendments`
2. `gg/github-native-read-link/github-native-read-link-progress.md`
   - standing execution rules
   - Phase 1 row
   - Current handoff
3. `gg/github-native-read-link/github-native-read-link-sweep-2026-08-14.md`
   - `Current GitHub URL handling`
   - `HTTP boundary`
   - `Authentication and secret handling`
   - `Live URL-semantic probes`
   - `GitHub API ceilings and partial-result risks`
   - Landmines 1, 5, 8–13, 15–16, 18

Then inspect the **current** source/tests/docs from Phase 1's `Files to read before starting` list. Reconcile the plan with what exists now; do not assume files/signatures are unchanged merely because planning named them.

## Implement Phase 1 only

Deliver the complete Phase 1 vertical slice:

- one semantic GitHub routing boundary that distinguishes native success, authoritative native failure, and unsupported generic fallback;
- one GitHub HTTP client boundary with response headers/status, REST versioning, optional `GH_TOKEN` then `GITHUB_TOKEN`, Link pagination primitives, GitHub-specific rate/auth errors, and deterministic test injection;
- integration with the existing env / `.env.local` / macOS Keychain credential model without changing current environment-precedence semantics;
- one shared provider-backed ref/path resolver that does not assume the first path segment is the branch;
- repository-root compact frontmatter, roughly 5,000-character safe README preview, canonical full README blob link, and concise useful GitHub URL hints;
- direct blob full reads plus `#L...` line/range and Markdown heading-section selectors;
- correct distinction between root human-view README sanitization and direct source preservation;
- tree one-level listings plus bounded directory README behavior and provider-limit truth;
- private/auth-capable source behavior within GitHub's provider limits;
- existing direct-Markdown and Firecrawl fallback behavior unchanged for unsupported URLs;
- coherent public docs/landing/credential updates for **only the behavior actually delivered in Phase 1**.

Do not implement Issues, PRs, Actions, Discussions, or later-phase route readers early. Do not add public read-link flags. Do not introduce `gh` as a dependency. Keep GitHub security pages outside native routing.

The plan deliberately leaves phase-local helper/file names to you. Preserve its cross-phase names/responsibilities (`GitHubTarget`, `GitHubClient`, native success/error/unsupported boundary) unless current code proves a better equivalent; if you must invalidate a load-bearing cross-phase assumption, add an Amendment immediately.

## Evidence requirements

Passing old tests is regression evidence, not proof Phase 1 works. Add evidence whose outcome is new because of Phase 1.

At minimum demonstrate:

- parser/classifier tables for root/blob/tree/fragments and unsupported/security URLs;
- deterministic injected-HTTP tests for REST headers/version, token precedence, Link parsing, 404/403/429/rate-limit classification, and secret non-leakage;
- slash-ref and simulated overlapping-ref ambiguity tests;
- root frontmatter/5k/safe-Markdown/full-blob/hint output tests;
- direct blob full/source-comment/line-range/heading-selector/binary/provider-limit tests;
- tree one-level/README/provider-ceiling tests;
- live public reads of `amxv/webctx` and at least one public slash-containing ref URL;
- existing direct Markdown and Firecrawl fallback regression behavior;
- applicable repository validation, docs checks/build, and packaging-relevant smoke if Phase 1 changes packaged source layout.

The planning session observed `go test ./...` get stuck in uninterruptible I/O on the Zodex host without a test result. Re-run the repository's real validation stack from the current environment. If an environment issue recurs, record the exact evidence and continue every other validation you can safely perform; do not label an unrun suite PASS.

Use current GitHub first-party documentation to verify unstable provider facts. No persistent cache is allowed. Never put a real token/private response into a fixture or log.

## Finish the phase durably

Before calling Phase 1 complete:

1. Confirm every Phase 1 observable requirement/positive test/regression test/live check/docs update is done.
2. If any load-bearing plan assumption proved false, append an implementation-plan Amendment immediately with evidence and impact on later phases.
3. Update `gg/github-native-read-link/github-native-read-link-progress.md`:
   - Phase 1 row/status/completion commit placeholder as appropriate before commit;
   - Current handoff;
   - append the full dated Phase 1 progress entry.
4. Run appropriate formatting/tests/vet/lint/docs/package checks defined by the current repository; do not invent a fake PASS for unavailable tools.
5. Inspect `git diff` for unrelated changes, secrets, private payloads, cache files, and accidental generated artifacts.
6. Commit the coherent Phase 1 implementation plus its docs/progress updates.
7. Push normally to `origin/main`.
8. Fetch again and verify local full HEAD equals `origin/main` full SHA.
9. Verify the working tree is clean except for any explicitly pre-existing unrelated work you preserved.
10. Return the exact pushed SHA, positive/regression/live validation results, Amendments if any, and the Phase 2 handoff.

Do not stop merely after writing the GitHub client foundation. Phase 1 is complete only when the real repository-root/blob/tree user path is observably working end to end.
