---
id: "01-migrate-snapshot-ownership"
title: "Migrate snapshot ownership"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/environment-owned-git-status.md
---

# Task 01: Migrate Snapshot Ownership

## Summary

Make the task environment the persisted owner of current Git snapshots. Add a
transactional SQLite and Postgres cutover from the session-scoped shape.

## In scope

- Add `TaskEnvironmentID` to `models.GitSnapshot`.
- Create the final table with an environment cascade foreign key.
- Change the session foreign key to nullable provenance with `ON DELETE SET NULL`.
- Backfill environment IDs through `task_sessions`.
- Remove rows that have no resolvable environment.
- Collapse each environment and repository partition to its current winner.
- Add environment-based current-status repository methods.
- Keep explicit session-history reads for provenance consumers.
- Add fresh, cutover, rollback, replay, and Postgres parity tests.

## Out of scope

- Orchestrator capture changes.
- WebSocket payload changes.
- Frontend state changes.
- Changes to session-commit persistence.

## Acceptance

- Session deletion sets snapshot provenance to null and keeps environment status.
- Environment deletion removes all snapshots in its scope.
- A newer row wins across sibling sessions for each repository.
- A newer sparse row does not reuse files from an older detailed row.
- The cutover is transactional and replayable on SQLite and Postgres.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/repository/... -run 'Test.*GitSnapshot.*(Environment|Migration|Cutover|Replay)' -count=1
```

## Files likely touched

- `apps/backend/internal/task/models/git.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/git_snapshots.go`
- `apps/backend/internal/task/repository/sqlite/git_snapshot_environment_migration.go`
- `apps/backend/internal/task/repository/sqlite/git_snapshot_environment_migration_test.go`
- `apps/backend/internal/task/repository/sqlite/git_snapshot_environment_postgres_test.go`
- `apps/backend/internal/task/repository/git_snapshots_upsert_test.go`

## Dependencies

None.

## Risks

- SQLite and Postgres use different table-lock behavior.
- A nullable session column needs `sql.NullString` at every scan boundary.
- Legacy repository identity is stored in JSON metadata.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9`
- System-design sections "Persistence ownership", "Authoritative selection",
  and "Migration"
- ADR `2026-08-30-environment-owned-git-status`
- ADR `0027-replayable-schema-migrations`
- Existing worktree ownership cutover pattern

## Results

- Added environment ownership and nullable session provenance to Git snapshots.
- Added a transactional SQLite/Postgres shadow-table cutover with winner
  backfill, unresolved-row removal, rollback failpoints, replay-safe indexing,
  and environment-ranked current reads.
- Verified with SQLite repository/cutover tests and the repository package
  suite. Postgres parity tests are included and skip when no DSN is configured.
- Added hybrid worktree-cutover coverage with a final-shape environment-owned
  snapshot. The cutover rehomes snapshots from losing environments and
  rebinds the Postgres snapshot foreign key before the old environment table is
  dropped, preserving snapshots without DROP CASCADE.
