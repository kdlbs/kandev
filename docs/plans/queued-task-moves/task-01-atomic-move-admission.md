---
id: "01-atomic-move-admission"
title: "Add atomic move admission"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 01: Add Atomic Move Admission

## Acceptance

- A repository move to a limited step atomically commits either admitted or
  destination-resident queued placement. A full target is a successful queued
  result, not `ErrWIPLimitExceeded`.
- Concurrent moves for the last slot admit no more than the configured limit;
  every other move remains durable in the target queue.
- Unlimited targets normalize to admitted with cleared queue metadata.
- `MoveTaskWithOptions` uses the committed admission result across ordinary,
  cross-workflow, active-session, approval, and MCP calls.
- Bulk moves fill available capacity and queue the remaining tasks instead of
  rejecting the batch during prevalidation.
- Moving a queued task clears its old queue/deferred-launch intent before the
  new target admission is committed.
- The move response and task update contain the committed `wip_admitted`,
  `queued_for_step_id`, and `queued_at` values.

## TDD Sequence

1. Add repository concurrency tests and service behavior tests. Run them RED
   against the current capacity conflict.
2. Implement the atomic admission repository method and narrow interface.
3. Route single and bulk moves through it, then refactor duplicate validation.
4. Run focused repository and service tests GREEN with the race detector.

## Verification

```bash
cd apps/backend
rtk go test -tags fts5 ./internal/task/repository/... \
  ./internal/task/service ./internal/orchestrator -count=1
rtk go test -tags fts5 ./internal/task/repository/sqlite \
  -run TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot -count=1
```

## Implementation Result

- Added `UpdateTaskWithWorkflowStepAdmission`, which locks/counts the target
  step and commits admitted or destination-resident queued placement in one
  transaction. Unlimited targets clear queue metadata.
- Single, cross-workflow, approval, MCP, HTTP, WebSocket, and bulk move paths
  use the committed admission result. Bulk moves queue overflow instead of
  rejecting the batch.
- Moving a queued task clears its old deferred state before applying the new
  target admission.
- Repository and service tests cover overflow, unlimited targets, the final
  slot race, event metadata, and bulk moves. Manual placement, admitted state,
  and queue-exit metadata now commit atomically.
- Lifecycle replay keeps move and promotion tokens pending when prerequisite
  loading fails, and promotion errors no longer terminate queue filling.

The focused implementation run was:

```text
rtk go test -tags fts5 ./internal/task/repository/... ./internal/task/service ./internal/orchestrator -count=1
Go test: 3150 passed in 5 packages

rtk go test -tags fts5 ./internal/task/repository/sqlite \
  -run TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot -count=1
Go test: skipped because KANDEV_TEST_POSTGRES_DSN is not set
```

The repository admission tests and the focused service/orchestrator move
tests also passed.

## Files Likely Touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/task_wip_test.go`
- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/task/service/service_workflow_test.go`
- `apps/backend/internal/task/service/service_workflow_bulk_test.go`

## Dependencies

None.

## Parallelism

`sequential`

## Output Contract

Record RED/GREEN evidence, the repository admission result shape, concurrency
coverage, bulk ordering, files changed, and exact command results. Update this
task and `plan.md` status in the same implementation conversation.
