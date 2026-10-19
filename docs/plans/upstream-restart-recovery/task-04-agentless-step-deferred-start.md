---
id: "04-agentless-step-deferred-start"
title: "Keep the deferred start on an agentless step"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DEFERRED-START-STEP-ENTRY-001
acceptance_criteria:
  - AC-TASKS-DEFERRED-START-STEP-ENTRY-001.1
  - AC-TASKS-DEFERRED-START-STEP-ENTRY-001.2
  - AC-TASKS-DEFERRED-START-STEP-ENTRY-001.3
system_design:
  - ../../specs/tasks/system-design/deferred-start-step-entry.md
---

# Task 04: Keep the deferred start on an agentless step

## Summary

A step move into a step without automatic start leaves the task's
deferred-launch record in place for the next agent step.

## In scope

- Write failing orchestrator tests first.
- In `autoStartTaskForLoadedStep`, skip `launchDeferredTask` for `task.moved`
  into a step without automatic start. This covers `handleTaskMoved`, which
  calls it with the loaded destination step, and `autoStartTaskForStep`.
- In `autoStartTaskForStep`, skip the early `launchDeferredTask` call for
  `task.moved`, and still attempt it when the step read fails.

## Out of scope

- Dependency resolution, queue promotion, and creation paths.
- Changes to deferred-launch record contents.

## Acceptance

- A `task.moved` entry into a step without automatic start launches nothing
  and keeps the record.
- A later entry into an automatic-start step consumes the record and launches
  once.
- An unreadable destination step still attempts the deferred launch.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'DeferredLaunch|AgentlessStep' -count=1)
```

## Likely files

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- Existing tests that call `autoStartTaskForLoadedStep`
- A source-adjacent `*_test.go` in `apps/backend/internal/orchestrator`

## Results

- RED before the fix: `TestTaskMovedIntoAgentlessStepKeepsDeferredStart` and
  `TestLoadedStepTaskMovedIntoAgentlessStepKeepsDeferredStart` launched the
  deferred start. `TestTaskMovedIntoUnreadableStepStillAttemptsDeferredStart`
  passed as a guard.
- GREEN after the fix: all three tests pass, and the
  `Deferred|AutoStart|TaskMoved|ForkPR|QueuePromot|Dependenc` group passes.
- Change from the original design: no `retainDeferredLaunch` argument. The
  decision sits in `autoStartTaskForLoadedStep`, because `handleTaskMoved`
  calls it directly for a task without a session; a flag set only in
  `autoStartTaskForStep` left that main path unchanged.
