---
id: "01-clear-stale-dispatch-gate"
title: "Clear stale dispatch gate"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 01: Clear Stale Dispatch Gate

## Acceptance

1. Cancel escalation clears a pending dispatch-only completion gate.
2. A prompt that follows the completed cancellation reaches agentctl without a
   backend restart.
3. Prompt and cancellation goroutines access the gate without a data race.

## Verification

```bash
cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestManager_CancelAgent_(EscalatesWhenAgentHangs|ClearsPendingDispatchGate)' -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestSendPrompt_DispatchOnly' -count=1
git diff --check
```

## Files Likely Touched

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_cancel_test.go`
- `docs/plans/lost-turn-completion-cancel-recovery/plan.md`
- `docs/plans/lost-turn-completion-cancel-recovery/task-01-clear-stale-dispatch-gate.md`

## Dependencies

None.

## Parallelism

`sequential`. The files share one lifecycle state machine.

## Inputs

- The cancel recovery scenario in
  `docs/specs/agents/requirements/agent-stall-recovery.md`.
- The root-cause analysis in
  `docs/plans/lost-turn-completion-cancel-recovery/plan.md`.
- Existing cancel escalation tests in
  `manager_interaction_cancel_test.go`.
- Existing dispatch-only prompt tests in `session_test.go`.

## Implementation Notes

1. Write a regression test that uses the real `Manager.CancelAgent` path.
2. Make sure that the test fails because the follow-up prompt does not reach
   agentctl.
3. Change `dispatchedPromptPending` to `atomic.Bool`.
4. Replace direct reads and writes with `Load` and `Store`.
5. Route nil or stale closed prompt barriers through escalation when a dispatch
   gate is still pending.
6. Clear the old gate before `escalateStuckCancel` sends its synthetic signal.
7. Do not add an elapsed-time timeout to the gate.

## Output Contract

Report the changed files, exact test commands, test results, and remaining
risks. Update this task and `plan.md` in the same conversation.

## Results

- RED: `TestManager_CancelAgent_ClearsPendingDispatchGate` failed with
  `context deadline exceeded` because escalation left the dispatch gate set.
- GREEN: `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle
  -run 'TestManager_CancelAgent_(EscalatesWhenAgentHangs|ClearsPendingDispatchGate)'
  -count=1` passed with 2 tests.
- GREEN: `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run
  'TestSendPrompt_DispatchOnly' -count=1` passed with 2 tests.
- GREEN: `git diff --check` passed.
- Changed files: `types.go`, `session.go`, `manager_interaction.go`, and
  `manager_interaction_cancel_test.go`, plus this plan package.
- Review remediation: the regression now uses the real dispatch-only state with
  no fabricated prompt barrier, asserts the gate is cleared before the
  follow-up, and bounds the initial prompt observation.
- The dispatch gate is atomic. Cancellation handles nil and stale closed
  barriers, clears the gate before releasing the synthetic completion signal,
  and marks the execution ready. No known blockers remain.
