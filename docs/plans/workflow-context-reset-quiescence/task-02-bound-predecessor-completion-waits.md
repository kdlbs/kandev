---
id: "02-bound-predecessor-completion-waits"
title: "Bound predecessor completion waits"
status: done
wave: 2
depends_on:
  - "01-quiesce-active-reset-turns"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 02: Bound Predecessor Completion Waits

## Summary

End a stale dispatch-only predecessor wait within a fixed interval. Preserve the pending gate so cancellation remains its only safe release owner.

## In scope

- Add a typed lifecycle error for a stale predecessor wait.
- Add a 10-second timeout to the pending dispatch-only barrier.
- Leave `dispatchedPromptPending` set after timeout.
- Treat the new error as transient in orchestrator prompt handling.
- Cover timeout, caller cancellation, signal arrival, and error classification.

## Out of scope

- Automatic cancellation when the timeout expires.
- Changes to normal prompt completion duration.
- A new user-facing error message or UI state.

## Acceptance

- A stale predecessor wait returns before it can hold session admission forever.
- The timed-out successor never reaches agentctl and never clears predecessor ownership.
- Existing cancellation escalation can clear the gate after the timeout releases session admission.

## Verification

```bash
go test ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(WaitForPendingDispatchedPrompt|IsTransientPromptError_PendingCompletionTimeout)' -count=1
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/errors_test.go`

## Dependencies

Task 01 establishes the primary active-reset correction.

## Risks

- A timeout that clears the gate can admit concurrent active prompts. The implementation must not clear it.
- A non-transient classification can drop an automatic step prompt instead of preserving it.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002`
- `docs/specs/tasks/system-design/workflow-step-agent-start-ownership.md`
- `docs/decisions/0035-version-agent-ready-events-by-prompt-generation.md`
- Existing cancellation escalation in `manager_interaction.go`

## Results

Implemented.

- Added `PendingDispatchedPromptTimeoutError` and a 10-second pending-completion bound.
- Preserved `dispatchedPromptPending` on timeout and caller cancellation; completion signals still clear it.
- Classified the typed timeout as transient so queued workflow prompts remain retryable.
- Added lifecycle barrier tests in `session_pending_prompt_test.go` and orchestrator classification coverage in `errors_test.go`.
- Rejected unnumbered completion events while a numbered dispatch-only prompt is pending, so delayed synthetic completion cannot release its gate.
- Made the timeout test use a nonblocking post-`synctest.Wait` assertion and `t.Cleanup` for cancellation.
- Red: the pre-fix stale barrier did not return and the typed timeout was not transient.
- Green: the exact verification command passed 4 tests.
