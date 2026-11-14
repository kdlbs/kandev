---
id: "01-lost-created-race"
title: "Treat a lost CREATED race as a no-op auto-start"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-STALE-CREATED-AUTOSTART-001
acceptance_criteria:
  - AC-TASKS-STALE-CREATED-AUTOSTART-001.1
  - AC-TASKS-STALE-CREATED-AUTOSTART-001.2
  - AC-TASKS-STALE-CREATED-AUTOSTART-001.3
  - AC-TASKS-STALE-CREATED-AUTOSTART-001.4
  - AC-TASKS-STALE-CREATED-AUTOSTART-001.5
  - AC-TASKS-STALE-CREATED-AUTOSTART-001.6
system_design:
  - ../../specs/tasks/system-design/stale-created-auto-start.md
legacy_specs: []
---

# Work order 01: Lost CREATED race

## Outcome

An auto-start launch that loses its CREATED race leaves the session and task
unfailed, starts no agent, and delivers the step prompt once only while the
task is still on the dispatching step.

## Changes

- `apps/backend/internal/orchestrator/executor/executor.go`: add
  `ErrSessionStateChangedBeforeStart`.
- `executor/executor_execute.go` and `event_handlers_streaming.go`: wrap the
  marker in the CREATED and WAITING_FOR_INPUT refusal of the guarded STARTING
  write.
- `task_operations.go`: `startCreatedSession` completes its turn and returns the
  marked error without `handleSessionLaunchFailure`.
- `event_handlers_workflow.go` and `auto_start_created_state_lost.go`:
  `launchCreatedAutoStartStepPrompt` routes a non-CREATED reload and a marked
  launch error to `continueAutoStartAfterCreatedStateLost`.
- `event_handlers_stale_created_autostart_test.go`: production-sequence tests.

## Acceptance criteria

- **AC-TASKS-STALE-CREATED-AUTOSTART-001.1:**
  `TestStaleCreatedAutoStart_LostRaceAfterStepMoveIsNotFailure` and
  `TestStaleCreatedAutoStart_LostRaceOnSameStepDeliversPromptOnce`.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.2:**
  `TestStaleCreatedAutoStart_ReplayAfterSessionWaitingDoesNotLaunch`.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.3:**
  `TestStaleCreatedAutoStart_LostRaceOnSameStepDeliversPromptOnce`.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.4:**
  `TestStaleCreatedAutoStart_LostRaceAfterStepMoveIsNotFailure` and
  `TestStaleCreatedAutoStart_ReplayAfterSessionWaitingDoesNotLaunch`.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.5:** All three tests assert no agent
  process start for the losing launch.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.6:** Existing
  `TestAutoStartCreatedLaunch_*` and `TestWorkflowAsyncStartFailure_*` tests.

## Validation

- `go test -tags fts5 ./internal/orchestrator/ -run 'TestStaleCreatedAutoStart|TestAutoStartCreatedLaunch|TestWorkflowAsyncStartFailure' -count=1`
- `go test -tags fts5 ./internal/orchestrator/executor/ -count=1`
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`


## Continuation review follow-up

The candidate now handles a fresh STARTING/RUNNING reload through the existing
busy-queue policy without losing the complete step input. Recorded-message,
attachment, reference, plan-mode and handoff metadata remain attached to that
input. A moved task keeps only its original taken handoff.

The original entry/session snapshot is reused at atomic queue insertion and at
normal/fresh provider admission. Existing ceiling bindings remain authoritative;
the callbacks also cover the supported unbound continuation path. Fresh fallback
checks the same admission under existing task-entry and cancellation guards
before resetting the original session. Superseded input is a no-op rather than
a new generic launch failure. Ordinary callers retain their existing behavior.

Root verification reproduced missing full input for both STARTING and RUNNING
reloads and stale dispatch after a step move on the original source. An extended
disposable fresh-fallback case then exposed late initial admission after an
explicit pause in the first correction. The shared continuation validator now
checks the existing original cancellation revision as well.

The corrected candidate passed all six disposable scenario groups and the
selected existing StaleCreated, AutoStartCreatedLaunch, WorkflowAsyncStartFailure,
handoff, cancellation-before-reset and successor-identity controls (11.351s).
The fixture uses actual repository/service/mock callback boundaries, not an
installed provider. No permanent test case was added. Independent source review
then found an entry/cancellation lock-order inversion against the existing
completion path. Fresh fallback now acquires cancellation before task-entry
admission, matching completion; both guards still cover reset and validation
and are released before provider I/O. The superseded race build was stopped
without a verdict. The final publication-checkout candidate passed the same
selected scenarios and existing controls with the race detector (40.988s),
after extracting unchanged workflow-entry validation into its own helper to
meet the existing cyclop/nestif limits. No checks or thresholds were weakened.
Independent source re-review confirmed the corrected lock order and I/O release
boundary, with no remaining required finding in that scoped review. Normal
commit hooks and updated-head hosted CI are separate publication results.
