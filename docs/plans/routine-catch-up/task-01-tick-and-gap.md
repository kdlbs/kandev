---
id: "01-tick-and-gap"
title: "Tick, claim, and gap durability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-CATCHUP-001
  - REQ-OFFICE-ROUTINE-CATCHUP-002
acceptance_criteria:
  - AC-OFFICE-ROUTINE-CATCHUP-001.1
  - AC-OFFICE-ROUTINE-CATCHUP-001.2
  - AC-OFFICE-ROUTINE-CATCHUP-001.3
  - AC-OFFICE-ROUTINE-CATCHUP-001.4
  - AC-OFFICE-ROUTINE-CATCHUP-001.5
  - AC-OFFICE-ROUTINE-CATCHUP-001.6
  - AC-OFFICE-ROUTINE-CATCHUP-001.7
  - AC-OFFICE-ROUTINE-CATCHUP-001.8
  - AC-OFFICE-ROUTINE-CATCHUP-001.9
  - AC-OFFICE-ROUTINE-CATCHUP-001.10
  - AC-OFFICE-ROUTINE-CATCHUP-001.11
  - AC-OFFICE-ROUTINE-CATCHUP-001.12
  - AC-OFFICE-ROUTINE-CATCHUP-001.13
  - AC-OFFICE-ROUTINE-CATCHUP-002.1
  - AC-OFFICE-ROUTINE-CATCHUP-002.2
  - AC-OFFICE-ROUTINE-CATCHUP-002.3
  - AC-OFFICE-ROUTINE-CATCHUP-002.4
  - AC-OFFICE-ROUTINE-CATCHUP-002.5
  - AC-OFFICE-ROUTINE-CATCHUP-002.6
  - AC-OFFICE-ROUTINE-CATCHUP-002.7
  - AC-OFFICE-ROUTINE-CATCHUP-002.8
  - AC-OFFICE-ROUTINE-CATCHUP-002.9
  - AC-OFFICE-ROUTINE-CATCHUP-002.10
  - AC-OFFICE-ROUTINE-CATCHUP-002.11
  - AC-OFFICE-ROUTINE-CATCHUP-002.12
system_design:
  - ../../specs/office/system-design/routine-catch-up-01.md
---

# Task 01: Tick, claim, and gap durability

## Summary

Make a cron trigger due while the backend is down produce exactly one run on
resume, whatever the gap size, and give that gap a durable, observable record.

## In scope

- `apps/backend/internal/office/routines/service.go`: `computeCatchUp`
  (replaces `computeRoutineMissed`), `buildGapSummary`, `processCronTrigger`,
  `reconcileStrandedTriggers`, `CreateRoutineTrigger` cron-satisfiability
  validation.
- `apps/backend/internal/office/models/catchup.go`: `NormaliseCatchUpMax`
  clamp [1, 1000]; `RoutineRun.CatchUpMissedTicks` /
  `CatchUpFirstMissedAt` / `CatchUpTruncated`.
- `apps/backend/internal/office/repository/sqlite/{routines.go,
  catchup_migrations.go,catchup_migrations_postgres.go,wakeup_requests.go}`:
  schema default, idempotent `ADD COLUMN`, table-rebuild migration (SQLite) /
  dialect-gated `ALTER TABLE ... SET DEFAULT` (Postgres); `GetDueTriggers`
  ordering; `ListStrandedTriggers` / `ReconcileTriggerNextRun`.
- `apps/backend/internal/office/service/{prompt_builder.go,
  scheduler_integration.go}`: `appendMissedTicksSection`,
  `applyRoutineCatchUpContext`.

## Acceptance

- A cron trigger with any elapsed-tick count on resume dispatches exactly one
  run; `catch_up_max` bounds only how many ticks are counted.
- The gap (missed-tick count, first-missed timestamp, truncated flag) is
  stored durably on the created `RoutineRun`, written once at creation so a
  run later marked skipped, coalesced, or failed still carries it.
- A cron trigger of kind "cron" is never persisted with a null `next_run_at`
  at creation; a claim left unarmed by a process stopping mid-tick is
  reconciled by a later tick without dispatching a spurious run.
- When the elapsed-tick computation fails, `next_run_at` arms to the
  processing instant plus 24 hours with a warning naming the trigger and the
  underlying error; a syntactically valid but permanently unsatisfiable
  expression is instead left disarmed rather than retried forever.

## Verification

```bash
cd apps/backend && go test ./internal/office/routines/... ./internal/office/repository/sqlite/... -count=1
cd apps/backend && go run ./cmd/sqlguard ./internal
cd apps/backend && KANDEV_TEST_POSTGRES_DSN=... go test ./internal/office/repository/sqlite/... -run Postgres -count=1
```

## Files likely touched

- `apps/backend/internal/office/routines/service.go`
- `apps/backend/internal/office/models/catchup.go`
- `apps/backend/internal/office/repository/sqlite/routines.go`
- `apps/backend/internal/office/repository/sqlite/catchup_migrations.go`
- `apps/backend/internal/office/repository/sqlite/catchup_migrations_postgres.go`
- `apps/backend/internal/office/repository/sqlite/wakeup_requests.go`
- `apps/backend/internal/office/service/prompt_builder.go`
- `apps/backend/internal/office/service/scheduler_integration.go`

## Dependencies

None.

## Parallelism

Sequential with task 02, which renames the policy this task's dispatch path
reads.

## Results

Implemented with TDD across the build history recorded in this task's Kandev
plan: the tick/claim/reconciliation path, the gap-summary funnel, the
SQLite/Postgres schema and migrations, and the prompt/wakeup payload fields.
Full `internal/office` suite green; a real PostgreSQL run proved the
dialect-gated default-policy migration.
