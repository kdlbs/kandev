---
spec: docs/specs/agents/requirements/agent-stall-recovery.md
created: 2026-08-19
status: done
---

# Implementation Plan: Lost Turn-Completion Cancel Recovery

## Overview

Issue [#2823](https://github.com/kdlbs/kandev/issues/2823) reports a session
that cannot accept prompts after one completion event is lost. A focused Go
reproduction confirms the root cause. Cancel escalation drains its synthetic
completion but leaves the dispatch-only prompt gate set.

This plan clears that gate at the cancellation boundary. It also makes the gate
state safe for access from prompt and cancellation goroutines.

## Backend

### Prompt gate state

- In `apps/backend/internal/agent/runtime/lifecycle/types.go`, change
  `dispatchedPromptPending` to `atomic.Bool`.
- In `apps/backend/internal/agent/runtime/lifecycle/session.go`, use atomic load
  and store operations in `waitForPendingDispatchedPrompt` and
  `finishAcceptedPrompt`.
- In `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`,
  route cancellation through the same escalation cleanup when a pending
  dispatch gate has no live prompt barrier, then clear the gate before
  `escalateStuckCancel` sends its synthetic completion signal. Escalation skips
  the waiter timeout when no prompt barrier exists.

The cancellation path must clear the old gate before it releases a waiter.
This order prevents a stale cleanup from clearing a successor dispatch.

## Tests

- **What:** A dispatch-only prompt has no completion. Cancellation escalates.
  The next prompt reaches agentctl without a backend restart.
- **File:**
  `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_cancel_test.go`
- **How:** Use the existing agentctl WebSocket test server and the real
  `Manager.CancelAgent` path. The regression test must fail before the code
  change because the follow-up prompt waits until its context expires.
- **Concurrency:** Run the focused lifecycle tests with the Go race detector.

## Verification Results

- RED: `TestManager_CancelAgent_ClearsPendingDispatchGate` failed because the
  follow-up prompt returned `context deadline exceeded` after cancellation.
- GREEN: `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run
  'TestManager_CancelAgent_(EscalatesWhenAgentHangs|ClearsPendingDispatchGate)'
  -count=1` passed with 2 tests.
- GREEN: `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run
  'TestSendPrompt_DispatchOnly' -count=1` passed with 2 tests.
- GREEN: `git diff --check` passed.
- Review remediation: cancellation now escalates when the real dispatch-only
  path has a nil or stale closed prompt barrier, and the regression asserts the
  gate is clear before the follow-up prompt.
- Review remediation checks: the cancel race pair passed with 2 tests, the
  dispatch-only regression passed with 2 tests, `gofmt -l` reported no files,
  and `git diff --check` passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-clear-stale-dispatch-gate](task-01-clear-stale-dispatch-gate.md)

Execution is sequential. The task changes one shared lifecycle state machine
and is not safe for parallel implementation.

## Risks

- Cancellation and prompt dispatch run on different goroutines. Non-atomic gate
  access can introduce a data race.
- If cancellation clears the gate after it releases a prompt waiter, it can
  clear state that belongs to a successor dispatch.

## Out of Scope

- Automatic turn cancellation based only on elapsed time.
- A fixed timeout for legitimate long-running dispatch-only turns.
- New conversation messages for dispatch errors.
- Changes to the stall detector or ACP adapter completion fallback.
