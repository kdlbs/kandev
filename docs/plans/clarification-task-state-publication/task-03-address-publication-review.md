---
id: "03-address-publication-review"
title: "Address ordered publication review findings"
status: completed
wave: 3
depends_on: ["01-publish-clarification-task-state"]
plan: "plan.md"
spec: "../../specs/tasks/runtime-state-publication-order.md"
---

# Task 03: Address ordered publication review findings

## Acceptance

- A clarification task-state event and its `RUNNING` session event remain in
  order when another publication for the same task is already draining.
- A task-service reconciliation error does not suppress the authoritative
  `session.state_changed(RUNNING)` event; a clean stale guard still suppresses
  the pair.
- Validation commands in Task 01 and Task 02 can be pasted and run from the
  repository root without accumulating `cd` state.

## Files

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/task/service/service_events.go`
- `docs/specs/tasks/runtime-state-publication-order.md`
- `docs/decisions/2026-07-30-runtime-task-state-before-running-event.md`
- `docs/plans/clarification-task-state-publication/plan.md`
- `docs/plans/clarification-task-state-publication/task-01-publish-clarification-task-state.md`
- `docs/plans/clarification-task-state-publication/task-02-prove-live-sidebar-regrouping.md`

## Verification

```bash
(cd apps/backend && go test -count=1 -run 'Test(SetSessionRunning_PublishesTaskStateBeforeSession|SetSessionRunning_PreservesSessionEventOnTaskServiceError|SetSessionRunning_QueuesSessionAfterBusyTaskPublication)$' ./internal/mcp/handlers)
(cd apps/backend && go test -race -count=1 -run 'Test(SetSessionRunning_PublishesTaskStateBeforeSession|SetSessionRunning_PreservesSessionEventOnTaskServiceError|SetSessionRunning_QueuesSessionAfterBusyTaskPublication)$' ./internal/mcp/handlers)
```

## Parallelism

Sequential. The handler implementation and both regression tests share the same
publication lifecycle seam.

## Results

- RED: the busy-queue regression observed `task.updated`, then
  `task_session.state_changed`, then `task.state_changed`; the task-service
  error regression emitted no session event.
- GREEN: normal and race-focused handler tests passed for all three
  `setSessionRunning` scenarios.
- GREEN: full `internal/mcp/handlers` and `internal/task/service` packages
  passed in normal and race modes.
- GREEN: changed-file `golangci-lint` passed with no issues.
