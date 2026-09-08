---
id: "04-classify-missing-task-plan-writes"
title: "Classify missing-task plan writes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/documents.md"
---

# Task 04: Classify missing-task plan writes

Translate a plan foreign-key rejection to the shared missing-task sentinel.

## Scope

- Use `internal/db.IsForeignKeyViolation` at the plan-head write boundary.
- Return `repoerrors.ErrTaskNotFound` with wrapped operation context.
- Preserve transaction rollback for the plan head and revision.
- Record the expected service rejection at debug level for both the upsert and
  revert write paths.
- Add SQLite coverage and environment-gated PostgreSQL parity coverage.

## Exclusions

- Do not add a separate task existence query before the transaction.
- Do not change successful plan coalescing or revision numbering.
- Do not change other foreign-key mappings.

## Traceability

- `REQ-TASKS-DOCUMENTS-001`
- `AC-TASKS-DOCUMENTS-001.2`
- Design: `docs/specs/tasks/system-design/plan-write-lifecycle.md`

## Acceptance

- A plan write for a missing task returns an error that matches
  `repoerrors.ErrTaskNotFound` on SQLite and PostgreSQL.
- The rejected transaction creates no plan head or revision.
- The plan service emits a debug entry for that sentinel from both
  `upsertPlan` and `RevertPlan`. Other write errors retain
  their error-level entry. Existing create/update coverage remains intact.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestWritePlanRevisionMissingTask|TestPostgresWritePlanRevisionMissingTask' -count=1
cd apps/backend && go test ./internal/task/service -run 'TestPlanService(MissingTaskWriteLogSeverity|OtherWriteErrorLogSeverity|RevertMissingTaskWriteLogSeverity|RevertOtherWriteErrorLogSeverity)' -count=1
```

The repository test must fail before the production change because the current
error exposes the raw foreign-key constraint failure.

The PostgreSQL case skips when `KANDEV_TEST_POSTGRES_DSN` is not set. CI runs
the environment-gated parity suite.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/plan.go`
- `apps/backend/internal/task/repository/sqlite/plan_test.go`
- `apps/backend/internal/task/repository/sqlite/document_plan_postgres_test.go`
- `apps/backend/internal/task/service/plan_service.go`
- `apps/backend/internal/task/service/plan_service_logging_test.go`

## Dependencies

None.

## Results

Implemented dialect-aware foreign-key classification at the plan-head write
boundary. Missing-task writes now return `repoerrors.ErrTaskNotFound`, roll
back both plan tables, and log at debug in `PlanService`; unrelated write
errors retain error-level logging. Both `upsertPlan` and `RevertPlan` use
the same classifier, with observer-backed coverage for the missing-task and
unrelated-error paths. Added SQLite, environment-gated PostgreSQL, and service
severity coverage without removing create/update coverage.

Verification:

```text
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestWritePlanRevisionMissingTask|TestPostgresWritePlanRevisionMissingTask' -count=1
Go test: 1 passed in 1 packages

cd apps/backend && go test ./internal/task/service -run 'TestPlanService(MissingTaskWriteLogSeverity|OtherWriteErrorLogSeverity|RevertMissingTaskWriteLogSeverity|RevertOtherWriteErrorLogSeverity)' -count=1
Go test: 4 passed in 1 packages
```
