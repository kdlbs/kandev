---
id: "02-terminal-stall-teardown"
title: "Tear down a never-started execution"
status: done
wave: 2
depends_on: ["01-honest-inactivity-clock"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
system_design: "../../specs/agents/system-design/agent-stall-recovery.md"
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.10
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.11
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.7
---

# Task 02: Tear down a never-started execution

## Outcome

`handleAgentStalled` disposes of the process it declares dead. After the
never-started branch runs, the session and task are `FAILED` with the
launch-failure message and no agent process remains for that session.

## In scope

- In the `NeverStarted` branch of
  `apps/backend/internal/orchestrator/event_handlers_stall.go`, after
  `recordSessionLaunchFailure`, stop the execution named by
  `payload.AgentExecutionID` through `agentManager.StopAgentWithReason` with
  `force: true` and a stable reason.
- Order is record-then-tear-down, per the system design: the recorded `FAILED`
  state stays authoritative even if teardown fails.
- Use `context.WithoutCancel` for the stop so a cancelled request context cannot
  abandon the teardown midway.
- Treat `lifecycle.ErrExecutionNotFound` as success — nothing is running — and
  log any other failure at warn with task, session, and execution identity.
- Skip the stop when `payload.AgentExecutionID` is empty.

## Exclusions

- Do not route through `StopByTaskID` or any path that writes session state; it
  would replace `FAILED` with a cancellation state and destroy the error message
  the requirement asks the user to see.
- Do not change the advisory branch. A stall after genuine activity must still
  leave task state, session state, prompt admission, and process liveness
  untouched.
- Do not change the notice copy, metadata, or the ownership guards that precede
  the branch.

## Acceptance conditions

1. A never-started stall stops the execution exactly once, with force, after the
   session and task are recorded `FAILED`.
2. The session's final state is `FAILED` with the launch-failure message; no
   cancellation state is written by this path.
3. A teardown failure is logged and does not change the recorded state or
   re-post the notice; an advisory stall still stops nothing.

## Verification

```sh
cd apps/backend && go test ./internal/orchestrator/... -count=1
```

New regressions in
`apps/backend/internal/orchestrator/event_handlers_stall_test.go`:

- `TestHandleAgentStalled_NeverStartedStopsExecution` — a fake agent manager
  records stop calls; asserts one forced stop for the payload's execution ID and
  a final session state of `FAILED`. It must first fail with zero stop calls.
- `TestHandleAgentStalled_NeverStartedKeepsFailedStateWhenStopFails` — the fake
  returns an error; asserts the session stays `FAILED` with the launch-failure
  message.
- `TestHandleAgentStalled_AdvisoryStallStopsNothing` — asserts zero stop calls
  for a stall with `NeverStarted` false.

These existing regressions must keep passing unchanged:
`TestHandleAgentStalled_PersistsNeutralRunningNotice`,
`TestHandleAgentStalled_NeverStartedFailsSessionAndTask`,
`TestHandleAgentStalled_RejectsActivityEpochChangedAfterSnapshot`,
`TestHandleAgentStalled_RejectsSettledOrStalePrompt`.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_stall.go`
- `apps/backend/internal/orchestrator/event_handlers_stall_test.go`

## Dependencies

Task 01. Without the honest clock the never-started branch is not reliably
reachable on schedule, so its teardown cannot be exercised against the real
condition.

## Results

Added `Service.stopNeverStartedExecution` (`event_handlers_stall.go`), called
after `recordSessionLaunchFailure` inside the `payload.NeverStarted` branch of
`handleAgentStalled`. It skips when `payload.AgentExecutionID` is empty, stops
via `s.agentManager.StopAgentWithReason` with `force: true` on a
`context.WithoutCancel(ctx)`-detached context and a stable reason, treats
`lifecycle.ErrExecutionNotFound` as success, and logs any other failure at
warn with task/session/execution identity. The advisory branch and its
notice/metadata are untouched.

Added the three specified regressions to `event_handlers_stall_test.go`:
- `TestHandleAgentStalled_NeverStartedStopsExecution` — failed first with zero
  stop calls, now asserts exactly one forced stop for the payload's execution
  and a final `FAILED` session state.
- `TestHandleAgentStalled_NeverStartedKeepsFailedStateWhenStopFails` — fake
  `StopAgentWithReason` returns an error; session stays `FAILED` with
  `errAgentNeverStarted`'s message.
- `TestHandleAgentStalled_AdvisoryStallStopsNothing` — `NeverStarted: false`
  asserts zero stop calls.

The four existing regressions named in the work order
(`TestHandleAgentStalled_PersistsNeutralRunningNotice`,
`TestHandleAgentStalled_NeverStartedFailsSessionAndTask`,
`TestHandleAgentStalled_RejectsActivityEpochChangedAfterSnapshot`,
`TestHandleAgentStalled_RejectsSettledOrStalePrompt`) pass unchanged.

Verification: `go test ./internal/orchestrator/... -count=1` is green for
every `TestHandleAgentStalled_*` test, run both individually and as part of
the full package, repeatedly. Full-package runs on this host are noisy: the
sandbox disk is at 99% capacity (~12Gi free), and a full run intermittently
fails a handful of unrelated tests elsewhere in the package (git/CI
automation, workflow clarification, task launch) with `disk I/O error: no
space left on device` / `sql: database is closed` / timing-sensitive
assertions — a different random subset each run. Every test that failed in a
full run passed cleanly when re-run in isolation, and none touches
`handleAgentStalled`, `stopNeverStartedExecution`, or stall detection. This is
a pre-existing environment condition, not a regression from this change.
