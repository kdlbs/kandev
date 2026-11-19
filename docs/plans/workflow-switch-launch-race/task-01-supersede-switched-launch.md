---
id: "01-supersede-switched-launch"
title: "Supersede a launch that a profile switch parked"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.17
system_design:
  - ../../specs/tasks/system-design/workflow-switch-launch-supersede.md
---

# Task 01: Supersede a launch that a profile switch parked

## Summary

A launch that finds its session parked by a profile switch exits as
superseded, and the switch completes on the step's profile.

## In scope

- `executor.SessionStateSwitchedError`, `ErrSessionStateSwitched`, and
  `IsSessionStateSwitched`.
- Parking detection in `persistSessionFullRowIfCurrentState` and
  `persistFullTaskSessionIfCurrent`.
- Superseded handling in `handleEarlyLaunchFailure`, `persistLaunchState`,
  `launchPreparedSession`, `handleSessionLaunchFailure`, the background launch
  of `PrepareTaskSession`, and `IsBenignLaunchTeardownErr`.

## Out of scope

- Switch and parking policy, manual recovery actions, automatic retries, and
  the frontend.

## Acceptance

1. With a real task move while a launch is blocked at its runtime persistence,
   the source session stays `WAITING_FOR_INPUT`, the task is not `FAILED`, and
   the destination session is primary with the step's profile. The replacement
   agent starts.
2. A launch that returns an error after a new source parking stamp keeps the
   workspace claim and yields as superseded.
3. An unexpected state without a valid source parking record, or with a record
   for another session, remains a launch failure.

## Verification

```bash
# from the repository root
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  -run 'WorkflowPark|WorkflowSwitch|UnexpectedStateWithoutSwitch|ForeignParkingMarker' \
  ./internal/orchestrator/ ./internal/orchestrator/executor/)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/orchestrator/ \
  ./internal/orchestrator/executor/)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

## Likely files

- `apps/backend/internal/orchestrator/executor/{executor,executor_execute,executor_resume}.go`
- `apps/backend/internal/orchestrator/executor/executor_workflow_switch_race_test.go` (new)
- `apps/backend/internal/orchestrator/{event_handlers_streaming,session_launch,task_operations}.go`
- `apps/backend/internal/orchestrator/workflow_switch_launch_race_test.go` (new)

## Results

- The new tests pass. With the four behavior files reverted to the previous
  code, `TestPersistFullTaskSessionIfCurrent_WorkflowParkIsSuperseded`,
  `TestPersistSessionFullRowIfCurrentState_WorkflowParkIsSuperseded`,
  `TestHandleSessionLaunchFailure_WorkflowSwitchSupersededKeepsTaskActive`,
  and `TestPrepareTaskSession_RealWorkflowSwitchDuringLaunchCompletes` fail.
- Full `internal/orchestrator` and `internal/orchestrator/executor` runs on the
  Windows development host fail the same tests before and after the change
  (Windows workspace inventory, Kubernetes, and GitHub credential bridge
  fixtures in the executor package; two passthrough-attachment tests in the
  orchestrator package). `TestCompleteTurnsExceptReportsIterationExhaustion`
  and `TestFreshStartWorkflowContext` fail intermittently on both sides.
- The launch ownership regressions pass on the Linux workspace, including the
  late `LaunchAgent` failure path, unchanged `WAITING_FOR_INPUT` parking stamp,
  MCP/WS handoff, and the real task move with replacement agent start.
- `go test -trimpath -tags fts5 -count=1 ./internal/task/repository/sqlite`
  passed. `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, `git diff --check`, and changed
  Go file lint passed.
