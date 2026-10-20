---
id: "01-queue-busy-parent-auto-start"
title: "Queue the auto-start for a busy parent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-001
acceptance_criteria:
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-001.1
system_design:
  - ../../specs/tasks/system-design/subtask-completion-busy-parent-wake.md
---

# Task 01: Queue the Auto-Start for a Busy Parent

## Summary

Honor `QueueIfBusy` in `autoStartAgentCallback`: queue the step prompt when the
session is mid-turn, and on a busy launch error, so the queue delivers it after
the turn.

## In scope

- `autoStartAgentCallback.queueStepPromptIfBusy` and `isBusyLaunchError`.
- `handleCreatedAutoStartLaunchFailure` reuses `isBusyLaunchError`.

## Out of scope

- The `on_enter` auto-start path.

## Acceptance

1. A parent running a turn when its last child completes is not prompted
   mid-turn and gets exactly one queued step prompt, delivered after the turn,
   even when the completion is delivered twice.
2. The children-completed operation is marked applied once the prompt is
   queued.

## Verification

```bash
(cd apps/backend && go test -count=1 ./internal/orchestrator/ -run 'ChildrenCompleted|TerminalStep|AutoStart')
```

## Files likely touched

- `apps/backend/internal/orchestrator/{workflow_callbacks,event_handlers_workflow}.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed_busy_test.go`

## Dependencies

None.

## Parallelism

`sequential`

## Results

Author-reported: the new test fails on `main` at `77247fa20` (operation not
applied) and passes with the change.
