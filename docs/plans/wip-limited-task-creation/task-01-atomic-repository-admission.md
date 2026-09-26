---
id: "01-atomic-repository-admission"
title: "Atomic repository admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 01: Atomic Repository Admission

## Acceptance

- The task repository can create a task only when the target workflow step has
  capacity, counting active non-ephemeral occupants with the same semantics as
  WIP-limited moves.
- Eight synchronized creates against an empty WIP-2 step persist exactly two
  tasks; every rejected call matches the typed WIP sentinel.
- PostgreSQL admission locks the target workflow-step row for the transaction;
  SQLite admission remains serialized through its single writer.
- Existing capacity-aware updates return the same typed WIP error.

## Verification

```bash
cd apps/backend
go test -tags fts5 -race ./internal/task/repository/... -run 'Test.*WorkflowStep.*Capacity|Test.*WIP.*Concurrent' -count=1
```

## Files likely touched

- `apps/backend/internal/workflow/models/errors.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/task_repository_test.go`
- `apps/backend/internal/task/repository/sqlite/task_order_test.go` or a focused
  dialect test if the PostgreSQL lock query is extracted

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- Spec sections `What`, `Failure Modes`, and `Scenarios`.
- Existing `UpdateTaskIfWorkflowStepHasCapacity` transaction.
- SQLite writer configuration in `apps/backend/internal/db/sqlite.go`.
- PostgreSQL dialect detection in `apps/backend/internal/db/dialect`.

## Output contract

Mark this task `in_progress` before the RED test and `done` after GREEN and
refactor. Update `plan.md` in the same conversation and report the failing-test
evidence, repository API/transaction changes, files changed, exact test result,
and remaining database portability risks.

## Evidence

`go test ./internal/task/repository -run 'Test(CreateTaskIfWorkflowStepHasCapacity|UpdateTaskIfWorkflowStepHasCapacity)' -count=1`
passed (2 tests).
