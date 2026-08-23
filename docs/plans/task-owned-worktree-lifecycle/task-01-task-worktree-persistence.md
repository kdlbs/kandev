---
id: "01-task-worktree-persistence"
title: "Normalize task-environment worktree ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/session-delete-resource-cleanup/spec.md"
decision: "../../decisions/2026-08-08-task-owned-worktree-lifetime.md"
---

# Task 01: Normalize Task-Environment Worktree Ownership

## Acceptance

- `task_environment_repos` is the only physical-worktree record and is queryable
  by task through its owning `task_environments` row without a live session.
- `task_sessions.task_environment_id` is the only session/workspace association.
- `task_session_worktrees`, its indexes, Go model, repository methods, duplicate
  schema initializer, and runtime readers/writers are removed.
- Deprecated flat `repository_id`, `worktree_id`, `worktree_path`, and
  `worktree_branch` fields are removed from the `task_environments` schema,
  models, persistence methods, and API.
- Initial, resumed/recreated, multi-repository, and mid-session materialization
  paths persist environment repository ownership before exposing the worktree
  for reuse.
- Environment ownership, repository worktrees, and the session environment link
  are committed consistently.
- Rejected persistence compensates only a worktree created by that attempt.
- SQLite and PostgreSQL upgrades normalize legacy environment and session data,
  validate it, and remove obsolete schema in one rollback-safe migration.
- The cutover uses a dedicated error-returning migration under a database writer
  or advisory lock; it never uses a helper that swallows unexpected errors.
- The complete legacy and final worktree identity/path/branch inventories match
  before the schema swap commits.
- Fresh databases contain only the final schema; production runtime code has no
  legacy read, write, or fallback path.
- Preview-build `session_delete` cleanup jobs are removed before worker startup;
  task-lifecycle cleanup jobs are preserved.
- Conflicting legacy identities or paths fail migration with enough detail to
  repair the data, leaving the pre-upgrade database intact.
- Injected failure at every copy, validation, drop, rename, and commit boundary
  rolls back to the complete old schema and data.
- PostgreSQL advisory and affected-table locks reject concurrent migration or
  mixed-version writes during cutover; lock timeout changes nothing.
- The migration performs no filesystem or Git operations, and cleanup workers
  cannot start against a partially initialized repository.
- SQLite snapshot restore and PostgreSQL operator-backup restore are verified as
  the downgrade path; old binaries are never run against the final schema.
- Task inventory and active-worktree-path reads no longer require a session join.

## TDD Sequence

1. RED: add SQLite and PostgreSQL fresh-schema tests for the exact final
   environment/repository/session shape and absence of obsolete tables/columns.
2. RED: seed a task environment, a legacy flat environment, and a session-only
   association; prove all three normalize into environment repository ownership,
   sessions receive the correct environment ID, and session deletion preserves
   the owner. Add conflicting-source cases that roll back the migration.
3. RED: assert successful upgrade drops `task_session_worktrees` and deprecated
   environment columns for both database engines, including migration replay.
4. RED: inject errors after each shadow-copy, validation, drop, rename, and
   pre-commit step; reopen the database and prove its legacy schema and data are
   byte-for-byte equivalent at the ownership-field level.
5. RED: exercise concurrent PostgreSQL initializers and prove the advisory lock
   and affected-table locks serialize the cutover and reject mixed-version
   writes. Exercise SQLite writer contention and prove timeout or failure leaves
   the database unchanged.
6. RED: add store tests for task and session projections through the environment,
   zero-session inventory, ownership transfer, and atomic persistence.
7. GREEN: implement the schema normalization and remove legacy models, CRUD,
   API fields, schema initialization, and dual-write paths.
8. GREEN: wire every physical materialization path and bounded compensation.
9. REFACTOR: make `task_environment_repos` the shared projection for branch
   updates, session responses, storage inventory, and worktree management.
10. REFACTOR: add an architecture assertion that legacy ownership symbols are
    confined to the migration and its fixtures.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite/... ./internal/worktree/...
cd apps/backend && go test ./internal/backendapp/... -run 'Worktree|BranchMaterial'
```

Run PostgreSQL schema/migration tests with the repository's configured test DSN;
recording only a skipped result is incomplete.

Also run the persistence provider upgrade tests to prove SQLite backup failure
prevents migration and successful upgrade leaves a restorable snapshot.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`
- `apps/backend/internal/task/repository/sqlite/schema_replay_test.go`
- `apps/backend/internal/persistence/upgrade_test.go`
- `apps/backend/internal/persistence/snapshot_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/worktree/manager.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/web/lib/api/domains/task-environment-api.ts`
- task-environment API consumers that use deprecated flat worktree fields

## Dependencies

None.

## Inputs

- `docs/specs/tasks/system-design/runtime-cleanup.md`, especially the normalized task
  environment data model.
- `docs/decisions/2026-08-08-task-owned-worktree-lifetime.md`.
- Existing `TaskEnvironment` ownership-transfer behavior and worktree manager
  materialization rollback patterns.

## Output contract

Report the exact final schema, removed tables/columns/types/methods, backfill
grouping and source precedence, old/new inventory comparison, conflict
diagnostics, every injected rollback result, SQLite snapshot evidence,
PostgreSQL advisory-lock evidence, every materialization path audited,
compensation behavior, backup-restore/downgrade evidence, and SQLite/PostgreSQL
results. No legacy reader or writer may be deferred.
