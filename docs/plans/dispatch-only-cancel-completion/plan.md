---
created: 2026-09-30
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Dispatch-only cancellation completion

## Overview

[Issue #4085](https://github.com/kdlbs/kandev/issues/4085) reports immediate cancellation escalation during workflow context reset.
The issue is assigned to `carlosflorencio`, the authenticated GitHub user.
Repair lifecycle completion waiting first, then prove workflow entry and failure containment.
Tasks 01 and 02 are complete.

## Evidence and root cause

Investigation used commit `9877965bb92952de9d480f06e7e81287f5ee4444`.
The canonical issue contains no comments or image attachments.
The issue's logs report provider completion 76–105 milliseconds after false escalation.
Those timing observations are reporter evidence, not a local provider replay.

The current source differs slightly from the reported revision:

- `SessionManager.sendPrompt` skips `beginPromptBarrier` for dispatch-only calls.
- `finishAcceptedPrompt` sets `dispatchedPromptPending` and returns before completion.
- `cancelAgentExecution` escalates immediately for a nil barrier with a pending dispatch.
- Its closed-barrier branch also escalates immediately for a pending dispatch.
- `quiesceActiveResetTurn` correctly rejects `ErrCancelEscalated` before provider reset.

A temporary `TestIssue4085_DispatchCancelWaitsForCompletion` exercised real `Manager.PromptAgent` and `Manager.CancelAgent` calls against the existing WebSocket mock.
The provider accepted cancellation. The test scheduled completion after 100 milliseconds and supplied a two-second cancellation budget.
Cancellation returned `ErrCancelEscalated` after 107.911 microseconds.
The expected-success assertion failed, which confirms the nil-barrier defect.
The stale closed-barrier variant is source-confirmed and requires permanent coverage.

The existing `EscalatesWhenAgentHangs` and `ClearsPendingDispatchGate` cancellation tests passed.
They cover missing completion but do not reject premature escalation.
The temporary reproduction is removed before handoff.
No real Claude ACP session or complete fan-out workflow ran during investigation.

## Requirement conformance

Reuse `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002` and criteria `.1` through `.7`.
Confirmed completion permits reset and successor delivery under `.1` and `.3`.
Unconfirmed cancellation still blocks reset under `.4` and `.7`.
Criterion `.6` preserves generation ownership.
No new requirement or ADR is necessary for this implementation defect.
The existing [quiescence ADR](../../decisions/2026-08-30-context-reset-quiesces-active-turn.md)
and [generation ADR](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md) remain authoritative.

The task system owns this package because workflow reset controls successor admission.
The existing [system design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#dispatch-only-cancellation-completion)
now specifies the missing cancellation wait.

## Scope

### In scope

- Wait for real completion after an acknowledged dispatch-only cancellation.
- Cover nil and stale closed `promptFinished` channels.
- Preserve one cancellation budget, caller cancellation, and genuine escalation cleanup.
- Prevent competing readers from losing a completion or clearing a successor gate.
- Prove successful workflow reset and retain genuine reset failure containment.

### Out of scope

- An orchestrator grace period after genuine `ErrCancelEscalated`.
- Provider restart, retry, or automatic step re-entry after failed reset.
- New cancellation settings, feature flags, database changes, or protocol fields.
- Frontend markup, copy, layout, and mobile interaction changes.
- Historical session repair and changes to fan-out workflow semantics.

## Technical approach

### Lifecycle completion wait

Change `cancelAgentExecution` in `manager_interaction.go` and extract a focused helper if required by lint limits.
Capture execution and prompt ownership before cancellation.
Retain immediate escalation for explicit unacknowledged cancellation or a disconnected active stream.
An idle disconnected execution still succeeds.

After successful cancellation, use one deadline for the ordinary barrier and dispatch-only completion paths.
A nil or closed ordinary barrier must not classify a pending dispatch as timed out.
Keep the ordinary prompt's completion consumer unchanged.

Serialize dispatch completion consumption with `promptMu`.
Use deadline-bound `TryLock` retries with a small internal interval, such as five milliseconds.
Recheck pending state and captured generation after acquisition.
Hold the mutex only through the bounded completion receive and gate update.
All exits release it. Do not launch a goroutine that can outlive the request while waiting for the mutex.
A competing predecessor waiter can consume completion first. Recheck its result instead of requiring a second signal.
A completion accepted for the captured generation remains authoritative even if it arrives before `markPromptDispatched` records that generation.
If a predecessor waiter holds `promptMu` through cancellation escalation, fence the captured generation under `promptLifecycleMu`, mark it ready, and send a generation-bound synthetic signal to the existing waiter. That waiter remains the only receiver and gate clearer; cancellation must not mutate execution state after signaling it.
A successor generation must cause a non-success ownership result without mutation.

Reject stale completion identities and retain the original deadline.
Propagate transport failure rather than reporting clean provider quiescence.
Prefer an available matching completion at the timeout boundary.
If completion remains absent, use the existing generation-owned escalation and cleanup.
Preserve asynchronous `AgentReady` publication and stale-signal draining.

### Compatibility matrix

| Path | Transport and identity | Intended result | Evidence and fallback |
| --- | --- | --- | --- |
| Claude ACP dispatch-only | agentctl WebSocket, execution and prompt generation | Await real completion | Real lifecycle with protocol mock; no live Claude claim |
| Other structured providers | Same normalized agentctl contract | Same bounded wait when the contract is supported | Shared-path tests; transport errors retain existing handling |
| Ordinary blocking prompt | `SendPrompt` owns the completion receive | Await its barrier | Existing cancellation tests plus no-stolen-signal regression |
| Missing completion | Acknowledged cancel, current generation remains pending | Escalate only after the budget | Existing cleanup and successor-dispatch tests |
| Explicit non-acknowledgement or disconnect | Existing client sentinels | Retain immediate escalation for active work | Existing and extended sentinel tests |
| Passthrough | Separate PTY cancellation path | Unchanged | No new provider-support claim |

### Workflow verification

Keep production `quiesceActiveResetTurn` unchanged unless targeted evidence identifies a separate defect.
Extend existing reset tests to delay successful cancellation completion and assert reset-before-prompt ordering.
Backend integration follows workflow entry through reset and exactly one distinct destination prompt.
Pair the positive scenario with the existing escalation failure scenario.
The lifecycle tests prove real wait behavior. The orchestrator simulator proves consumer behavior, not provider timing.

## Tests

All criterion suffixes below belong to `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002`.

| Criteria | Test file and evidence |
| --- | --- |
| `.1`, `.3` | `manager_interaction_dispatch_cancel_test.go`: new `TestManager_CancelAgent_DispatchCompletion` table, nil and stale closed barriers |
| `.1`, `.5`, `.7` | Same file: absent completion, caller cancellation, transport failure, and immediate explicit non-acknowledgement |
| `.5`, `.6` | Same file: competing predecessor waiter, stale signal, successor generation, completion at deadline |
| `.1`, `.3`, `.6` | Existing `TestManager_CancelAgent_ClearsPendingDispatchGate`, `TestSendPrompt_DispatchOnly`, and ordinary cancellation tests |
| `.1`, `.2`, `.3` | `event_handlers_workflow_reset_quiescence_test.go`: delayed cancellation success and provider-reset ordering |
| `.4`, `.7` | Existing escalated-cancel reset tests and `TestProcessOnEnter_ResetFailurePersistsNotice` |

## E2E tests

Backend end-to-end evidence belongs in `internal/integration/workflow_context_reset_failure_test.go`.
Add `TestWorkflowResetConfirmedDispatchCancelStartsStep` beside `TestWorkflowResetEscalationLeavesSessionDeletable`.
Assert one context reset, one destination prompt, preserved session identity, and no reset-error notice after successful cancellation.
Assert that genuine escalation still prevents reset and prompt delivery and leaves deletion available.
These scenarios cover `.1`, `.2`, `.3`, `.4`, and `.7`.
No new Playwright scenario is required because the package changes no rendered UI or interaction contract.

## Companion packages

The completed packages remain historical delivery records:

- [Lost turn-completion recovery](../lost-turn-completion-cancel-recovery/plan.md): preserve gate cleanup and next-prompt delivery.
- [Context-reset quiescence](../workflow-context-reset-quiescence/plan.md): preserve internal cancellation and bounded predecessor waits.
- [Reset failure containment](../workflow-reset-failure-containment/plan.md): preserve rejection of genuine escalation.
- [Idle reset dispatch gate](../idle-context-reset-dispatch-gate/plan.md): preserve idle reset behavior.
- [Runtime configuration restoration](../acp-context-reset-runtime-configuration/plan.md): provider configuration remains unchanged.

This package supersedes only the historical immediate-escalation choice for nil or closed barriers after successful cancellation.
It does not reopen completed work orders or replace their recorded results.

## Work orders

Execute sequentially after a later implementation request.

- [x] [Task 01: Await dispatch cancellation completion](task-01-await-dispatch-completion.md)
- [x] [Task 02: Verify workflow reset after dispatch cancellation](task-02-workflow-reset-regression.md)

## Verification results

Investigation:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestIssue4085_DispatchCancelWaitsForCompletion$' -count=1 -v)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestManager_CancelAgent_(ClearsPendingDispatchGate|EscalatesWhenAgentHangs)$' -count=1)
```

The reproduction failed at the expected assertion. Both control tests passed.
Package validation passed:

- `python3 scripts/list-docs.py validate`: 337 decisions and 1,271 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- PR documentation `validateCoverage`: actual documentation changes are exempt.
- The same preflight with the planned lifecycle source path was covered, with both work orders and requirement/design references accepted.
- `git diff --check`: passed.

The package reuses the existing requirement document without changing its criteria.

Implementation:

- Dispatch-only cancellation now waits within one deadline for the captured generation's accepted completion. It serializes completion consumption, preserves ordinary prompt ownership, rejects successor-generation mutation, and retains escalation for a missing completion.
- Added nil and stale-closed barrier coverage for available and delayed completion, plus timeout, caller cancellation, stale signal, competing consumer, generation replacement, deadline-boundary completion, transport failure, immediate non-acknowledgement, and ordinary-prompt cases.
- The workflow reset regression confirms cancellation completion precedes provider reset, which precedes exactly one destination prompt. The integration pair verifies successful reset and the existing genuine-escalation failure behavior.
- `make -C apps/backend build` passed.
- Lifecycle, orchestrator, and workflow integration tests passed, including race-enabled lifecycle, orchestrator, and integration runs.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.

The integration simulator was extended to model prompt admission and the executor inventory row expected from the production lifecycle manager. It does not model the lifecycle cancellation wait; the lifecycle regression exercises that contract directly.

## Risks

- Two consumers of `promptDoneCh` can steal completion. The wait must retain one owner.
- An unbounded mutex acquisition can recreate cancellation stalls.
- Cleanup after a successor starts can clear the wrong gate. Capture and recheck prompt identity.
- Global test timeout variables cannot change in parallel tests.
- A protocol mock does not establish provider-specific behavior for every adapter.
