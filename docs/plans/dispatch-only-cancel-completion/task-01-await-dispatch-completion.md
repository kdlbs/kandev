---
id: "01-await-dispatch-completion"
title: "Await dispatch cancellation completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.7
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Await dispatch cancellation completion

## Summary

An acknowledged cancellation must await the current dispatch-only turn's completion.
A missing or closed `promptFinished` channel cannot establish a timeout.
Preserve bounded escalation for absent completion and exclusive ownership of the completion receive.

## In scope

- Add `TestManager_CancelAgent_DispatchCompletion` before production edits.
- Exercise real dispatch-only `Manager.PromptAgent` and `Manager.CancelAgent` calls with the existing WebSocket fixture.
- Cover nil and stale closed barriers with delayed and already-available matching completion.
- Deliver a matching provider completion from the `agent.prompt` handler before the prompt RPC acknowledgement and dispatch bookkeeping finish.
- Add bounded serialization with predecessor completion consumers under `promptMu`.
- Cover non-acknowledgement, stream disconnection, and missing completion when that consumer holds `promptMu`.
- Cover caller cancellation, timeout, completion at deadline, and generation replacement.
- Retain ordinary prompt, non-acknowledgement, disconnection, and escalation cleanup behavior.

## Out of scope

- Orchestrator grace periods, provider restart, new protocol fields, and frontend changes.
- Global cancellation redesign or changes to normal prompt admission policy.

## Acceptance

1. Matching completion within one cancellation budget returns success and permits a follow-up prompt for both barrier shapes.
2. Missing completion escalates after the budget. Caller cancellation releases the wait, and explicit transport sentinels retain current behavior.
3. Completion has one consumer. Ordinary prompts retain their signal, and stale work cannot clear or escalate a successor generation.
4. If a predecessor waiter retains `promptMu`, bounded generation-fenced escalation releases that same consumer, which clears the old gate before a follow-up prompt is admitted.

## Implementation notes

Use the [plan's lifecycle approach](plan.md#lifecycle-completion-wait).
Capture prompt ownership before the cancel RPC.
The dispatch helper uses deadline-bound `promptMu.TryLock` retries and rechecks identity after acquisition.
Do not block cancellation on an ordinary `SendPrompt` mutex owner.
If the predecessor waiter still owns the mutex at escalation, update readiness before sending a generation-bound wake signal; the existing waiter alone receives it and clears the old gate.
Route ordinary prompts through their existing completion barrier.
Do not restart the deadline after a barrier or stale signal.

Use channels to control the mock's cancellation acceptance and completion release.
Use a generous failure bound only to detect deadlock.
The missing-completion case must assert that escalation cannot occur before the cancellation budget.
The existing cleanup test alone cannot detect premature escalation.
Use non-parallel tests when changing package timeout variables.
Join all fixture goroutines before cleanup.

Run the nil-barrier delayed-completion regression first.
It must fail with premature `ErrCancelEscalated` before the implementation change.
Then add the remaining table cases and make the smallest correction.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestManager_CancelAgent_DispatchCompletion$' -count=1 -v)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestManager_CancelAgent|TestSendPrompt_DispatchOnly|TestWaitForPendingDispatchedPrompt|TestMarkReadyAsync' -count=1)
git diff --check
```

The first command supplies RED evidence before the correction and GREEN evidence afterward.
Record every changed test name and confirm that the command selects it.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_dispatch_cancel.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_dispatch_cancel_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/execution_store.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`

## Dependencies

None.

## Risks

- A predecessor waiter can consume completion before cancellation acquires `promptMu`.
- A successor can change generation before cleanup resumes.
- An error signal cannot automatically count as clean provider quiescence.
- Synchronous escalation publication can reintroduce the existing cancellation-guard deadlock.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/workflow-step-agent-start-ownership.md), requirement `002`.
- [Design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#dispatch-only-cancellation-completion).
- `manager_interaction_cancel_test.go`, especially `TestManager_CancelAgent_ClearsPendingDispatchGate`.
- `session.go`: `finishAcceptedPrompt`, `beginPromptBarrier`, and `waitForPendingDispatchedPrompt`.
- [Investigation evidence](plan.md#evidence-and-root-cause).

## Results

The new nil-barrier regression failed before the lifecycle change with immediate `ErrCancelEscalated` for both nil and stale closed barriers, including when completion was already available. It passes after the fix for delayed and available completion.

Additional coverage verifies cancellation timeout escalation, immediate escalation for a non-acknowledged cancel, caller cancellation, stale signal rejection, a competing predecessor waiter, prompt-generation replacement, completion available at the timeout boundary, transport failure escalation, and ordinary prompt completion ownership.

Review corrections:

- Added `TestManager_CancelAgent_DispatchCompletionBeforePromptAcknowledgement` for nil and stale closed barriers. The mock delivers `complete` from inside `triggerPrompt`, before `markPromptDispatched`; both cases failed before the correction with `ErrPromptActivityNotOwned` and pass with completed-generation ownership.
- Added `TestManager_CancelAgent_DispatchCompletionCompetingWaiterEscalates` for non-acknowledged cancellation, disconnected stream, and acknowledged cancellation timeout while the predecessor waiter holds `promptMu`. The cases failed before the correction with `ErrPromptActivityNotOwned`; they now return `ErrCancelEscalated`, release the existing consumer, clear only the old gate, admit a successor, and reject a late predecessor completion without changing successor state.
- Updated `waitForPendingDispatchedPrompt` to ignore signals for another generation, so a stale completion cannot release the successor gate while the consumer retries for its own signal. `TestWaitForPendingDispatchedPrompt_IgnoresStaleGenerationSignal` verifies it consumes only its own signal.
- Updated `TestSendPrompt_DispatchOnlyBlocksNextPromptUntilItsCompletion` and `TestSendPrompt_AdvancesGenerationForEveryDispatch` to attach the generation identity required by the lifecycle completion contract.

Verification passed:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestManager_CancelAgent_DispatchCompletion(BeforePromptAcknowledgement|CompetingWaiterEscalates)$' -count=1 -v)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestManager_CancelAgent|TestSendPrompt_DispatchOnly|TestWaitForPendingDispatchedPrompt|TestMarkReadyAsync|TestSendPrompt_AdvancesGenerationForEveryDispatch' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'TestResetAgentContext_|TestProcessOnEnter_.*Reset|TestHasActiveResetTurn_' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/integration -run 'TestWorkflowReset(ConfirmedDispatchCancelStartsStep|EscalationLeavesSessionDeletable)' -count=1)
make -C apps/backend build
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```
