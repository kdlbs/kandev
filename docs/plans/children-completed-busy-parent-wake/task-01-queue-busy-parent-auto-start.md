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
session is mid-turn, and when a launch is refused as busy before the agent
accepts the prompt, so the queue delivers it after the turn.

## In scope

- `autoStartAgentCallback.queueStepPromptIfBusy`, `isPromptRejectedAsBusy`,
  and `drainIfTurnEnded`.
- `withStepHandoffOnce` / `claimStepHandoffTextForLaunch`, used by
  `StartSessionForWorkflowStep`.

## Out of scope

- The `on_enter` auto-start path.

## Acceptance

1. A parent running a turn when its last child completes is not prompted
   mid-turn, gets exactly one queued step prompt delivered after the turn (even
   when the completion is delivered twice or the turn ends while the prompt is
   being queued), and the children-completed operation is marked applied.
2. A launch refused as busy before the agent accepts the prompt is queued when
   a turn is running and launched again when it has ended; a failure after
   acceptance is returned without queueing.
3. A step handoff claimed by a refused launch is carried by the retry or the
   queued prompt.

## Verification

```bash
(cd apps/backend && go test -count=1 ./internal/orchestrator/ -run 'ChildrenCompleted|TerminalStep|AutoStart')
```

## Files likely touched

- `apps/backend/internal/orchestrator/{workflow_callbacks,service,step_handoff_carry,task_operations,event_handlers_workflow,dynamic_launch,resume_attempt,workflow_auto_start_admission,workflow_prompt_fallback}.go`
- `apps/backend/internal/orchestrator/workflow_callbacks_busy_{consumer,guard,terminal}_test.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed_busy_test.go`

## Dependencies

None.

## Parallelism

`sequential`

## Results

Author-reported: the busy-parent test fails on `main` at `77247fa20`
(operation not applied) and passes with the change; the turn-ending test fails
without the post-queue drain (wake never dispatched) and passes with it.


## Contributor review remediation

The busy queue path shares the direct launch's prompt authorization, profile,
and approval checks.
One cancellation revision and session incarnation span the callback's attempts
and queue insertion. The callback also retains an already-claimed initial
prompt fallback across pre-admission busy rejection, alongside the step handoff.
A busy retry stops before terminal-session recovery, and guard ownership stays
scoped to the session that holds it. No repository claim policy, retry limit,
initial/manual recovery policy, or session concurrency policy changes.

Consumer regressions also cancel during actual runtime readiness before resume
registration and verify that neither the ordinary nor already-held registration
guard admits stale startup. The ordinary path asserts no runtime resume binding.

Consumer regressions cover real launch/prompt rejection, cancellation after a
busy rejection and before queue insertion, replaced session incarnation,
replacement during runtime readiness before handoff consumption (including
supported same-ID recreation under another task after retirement), inherited
child guard authority at actual parent resume registration, profile mismatch,
pending approval and denied prompt authorization, SQLite queue
merging, retained fallback and handoff text, and concurrent duplicate children
completion.

Review validation in the combined current-base tree passed the focused
orchestrator scope and all workflow packages. Scoped race checks passed for
the callback consumers, concurrent busy wake, execution-less stop settlement,
and completed teardown ownership. Focused turn-change start-failure and
startup-model consumer checks also passed. Delivery and exact-head CI/review
gates remain external.
