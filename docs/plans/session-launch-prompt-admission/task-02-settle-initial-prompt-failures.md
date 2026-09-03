---
id: "02-settle-initial-prompt-failures"
title: "Settle initial prompt failures"
status: complete
wave: 2
depends_on:
  - "01-admit-launch-attachments"
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.9
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.10
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 02: Settle Initial Prompt Failures

## Summary

Route every genuine initial-prompt error through the terminal execution path.
Settle only the execution and prompt that own the error.

## In scope

- Carry the dispatch error through the initial-prompt callback.
- Wire the callback in the lifecycle manager constructor.
- Publish the existing correlated `agent.failed` result.
- Keep shutdown teardown non-failing.
- Add race-aware lifecycle and orchestrator tests.

## Out of scope

- Attachment claim admission.
- Provider-specific retry policy.
- Timeout changes for quiet turns.

## Acceptance

- A materialization or ACP submission error fails the current execution.
- The active turn and session receive a durable terminal result without paths
  or provider secrets.
- A stale or duplicate error cannot fail replacement work.

## Verification

```bash
cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(DispatchInitialPromptReportsDeliveryFailure|InitialPromptFailureMarksExecutionFailedAndReleasesActivity|InitialPromptFailureCannotFailReplacementExecution)' -count=1
make -C apps/backend test
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/activity_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_attachments_test.go`

## Dependencies

- Task 01 establishes the attachment admission boundary.

## Risks

- The callback can race with process exit and execution replacement.
- A shutdown transport error can create a false durable failure.
- The terminal event must retain current prompt evidence for stale-event checks.

## Parallelism

`sequential`

## Inputs

- Task launch failure requirements and system design.
- ADR `2026-08-18-never-started-agent-stall-terminal`.
- Existing `MarkCompleted` and `agent.failed` correlation behavior.

## Results

- Carried the asynchronous prompt delivery error through the session callback.
- Wired the lifecycle manager to settle that execution through `MarkCompleted`.
- Verified failure state, activity release, and exact execution identity.
- Verified a late callback for a removed execution does not affect its replacement.
- Reused the existing shutdown-aware and duplicate-terminal guards.
