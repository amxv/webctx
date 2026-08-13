# Subsequent Implementation Agent Prompt — GitHub-Native `read-link`

Work in `/workspace/repos/webctx` on the approved `main` branch. Continue the `github-native-read-link` workstream from the **earliest genuinely incomplete phase** and keep going across as many contiguous phases as you can finish thoroughly in this session.

## Repository safety first

1. Inspect branch and working tree.
2. Fetch `origin` and fast-forward safely. Current remote source/tests/docs are authoritative; old SHAs and progress claims are evidence, never proof.
3. Preserve newer work and any other agent's uncommitted files. Do not clean, reset, revert, reformat, stage, or overwrite unrelated work.
4. Never force-push or rewrite shared history.

## Resolve the current phase from evidence

Read:

1. `gg/github-native-read-link/github-native-read-link-progress.md`
   - standing execution rules;
   - Current handoff;
   - latest progress entries relevant to the earliest incomplete/in-progress phase.
2. `gg/github-native-read-link/github-native-read-link-implementation-plan-2026-08-14.md`
   - `Planning Basis` execution protocol;
   - `State of Ideal System`;
   - `Decisions and Assumptions`;
   - `Amendments` in full;
   - the earliest incomplete phase in full;
   - acceptance criteria that phase is supposed to make observable.
3. Only the Sweep sections/landmines named by that phase's `Files to read before starting` section.

Then inspect the **current** source/tests/help/docs named by the phase. Locate current cross-phase symbols by responsibility if earlier agents moved/split files. Do not reconstruct old implementation history merely because progress mentions commits.

If progress says a phase is complete but current code contradicts its acceptance contract, reopen that phase and repair it before moving later. If a phase is `in_progress`, finish it before starting another one.

## Preserve the hard workstream contract

Across every phase:

- one public `webctx read-link <url>` command; use GitHub path/query/fragment semantics instead of new read flags;
- one canonical semantic GitHub router/client/ref-resolution path, not permanent duplicate implementations;
- deterministic semantic projection: remove UI/API noise, never model-summarize substantive selected content;
- root repositories use compact frontmatter + roughly 5k-character safe README preview + canonical full README blob + concise native URL hints;
- direct blobs/conversations/diffs/logs obey their own explicit URL semantics rather than inheriting the root preview cap;
- hints teach only unique useful GitHub/webctx URL capabilities, never generic grep/sed/awk/head/tail-style operations;
- public REST works anonymously whenever GitHub permits; `GH_TOKEN` then `GITHUB_TOKEN` are optional enhancements;
- successful anonymous reads do not nag for tokens; rate/auth-only boundaries give one concise useful hint;
- no persistent GitHub cache: every invocation performs fresh provider reads, while provider-side delay/cache is described truthfully where relevant;
- follow provider `Link` pagination; selected Issue/PR conversations are complete unless GitHub itself exposes a cap, while list/search views stay bounded;
- surface provider caps/truncation/expired data instead of presenting partial output as complete;
- REST first, GraphQL only for capability-specific truth such as Discussions, blame, resolved review threads, Projects v2;
- human-view bodies may strip invisible HTML comments, direct source blobs must preserve source content;
- GitHub security pages remain outside this workstream;
- unsupported GitHub routes keep generic fallback, while recognized authoritative GitHub failures do not silently become Firecrawl;
- no `gh` executable dependency and standard-library Go remains the default.

Do not silently weaken these because a provider edge case is inconvenient. If current GitHub behavior disproves a load-bearing assumption, add an implementation-plan Amendment when you make the replacement decision, state which later phases change, and preserve the acceptance goal.

## Implement and validate the current phase

Follow the current phase's four plan subsections exactly:

- `Files to read before starting`
- `What to do`
- `Validation strategy`
- `What must not break`

Keep the phase vertically exercisable. Do not leave backend-only “foundation” claimed complete when no real `read-link` URL exercises it.

For every phase, collect both:

- **positive evidence** that the newly supported URL/resource now behaves differently and correctly;
- **regression evidence** that relevant existing GitHub and generic read-link behavior still works.

Use live public GitHub evidence wherever safe and possible. Use deterministic HTTP/GraphQL fixtures for rate limits, auth, pagination, provider caps, malformed responses, expired data, ambiguity and other conditions live tests cannot reliably induce. Auth-only live checks require a safe token; if none exists, record that gap truthfully instead of faking completion from a live perspective.

Update user-facing docs/help/landing examples at coherent public boundaries and document only behavior that exists on the current branch.

## Progress, commits, and continuation

After each phase you fully complete:

1. Add any necessary plan Amendment first.
2. Update the phase row, Current handoff, and append the full required dated entry in `github-native-read-link-progress.md`.
3. Run the current repository's appropriate formatting/tests/vet/lint/docs/package checks.
4. Inspect the diff for unrelated changes, private/token material, caches and accidental generated files.
5. Commit coherently and push normally.
6. Fetch and verify local HEAD equals `origin/main` exactly.
7. If the session still has working capacity, begin the next contiguous incomplete phase rather than stopping merely because one phase was completed.

If interrupted mid-phase, leave it `in_progress`, record the exact safe completed slice/evidence/unfinished boundary, and commit/push only a coherent usable slice if repository policy allows it. Never mark partial work complete.

At final handoff, return exact pushed SHA(s), phases completed, positive/regression/live evidence, plan Amendments, known defects/risks, and the earliest remaining phase/boundary.
