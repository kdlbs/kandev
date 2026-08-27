---
spec: docs/specs/tasks/requirements/session-delete-resource-cleanup.md
created: 2026-08-13
status: complete
---

# Fix Plan: Recover Cutover From Orphaned Session Worktrees

## Overview

Repair the one-time task-worktree ownership cutover for legacy databases where
`task_session_worktrees` rows survived deletion of their `task_sessions` rows.
The migration will discard only orphaned history whose physical worktree is
already owned by a higher-precedence task source, while preserving the current
fail-closed behavior for active orphaned worktrees with no recoverable owner.

## Root Cause

`worktreeCutover.loadLegacySessionWorktrees` currently requires every legacy
row to resolve through `c.sessions` before the migration has a chance to apply
the established task-owned source precedence. It records every missing session
as an unconditional conflict and omits the row from `c.sessionWts`.

The read-only production-shaped reproduction contains an active legacy row for
worktree `30e76351-5328-4dd5-9a2e-a8bf4825dfbd` whose session is missing, while
the surviving flat `task_environments` row already owns that exact worktree ID,
path, and branch. Startup therefore aborts even though the physical identity has
one unambiguous higher-precedence owner. `PRAGMA foreign_key_check` confirms the
legacy database also contains other child rows for deleted sessions, explaining
how this state can exist despite the declared foreign key.

The smallest reliable automated reproduction is a legacy SQLite fixture with a
task and flat environment, an orphaned `task_session_worktrees` row carrying the
same worktree ID, and no corresponding `task_sessions` row. The current
initializer must fail with `references missing session` before the repair.

## Backend

### Deferred orphan classification

Update
`apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`.
Retain missing-session rows in a separate orphan inventory during
`loadLegacySessionWorktrees` instead of immediately adding a conflict. After
`pickSurvivingEnvironments` has selected the authoritative flat environment,
classify each orphan with one narrow helper. Treat the row as superseded only
when:

- the row is legacy-deleted (`status = deleted` or `deleted_at` is set); or
- the row has a non-empty `worktree_id` represented by a non-deleted normalized
  target already built from a canonical `task_environment_repos` row or
  surviving flat `task_environments` row.

The match is based on physical identity, not path, branch, repository, or a
guessed task association, because the missing session removes the authoritative
task link. A superseded orphan contributes no ownership, metadata, inventory,
or demotion entry. An orphan not satisfying the rule adds the exact current
missing-session conflict. Deleted canonical repository rows, flat rows from
collapsed environments, and raw flat rows absorbed into a deleted normalized
target are not owners and must not suppress an active orphan.

Keep transaction, snapshot, shadow-table validation, schema swap, rollback,
replay, PostgreSQL locking, and filesystem/Git behavior unchanged. The shared
normalizer covers SQLite and PostgreSQL cutovers, so both engines use the same
classification.

## Tests

- **What:** An orphaned legacy session-worktree that repeats the surviving flat
  environment's physical ID does not block cutover, and stale orphan metadata
  does not override the flat source.
  **File:**
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_orphan_migration_test.go`.
  **How:** Seed the flat source, insert the session-worktree without a session,
  run the initializer, and assert the normalized row retains flat metadata and
  the legacy table is dropped.
- **What:** An orphan that repeats a non-deleted canonical repository row is
  ignored, including when its path and branch differ.
  **File:** the same migration test file.
  **How:** Seed the canonical row and orphan, then assert canonical metadata and
  successful cutover.
- **What:** A deleted orphan is historical even without another source.
  **File:** the same migration test file.
  **How:** Seed a deleted orphan without its session and assert successful
  cutover without creating an owner from the orphan.
- **What:** An active orphan with an empty or unique physical ID, one matching
  only a deleted canonical repository row or collapsed flat row, or one whose
  raw flat source is absorbed into a deleted normalized target remains fatal
  and transactional.
  **File:** the same migration test file.
  **How:** Assert the missing-session diagnostic and that the legacy table and
  row survive rollback.
- **What:** PostgreSQL applies the same shared classification.
  **File:**
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_postgres_test.go`.
  **How:** Add the smallest PostgreSQL fixtures for one recoverable orphan and
  one unique active orphan that must fail closed.
- **What:** Existing precedence, inventory, rollback, replay, hybrid-schema,
  and contested-slot behavior remains unchanged.
  **File:** existing migration test files.
  **How:** Run the focused orphan tests, all cutover tests, and the complete
  SQLite repository package.

## Verification Results

- RED: `make -C apps/backend test
  GOFLAGS="-v -run=TestCutover.*Orphan.* -count=1"` failed for the recoverable
  flat, canonical, and deleted-history fixtures with the reported
  `references missing session` diagnostic. The unique active and
  deleted-canonical controls passed by proving the current fail-closed result.
- GREEN: `go test -tags fts5 ./internal/task/repository/sqlite -run
  'TestCutover.*Orphan.*' -count=1 -v` passed four SQLite top-level tests and
  three fail-closed subtests. The PostgreSQL test skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` is not set.
- Focused gate: `make -C apps/backend test
  GOFLAGS="-v -run=TestCutover.*Orphan.* -count=1"` passed.
- Cutover gate: `make -C apps/backend test
  GOFLAGS="-v -run=TestCutover -count=1"` passed. The package discovers 44
  top-level cutover tests; environment-gated PostgreSQL cases skip locally.
- Backend gate: `make -C apps/backend test GOFLAGS="-v -count=1"` passed.
- A temporary `-tags fts5` repository test ran the initializer against an
  SQLite backup of the reported 1.4 GB database. The cutover completed, removed
  `task_session_worktrees`, and retained worktree
  `30e76351-5328-4dd5-9a2e-a8bf4825dfbd` at the original path and branch. The
  temporary test and database copy were removed; the live database was never
  modified.
- `git diff --check` passed.
- Public docs need no change: the existing operations guidance still correctly
  requires transactional rollback and backup recovery for conflicts that
  remain irreconcilable. The repair behavior is recorded in the internal spec.
- PR review remediation added empty-ID and PostgreSQL fail-closed parity plus a
  regression for a raw flat row absorbed into a deleted canonical target. The
  new P1 regression failed before the classifier inspected normalized targets,
  then the focused orphan and complete cutover gates passed after the repair.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [task-01-classify-orphaned-session-worktrees](task-01-classify-orphaned-session-worktrees.md) - done

No task is parallel-safe. Classification, migration fixtures, and rollback
coverage share one persistence boundary.

## Risks

- Matching on path or branch could discard a distinct physical worktree. The
  recovery requires an identical, non-empty `worktree_id`.
- Treating a deleted canonical row as authoritative could hide the only active
  legacy owner. Only non-deleted canonical rows suppress active orphans.
- Trusting a raw surviving flat row is insufficient when merge precedence has
  left its physical identity on a deleted target. The classifier must inspect
  the normalized target that will actually be persisted.
- Trying to infer the missing session's task from repository or path data could
  silently assign ownership to the wrong task. Recoverable orphans contribute
  no ownership; a higher-precedence source must already own the identity.
- Merge and inventory logic must never see a discarded orphan, or its stale
  metadata can reappear as a validation conflict.

## Out Of Scope

- Repairing unrelated orphaned session messages, turns, Git snapshots, or other
  historical foreign-key violations.
- Manual database edits or automated deletion outside this one-time cutover.
- Filesystem or Git inspection, reconciliation, or cleanup during startup.
- Changes to runtime session deletion or task cleanup behavior.
- A new migration, public API, UI, or feature flag after the one-time cutover.
