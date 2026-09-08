---
id: "03-reconcile-queued-promotion"
title: "Reconcile queued promotion"
status: completed
wave: 3
depends_on:
  - "02-implement-atomic-overflow-placement"
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 03: Reconcile Queued Promotion

## Acceptance

- One reconciler handles same-step and feeder promotion for task-service and
  workflow-engine capacity changes.
- Promotion atomically claims one slot and one eligible task, clears queue
  metadata, and cannot over-admit under concurrent triggers.
- Destination-tagged tasks from shared feeders cannot cross-promote; legacy
  untagged feeder tasks remain eligible.
- Promotion order matches the spec.
- Move-out, archive, delete, WIP changes, feeder changes, and startup/recovery
  all trigger bounded idempotent reconciliation.
- Feeder promotion emits `task.moved`; same-step promotion emits a task update.
- Manually moving queued work away cancels its queue target before applying the
  normal destination WIP rule.

## TDD sequence

1. Add failing candidate-order, shared-feeder, and concurrent-claim tests.
2. Add failing trigger tests for every capacity-changing event.
3. Extract the duplicated pull logic behind one reconciliation service.
4. Implement event emission and startup batching.
5. Add manual relocation and deleted-destination cleanup tests.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/service ./internal/workflow/service ./internal/orchestrator -run 'Test.*(QueuePromotion|QueueReconcile|SharedFeeder|CapacityVacancy|StartupQueue)' -count=1
```

## Files likely touched

- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/orchestrator/workflow_store.go`
- backend startup/lifecycle wiring
- focused task, workflow, and orchestrator tests

## Dependencies

- Task 02 supplies the atomic admission result and queue records.

## Parallelism

`sequential`

## Output contract

Record the single reconciliation ownership boundary, trigger matrix, batching
policy, event behavior, files changed, and exact test output.
