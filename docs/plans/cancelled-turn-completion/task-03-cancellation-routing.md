---
id: "03-cancellation-routing"
title: "Explicit cancellation workflow routing"
status: done
wave: 2
depends_on: ["01-step-contract-and-template"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-cancelled-turn-completion.md"
---

# Task 03: Explicit cancellation workflow routing

## Acceptance

- Only `Service.CancelAgent` evaluates configured cancellation completion, exactly once after turn/session reconciliation; disabled steps and every silent/system cancellation path retain current behavior.
- Cancellation reconciliation is authoritative: session state must be confirmed as `WAITING_FOR_INPUT` and no active turn may remain before completion evaluation. A session-state or turn-close failure fails closed and leaves the workflow step unchanged.
- Configured user cancellation bypasses the agent completion-signal gate but not the pending-clarification, archive, ownership, Office, ephemeral, or stale-event guards.
- A successful transition preserves ordinary `on_exit`, terminal, destination `on_enter`, and auto-start behavior; failed/no-transition paths remain input-ready and queued messages stay parked.
- Terminal transitions publish their terminal task state directly without a transient `REVIEW` write; no-transition and successful nonterminal outcomes reconcile the task to `REVIEW` after settlement.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestCancelAgent_(TriggersConfiguredTurnComplete|KeepsDisabledStep|BypassesAgentSignal|BlocksPendingClarification|SkipsIneligibleTask|HandlesReconciledRuntime|LeavesQueuedMessageParked|CannotDoubleTransition)|TestUserCancelCompletion_' -count=1)
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/event_handlers_step_completion_test.go`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 02 after Task 01: this task owns orchestrator behavior and tests only.

## Inputs

- Spec `State Machine`, `Failure Modes`, and cancellation scenarios.
- ADR `2026-08-02-explicit-user-cancel-completion`.
- `Service.CancelAgent`, `handleAgentReady`, `processOnTurnCompleteViaEngine`, `turnCompleteBlockedByUserInput`, and existing cancel/queue race tests.

## Risks

- Evaluate only after the cancel guard owns and settles the turn; a late ready event must see the non-running session and return without a second transition.
- Do not route `cancelAgentSilent`, `AgentStopped`, parent stop, archive, or failure cleanup through the helper.
- Preserve the cancellation message's original turn association and do not drain queued work as a side effect.

## Output Contract

Report the typed completion-cause boundary, transition ordering, files changed, exact tests/results, blockers, and residual race risks. Update this task and `plan.md` status in the same conversation.

## Results

Added a typed `turnCompletionCause` boundary and routed only an initially working, explicit `Service.CancelAgent` through the existing completion engine after session/message/turn settlement. User cancellation honors the per-step flag, bypasses only the agent signal gate, retains the clarification barrier, skips Office/ephemeral/archived/terminal tasks, and keeps the cancel guard held until evaluation completes. Silent cancellation remains outside the helper; stale ready events observe the settled session and cannot double-transition.

Verification:

- Initial RED run: `rtk go test -tags fts5 ./internal/orchestrator -run 'TestCancelAgent_(TriggersConfiguredTurnComplete|KeepsDisabledStep|BypassesAgentSignal|BlocksPendingClarification|SkipsIneligibleTask|HandlesReconciledRuntime)' -count=1` — 6 passing existing/negative cases and 6 expected failures before routing.
- `rtk go test -tags fts5 ./internal/orchestrator -run 'TestCancelAgent_(TriggersConfiguredTurnComplete|KeepsDisabledStep|BypassesAgentSignal|BlocksPendingClarification|SkipsIneligibleTask|HandlesReconciledRuntime)' -count=1` — 12 tests passed after implementation.
- `rtk go test -tags fts5 ./internal/orchestrator -run 'TestCancelAgent_CannotDoubleTransitionFromStaleReady' -count=1` — 1 test passed.
- `rtk go test -tags fts5 ./internal/orchestrator -run 'TestUserCancelCompletion_SilentCancelDoesNotTrigger' -count=1` — 1 test passed.
- `rtk go test -tags fts5 ./internal/orchestrator -run 'TestCancelAgent_(TriggersConfiguredTurnComplete|KeepsDisabledStep|BypassesAgentSignal|BlocksPendingClarification|SkipsIneligibleTask|HandlesReconciledRuntime|LeavesQueuedMessageParked|CannotDoubleTransition)|TestUserCancelCompletion_' -count=1` — 18 tests passed.
- `rtk go test -tags fts5 ./internal/orchestrator -run 'Test(CancelAgent|ReconcileCancelledTurn|UserCancelCompletion)' -count=1` — 30 tests passed, including injected session-state and turn-close failures and terminal-state ordering.
