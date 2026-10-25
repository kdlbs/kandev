---
id: "02-implement-atomic-overflow-placement"
title: "Implement atomic overflow placement"
status: completed
wave: 2
depends_on:
  - "01-persist-wip-admission-state"
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 02: Implement Atomic Overflow Placement

## Acceptance

- Creation into an available limited step admits the task normally.
- Creation into a full step with no feeder succeeds as same-step queued work.
- Creation into a full step with an available feeder succeeds in that feeder
  and records the requested destination.
- A full configured feeder causes a typed WIP conflict and no partial insert.
- Destination and feeder capacity checks plus insertion are one transaction,
  with stable row-lock order for PostgreSQL and serialized SQLite writes.
- Concurrent WIP-2 creation persists every no-feeder request while admitting
  exactly two; feeder creation never exceeds either step's admitted capacity.
- Unlimited and ephemeral task creation remain compatible.

## TDD sequence

1. Replace the old concurrent rejection expectation with a failing
   admit-two/queue-rest test.
2. Add failing explicit-step and resolved-start-step tests for same-step and
   feeder overflow.
3. Add failing full-feeder rollback and concurrent-dialect tests.
4. Implement the repository admission result and task-service placement.
5. Keep conflict mapping for the full-feeder terminal case.

## Verification

```bash
cd apps/backend
go test -tags fts5 -race ./internal/task/repository ./internal/task/service -run 'Test.*(OverflowPlacement|SameStepQueue|FeederQueue|ConcurrentWIP|FullFeeder)' -count=1
```

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/workflow/models/errors.go`
- repository and service WIP tests

## Dependencies

- Task 01 supplies persisted admission state and admitted-count queries.

## Parallelism

`sequential`

## Output contract

Record the final repository API, transaction/lock order for both dialects,
concurrency evidence, and exact verification result. Do not implement launch or
watcher behavior in this task.
