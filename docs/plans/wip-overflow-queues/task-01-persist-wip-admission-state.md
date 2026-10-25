---
id: "01-persist-wip-admission-state"
title: "Persist WIP admission state"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 01: Persist WIP Admission State

## Acceptance

- Tasks persist `wip_admitted`, `queued_for_step_id`, and `queued_at` through
  SQLite and PostgreSQL-compatible schema paths.
- Existing active workflow tasks migrate as admitted without changing their
  visible step.
- Repository reads, task DTOs, boot payloads, and task events preserve the
  fields; missing legacy values normalize safely.
- Capacity counting has one repository helper that counts admitted,
  non-archived, non-ephemeral tasks.
- Queue-candidate lookup supports destination scoping and the deterministic
  order in the spec.

## TDD sequence

1. Add failing migration/repository round-trip tests for admitted, same-step
   queued, and feeder-queued tasks.
2. Add a failing legacy migration test proving existing workflow tasks become
   admitted.
3. Add failing DTO/event conversion tests.
4. Implement the schema, models, repository scans, indexes, and converters.
5. Refactor all WIP occupancy reads to the shared admitted-count helper without
   changing creation behavior yet.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/repository ./internal/task/dto ./internal/db -run 'Test.*(WIPAdmission|QueuedForStep|TaskQueueMigration|AdmittedCount)' -count=1
```

## Files likely touched

- `apps/backend/internal/db/migrations/**`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/converters.go`
- focused repository, migration, and DTO tests

## Dependencies

None.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed command passes. Record migration compatibility, exact schema/index
choices, files changed, and test results in this file and `plan.md`.
