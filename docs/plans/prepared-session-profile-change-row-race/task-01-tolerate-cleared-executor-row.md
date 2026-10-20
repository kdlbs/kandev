---
id: "01-tolerate-cleared-executor-row"
title: "Tolerate a cleared executor row on profile change"
status: planned
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SESSION-IDENTITY-001
  - REQ-OFFICE-ROUTING-001
acceptance_criteria:
  - AC-OFFICE-SESSION-IDENTITY-001.9
  - AC-OFFICE-ROUTING-001.3
system_design:
  - ../../specs/office/system-design/routing-02.md
---

# Task 01: Tolerate a Cleared Executor Row on Profile Change

## Summary

When `LaunchPreparedSession` replaces an execution bound to a different profile,
a missing `executors_running` row after the stop means the clear already
happened. The launch continues instead of failing.

## In scope

- Add a test-local delete hook to the executor mock repository
  (`deleteExecutorRunningFunc`), defaulting to the current `nil` return.
- Regression test `TestLaunchPreparedSession_ProfileChangeToleratesRowClearedByStop`:
  seed a prepared session with an `executors_running` row whose
  `ExecutionProfileID` is `profile-old` and `AgentExecutionID` is set; make
  `StopAgentWithReason` remove the row; make delete return
  `models.ErrExecutorRunningNotFound` when the row is absent. Launch with
  `profile-new`; expect no error and one `LaunchAgent` call.
- Companion test `TestLaunchPreparedSession_ProfileChangePropagatesOtherDeleteErrors`:
  delete returns a non-not-found error; expect the launch to fail with
  `clear previous execution profile` and no `LaunchAgent` call.
- In `executor_execute.go`, ignore `models.ErrExecutorRunningNotFound` from
  `DeleteExecutorRunningBySessionID` in the profile-change branch.

## Out of scope

- Other callers of `DeleteExecutorRunningBySessionID`.
- Office scheduler classification.

## Acceptance

- The regression test fails before the production change with
  `clear previous execution profile: executor running not found` and passes
  after it.
- Non-not-found delete errors still fail the launch.

## Verification

```bash
# From apps/backend:
go test ./internal/orchestrator/executor -run 'TestLaunchPreparedSession_ProfileChange' -count=1 -race
go test ./internal/orchestrator/executor/... -count=1
make lint
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_mocks_test.go`
- `apps/backend/internal/orchestrator/executor/executor_profile_change_test.go` (new)

## Dependencies

None.

## Risks

- See plan: match only `ErrExecutorRunningNotFound`.

## Parallelism

`sequential`

## Inputs

- Issue #4611 logs and the failing code excerpt.
- `routing-02.md` Session/executor binding guarantee.

## Results

_Pending implementation._
