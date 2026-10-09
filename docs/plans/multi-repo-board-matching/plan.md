---
created: 2026-10-01
status: in_progress
requirements:
  - REQ-UI-BOARD-REPOSITORY-MATCHING-001
system_design:
  - ../../specs/ui/system-design/board-repository-matching.md
legacy_specs: []
---

# Implementation Plan: Multi-repository board matching

## Overview

Repair primary-only board predicates in one sequential work order. Complete the
required design handoff to the parent coordinator before permanent tests or
production edits. Publication and normal merge are already authorized; delivery
is complete only after exact-head checks, trusted semantic review, and merge.

## Scope

### In scope

- Full-collection membership filtering and existing repository-name/path search.
- Regression coverage through helper, single-workflow hook, shared projections,
  multi-workflow swimlane and occupancy paths.
- Minimal public board-filter documentation and accurate delivery results.

### Out of scope

- Other selected fixes, API/schema changes, sidebar path filtering, layout and
  interaction changes, new search arms on unrelated surfaces, and live data.
- Additional persistent tasks, sessions, or implementation workers.

## Technical approach

Extend `FilterableTask` and introduce a minimal shared ID helper in
`apps/web/lib/kanban/filters.ts`. Reuse it in the existing repository search arm
of `use-kanban-data.ts`. Preserve explicit empty collections and the no-filter
array reference. `task-projections.ts` propagates corrected membership to both visibility and
occupancy and applies repository name/path search to visible cards. The live
swimlane hooks supply workspace-scoped lookups and invalidate caches on metadata
changes.

Root cause: `filterTasksByRepositories` and the hook's repository search arm
read only `repositoryId`, despite the full optional collection in the existing
task type. Read-only reproduction on main `08e4ffdb9` called the actual helper
with backend/web links and selection web: expected `["multi"]`, actual `[]`.
The supplied investigation also reproduced the real hook returning no task for
secondary repository name `client-ui`; its temporary tests were removed.

PR #1512 head `a8655ad92faa050a68140a904500fc99ebc0a04b` changes the hook to
query-backed board/workspace reads but retains both primary-only predicates.
Refresh its state and main before implementation and final rebase; preserve
whichever data source is current, including active-workspace metadata scoping.

## Tests

- `filters.test.ts`: AC .1-.3, .7; table-driven membership and compatibility.
- `use-kanban-data.test.tsx`: AC .1-.4, .7; real hook with secondary name/path,
  collection authority, legacy payloads, no filter, missing/other-workspace
  metadata, and filter/search composition.
- `task-projections.test.ts`: AC .1-.3, .5-.6; per-workflow mixed memberships,
  scalar contradiction and occupancy even when search excludes cards.
- `use-swimlane-render-data.test.tsx`: AC .5-.7; real shared hooks retain
  secondary-linked tasks and their occupied steps in focused/all-workflow views.
- Existing swimlane-container and workflow-swimlanes suites guard composition.

## Rendered and mobile verification

No layout or viewport-dependent interaction changes. The mobile-parity
state/data normalization exception applies. The real hook and shared projection
regressions cover the same data on desktop and phone; no new Playwright case or
ASCII layout preview is needed for this projection-only package.

## Work orders

- [x] [Task 01: Complete repository membership](task-01-repository-membership.md)

## Verification results

- Refreshed origin/main; HEAD equals `08e4ffdb99caf40b0df5baa67b29cf4313188f15`.
- Dependency install: `(cd apps && pnpm install --frozen-lockfile)` passed.
- Read-only actual-helper reproduction failed as expected: secondary selection
  produced an empty result.
- `python3 scripts/list-docs.py validate`: passed (339 decisions, 1283 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight: passed against the prospective
  two-source-file change and all four package artifacts (covered). Actual
  candidate-head coverage must be checked again after implementation.
- `git diff --check -- docs/specs docs/plans`: passed; all four artifacts are
  unstaged/uncommitted as required for design handoff.
- Parent completed the required separate-turn handoff and explicitly requested
  implementation of this package.

- Permanent red evidence: helper and real useKanbanData regressions failed on
  secondary membership/search and stale scalar authority before production edits.
  Fixture corrections preceded the accepted hook red run (10 expected failures,
  5 compatibility controls passed).
- Focused Vitest command in Verification: passed, 6 files and 67 tests.
- Focused ESLint command: passed with zero warnings after naming the repeated
  first-task fixture ID.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:ratchet`: passed; zero added and two modified source files clean.
- `python3 scripts/list-docs.py validate`: passed (339 decisions, 1283 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Public-doc validator tests: passed (62 tests); validator passed (47 pages).
  Commands were corrected to launch Node from apps but run the scripts at the
  repository root, where their relative fixture paths resolve.
- `git diff --check`: passed; all four delivery artifacts present.
- Mobile parity: shared state/data projection exception; no layout or
  interaction changes. Hook/projection/swimlane regressions cover both surfaces.
- Public docs updated: `docs/public/tasks-and-workflows.md` (how-to guide), one
  sentence documenting secondary repository selection on Kanban/Pipeline boards.
- Exact-head documentation preflight and PR CI/review/merge remain delivery
  gates, recorded externally after commit/publication. No schema or API change.


## Risks

- Falling back on array length would resurrect stale scalar membership for [].
- Attachment IDs must not be mistaken for `repository_id`.
- PR #1512 or another main update may require adapting hook fixtures and source
  reads, then rerunning all affected checks before publication/merge.

## Review remediation results

Parent approved the bounded design amendment and explicitly released this task
for sequential implementation on 2026-10-02. Live board cards now search the same
repository name/path fields as the legacy hook, using canonical membership and
owning-workflow workspace lookups. Overview caches and focused memos recompute
when metadata or workflow workspace changes. Occupancy excludes the search lens.
Repeated per-task repository scans are replaced by ID lookups.

Two permanent real-swimlane and two direct projection tests failed before the
live search fix. Final targeted Vitest passed: 6 files, 71 tests. Focused ESLint
and typecheck passed. Coverage includes metadata arrival, rename/path changes,
removal, workflow workspace changes, missing workflow metadata, collection
fallback/authority and mobile navigator counts. No full local E2E suite run.

Current main compatibility is checked using an isolated synthetic merge rather
than rewriting the published candidate to chase unrelated changes. #1512 remains
open at its previously inspected head; main has no intervening changes to the
owned board projection source. Local implementation is complete; exact-head
hosted checks, all thread dispositions, substantive authenticated CodeRabbit
review and normal merge remain external delivery gates. Initial Claude workflow
success had no semantic verdict and `is_error: true`; it is not review evidence.
