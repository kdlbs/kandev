---
id: "01-recover-hybrid-schema"
title: "Recover hybrid cutover schemas"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/session-delete-resource-cleanup.md"
---

# Task 01: Recover Hybrid Cutover Schemas

## Acceptance

- A hybrid database with normalized `task_environments` and a stale
  `task_session_worktrees` table starts successfully without manual edits.
- Canonical environment/repository data and remaining session-worktree inventory
  survive normalization, and the stale legacy table is removed.
- Fully legacy conflicts still fail closed and all existing cutover fixtures
  retain their behavior.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run TestCutover_HybridNormalizedEnvironmentWithLegacySessionWorktrees -count=1 -v
go test ./internal/task/repository/sqlite -run TestPostgresCutoverHybridNormalizedEnvironmentWithLegacySessionWorktrees -count=1 -v
go test ./internal/task/repository/sqlite -run TestCutover -count=1
go test ./internal/task/repository/sqlite -count=1
```

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`
- `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`
- `docs/specs/tasks/requirements/session-delete-resource-cleanup.md`
- `docs/plans/task-worktree-cutover-hybrid-schema/plan.md`
- `docs/plans/task-worktree-cutover-hybrid-schema/task-01-recover-hybrid-schema.md`

## Dependencies

None.

## Parallelism

Sequential. Production and test changes share the cutover schema and migration
contract.

## Inputs

- Confirmed `main` reproduction through `NewWithDB` with the exact error
  `cutover: read legacy environments: no such column: repository_id`.
- Existing `openLegacyDB`, schema-shape assertions, inventory checks, and
  cutover rollback tests in `worktree_ownership_migration_test.go`.
- The transactional cutover contract in the linked spec.

## Output Contract

Report the red failure, green test results, files changed, compatibility scope,
remaining risks, synchronized task/plan status, commit, and PR URL.

## Results

- Added a real-initializer hybrid-schema regression that preserves canonical
  normalized ownership and removes the stale legacy table.
- The cutover probes the four deprecated flat environment columns inside its
  transaction and substitutes typed empty values only for columns already
  absent. Required columns and downstream inventory validation are unchanged.
- RED: the focused hybrid test failed with the reported missing-column error.
- GREEN: the focused hybrid test, all cutover tests, and the complete SQLite
  repository package passed.
- The PostgreSQL counterpart compiled and used the standard environment gate;
  it skipped locally because `KANDEV_TEST_POSTGRES_DSN` is unset. The PR's
  Backend Postgres job executes the live-dialect test.
- `git diff --check` passed.
- External side effects: none. Tests used temporary SQLite databases only.
- Files changed: the cutover migration/normalizer, SQLite and PostgreSQL
  migration tests, the worktree cleanup spec, and this plan/task package.
- Compatibility scope: only hybrid schemas missing one or more of the four
  deprecated flat `task_environments` worktree columns are relaxed. Required
  columns, conflict detection, inventory validation, and rollback remain strict.
- Remaining risk: unrecognized intermediate schemas missing required columns
  still fail closed by design; local PostgreSQL execution requires the external
  test DSN and is therefore delegated to the existing CI job.
- Task and plan status are synchronized as `done` and `implemented`.
- Initial implementation commit: `2d3384b01789d8aff735151249f66defd3db5863`.
- Pull request: https://github.com/kdlbs/kandev/pull/2531
