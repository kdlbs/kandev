---
id: "01-launch-queue-refresh-sources"
title: "Refresh the launch queue only from its sources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.15
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---

# Task 01: Refresh the launch queue only from its sources

## Summary

The task status summary projector re-reads the launch queue only for events
that can change it, plus a task whose queue has not been observed.

## In scope

1. Add tests in `task/statussummary` before the change, counting
   `LoadLaunchQueue` reads through the existing projector config:
   - After the queue is observed and persisted, `message.added`,
     `message.updated`, `turn.started`, `turn.completed`, and
     `task_session.activity_changed` do not read it, and the stored capacity
     stays at the last observation while the authoritative queue moves
     (baseline: one read each). A following `task.updated` reads it once and
     persists the moved capacity.
   - `task.created`, `task.updated`, `task.state_changed`,
     `task_session.state_changed`, and `message.queue.status_changed` each
     read it once and persist a changed queue.
   - The first event for a cold task observes the queue once (baseline: twice,
     restore plus the event).
2. Add a closed `isLaunchQueueRefreshEvent` set next to the existing refresh
   classifiers and gate the loader call in `handleEvent` with it.
3. Keep the missing-task handling for queue-status events unchanged.

## Out of scope

The loader, ceiling observation, sweep cadence, deferral records, summary
schema, and frontend.

## Acceptance

- The unrelated-event and cold-task tests fail on the baseline and pass after
  the change; the refresh-source tests pass on both.
- Existing projector and launch-queue tests still pass.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/task/statussummary -run '^TestProjectorLaunchQueue' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/statussummary -run '^Test(Projector|EqualLaunchQueue|LaunchQueueSummaryFromTask|TaskStatusSummaryLaunchQueue)' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestDeferCeilingRefusalPublishesTaskUpdated$' -count=1)
(cd apps/backend && go build ./... && go vet ./internal/task/statussummary/...)
```

## Files likely touched

- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_launch_queue_test.go`

## Dependencies

None.

## Risks

A future subscribed event that changes the queue must join the closed set.

## Results

Implemented on 2026-10-12 against `add376a2af`.

- RED (baseline projector, new tests): `message.added re-read the launch queue
  1 times, want 0` and `first event read the launch queue 2 times, want one
  observation`. The five refresh-source subtests passed on the baseline.
- GREEN: the three `^TestProjectorLaunchQueue` tests pass.
- Neighbours: 69 projector and launch-queue tests pass. The existing
  `TestDeferCeilingRefusalPublishesTaskUpdated` passes, covering the deferral
  write that publishes `task.updated`. The sweep's still-deferred publish is
  confirmed by source reading (`settleCeilingReplay`); no dedicated test covers it.
- `go build ./...` and `go vet ./internal/task/statussummary/` pass.
