---
id: "01-persistence"
title: "Persist completion settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-COMPLETION-001.5
  - AC-TASKS-COMPLETION-001.6
  - AC-TASKS-COMPLETION-001.7
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 01: Persist completion settings

## Summary

Persist explicit completion decisions and preserve existing workflow behavior across upgrade. This work owns schema and legacy stored-data conversion.

## In scope

- Add CompleteTaskOnEnter to workflow and task step models, stored definitions, and every repository create/update/select/scan/bootstrap projection.
- Add the shared column in both repository startup paths. Implement workflow-owned transactional backfill plus durable marker, including stored template JSON without overwriting unrelated fields.
- Test real constructor order, fresh databases, pre-column upgrade, mixed names/case/whitespace/positions, explicit false, replay after disabling, and injected failure followed by retry on SQLite and PostgreSQL.

## Out of scope

Runtime terminal predicates, portable API behavior, and UI are later work orders.

## Acceptance

- Existing qualifying final steps are enabled once; nonqualifying steps and new custom steps default false.
- True/false survive CRUD, bootstrap, table replay, and migration rollback/retry. IDs, timestamps, history, and user edits remain intact.
- Required-store SQL guard, fresh/replay conformance, and legacy upgrade assertions pass on both database dialects.

## TDD entry

Create completion_policy_test.go beside workflow repository tests. TestCompletionPolicyMigration should fail because legacy Done has no enabled field after current initialization; TestCompletionPolicyReplayPreservesDisabled must fail if replay re-enables false. Use a minimal field declaration only if needed to compile the regression before implementation.

## Verification

Run from `apps/backend/`. PostgreSQL-dependent cases use an isolated `KANDEV_TEST_POSTGRES_DSN`; never point tests at the reported installation.

```bash
rtk go test -race ./internal/workflow/repository ./internal/task/repository/sqlite -run 'TestCompletionPolicy|TestWorkflowStep|TestBuiltin' -count=1
rtk go run ./cmd/sqlguard ./internal
rtk go test -race ./internal/persistence/requiredstores ./internal/persistence/storeconformance -count=1
```


## Files likely touched

- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/workflow/repository/completion_policy_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/defaults.go`
- `apps/backend/internal/task/repository/sqlite/workspace_bootstrap.go`
- `apps/backend/internal/persistence/storeconformance/owner_actions.go`
- `apps/backend/internal/persistence/storeconformance/upgrade_test.go`

## Dependencies

None.

## Risks

Do not put a new-column reference ahead of ADD COLUMN, replay a semantic backfill without its marker, or alter historical release snapshots to look like the new schema. Provision an isolated PostgreSQL DSN for the same commands and report skips explicitly.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), criteria in frontmatter.
- [System design](../../specs/tasks/system-design/task-completion.md).
- [Plan](plan.md), confirmed source trace and existing reproduction tests.
- Nearby Go repository/handler tests supply fixture conventions.
- Follow `/tdd` in the primary session.

## Results

- RED: `rtk go test ./internal/workflow/repository -run TestCompletionPolicyMigration -count=1` failed before the migration implementation because the legacy final step remained disabled.
- Final: `rtk go test -race ./internal/workflow/repository ./internal/task/repository/sqlite -run 'TestCompletionPolicy|TestWorkflowStep|TestBuiltin' -count=1` passed (8 tests).
- Final: `rtk go run ./cmd/sqlguard ./internal` passed.
- Final: `rtk go test -race ./internal/persistence/requiredstores ./internal/persistence/storeconformance -count=1` passed (305 tests).

The workflow and task repository schemas now persist the explicit boolean. The
transactional marker-backed migration preserves explicit false values and
normalizes legacy stored templates without overwriting unrelated fields.
