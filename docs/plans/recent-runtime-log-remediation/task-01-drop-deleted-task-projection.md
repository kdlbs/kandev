---
id: "01-drop-deleted-task-projection"
title: "Drop deleted task projection state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.11
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---

# Task 01: Drop deleted task projection state

## Summary

A late queue-status event can reach a projector with a cached workspace after
task deletion. Treat authoritative not-found from the launch-queue loader as
deletion and drop the cached state without publishing an update.

## In scope

- Add a failing cached-workspace regression, then handle verified not-found at
  `LoadLaunchQueue` in `Projector.handleEvent` using the repository's typed
  `ErrTaskNotFound` sentinel.
- Keep transient loader failures observable as errors.
- Return the same typed sentinel from gateway task lookup fallbacks that receive
  a nil task with no error.

## Out of scope

- Changing task deletion order or generic projection error handling.

## Acceptance

- Deleted-task queue event returns nil, publishes nothing, and retains no state.
- Wrapped `ErrTaskNotFound` is treated as deletion; a different error whose
  text contains "not found" still propagates and retains cached state.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/statussummary -run 'TestProjectorQueueEvent' -count=1)
```

## Files likely touched

- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_queued_test.go`

## Dependencies

None.

## Risks

- Text-based not-found checks can suppress transient failures. Use only
  `errors.Is(err, repoerrors.ErrTaskNotFound)` for the deletion decision.

## Parallelism

`sequential`

## Inputs

- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.11` and its system design.
- Existing cold-task and warm-projector regressions in `projector_queued_test.go`.

## Results

Implemented the cached-workspace deletion guard with typed sentinel matching,
and made gateway nil-task fallbacks return that sentinel. Wrapped sentinel
errors are ignored as deletion while a driver error containing "task not found"
propagates and retains state. Verification passed:

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/statussummary -run 'TestProjectorQueueEvent' -count=1)
```
