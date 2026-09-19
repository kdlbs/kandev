---
spec: docs/specs/tasks/requirements/session-delete-resource-cleanup.md
created: 2026-08-11
status: implemented
---

# Fix Plan: Recover Hybrid Worktree Cutover Schemas

## Overview

Repair startup for databases where `task_environments` already has its final
normalized shape while the legacy `task_session_worktrees` table still exists.
The cutover currently uses that table as its only legacy sentinel and then
unconditionally selects removed flat environment columns. The repair makes the
legacy environment read reflect the columns actually present while preserving
the existing transactional normalization, validation, and fail-closed behavior.

Confirmed root cause: `normalizeTaskWorktreeOwnership` starts whenever
`task_session_worktrees` exists, but `loadLegacyEnvs` always selects
`repository_id`, `worktree_id`, `worktree_path`, and `worktree_branch` from
`task_environments`. Intermediate or alternate branches can leave only the
sentinel table behind, causing startup to fail before normalization.

## Backend

### Hybrid-schema compatibility

- Update `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration.go`
  and `worktree_ownership_normalize.go` to determine which deprecated flat
  `task_environments` columns exist inside the cutover transaction.
- Project typed empty SQL values for missing flat columns so the existing
  normalization pipeline can still consume canonical `task_environment_repos`
  and legacy `task_session_worktrees` rows.
- Keep the legacy-table sentinel, shadow-table swap, inventory validation,
  PostgreSQL locking, SQLite transaction, and rollback behavior unchanged.

## Tests

- **What:** The repository initializer upgrades a hybrid SQLite schema with a
  final `task_environments` table and stale `task_session_worktrees` table.
  **File:** `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`.
  **How:** Initialize a fresh database, recreate only the legacy session table,
  seed a session-worktree row backed by normalized environment/repository data,
  reopen through `NewWithDB`, and assert startup succeeds, inventory survives,
  and the legacy table is removed. The test must first fail with the reported
  `no such column: repository_id` error.
- **What:** Fully legacy, final, rollback, and conflict fixtures remain valid.
  **File:** same migration test file.
  **How:** Run all `TestCutover` tests and the complete repository package.
- **What:** PostgreSQL uses the same hybrid-schema recovery behavior through
  its `information_schema` column probes.
  **File:** `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`.
  **How:** Run the environment-gated PostgreSQL counterpart locally when
  `KANDEV_TEST_POSTGRES_DSN` is available and in the Backend Postgres CI job.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [task-01-recover-hybrid-schema](task-01-recover-hybrid-schema.md)

The migration and regression fixture share schema state and are not
parallel-safe.

## Out Of Scope

- Repairing arbitrary missing required columns outside the four deprecated flat
  worktree fields.
- Changing post-cutover task, session, worktree, cleanup, filesystem, or Git
  behavior.
- Adding a permanent dual-read or dual-write compatibility path.

## Verification Results

- RED: `go test ./internal/task/repository/sqlite -run
  TestCutover_HybridNormalizedEnvironmentWithLegacySessionWorktrees -count=1 -v`
  failed with `cutover: read legacy environments: no such column: repository_id`.
- GREEN: the same focused command passed after the repair.
- `go test ./internal/task/repository/sqlite -run TestCutover -count=1` passed.
- `go test ./internal/task/repository/sqlite -count=1` passed.
- `go test ./internal/task/repository/sqlite -run
  TestPostgresCutoverHybridNormalizedEnvironmentWithLegacySessionWorktrees
  -count=1 -v` compiled and skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` is unset; the Backend Postgres CI job owns the live
  dialect execution.
- `git diff --check` passed.
