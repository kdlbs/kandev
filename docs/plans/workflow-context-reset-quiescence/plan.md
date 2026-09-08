---
created: 2026-08-30
status: implemented
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs:
  - ../../specs/workflow-on-enter-action-dispatch/spec.md
---

# Implementation Plan: Workflow Context Reset Quiescence

## Overview

This plan stops an active turn before workflow context replacement. Then it bounds any remaining wait for a dispatch-only predecessor.

The quiescence change comes first because it removes the known cause. The timeout follows as a recovery boundary for other lost completions.

## Scope

### In scope

- Use internal cancellation before a workflow reset replaces an active provider session.
- Preserve explicit user-cancellation and workflow-completion behavior.
- Prevent automatic step prompt dispatch when quiescence or reset fails.
- Bound the wait for an unresolved dispatch-only completion.
- Keep delayed completion events isolated by prompt generation.

### Out of scope

- Automatic repair of sessions that became stuck before this change.
- A REST cancellation endpoint or new UI recovery controls.
- Provider-specific ACP reset behavior.
- Runtime configuration restoration during reset.
- Changes to explicit user cancellation or `cancel_triggers_turn_complete`.

## Technical approach

### Workflow reset quiescence

Update `orchestrator.Service.resetAgentContext` in `apps/backend/internal/orchestrator/event_handlers_workflow.go`.

Keep the session lifecycle lock and the shared per-session `cancelInFlight` guard as the outer boundary. Publish the reset marker while that guard is held, and recheck the marker under the same guard in both normal and lifecycle prompt claims. If the session owns an active turn, register the internal cancellation coordinator exclusively before releasing the guard for its potentially blocking wait. Reacquire the guard before `AgentManager.ResetAgentContext` and persisted reset cleanup.

If no active turn exists but another cancellation owns the session, fail closed instead of allowing reset to continue with stale workflow inputs.

Use the existing bounded lifecycle cancellation and escalation path. Do not call `Service.CancelAgent`, because that path can evaluate user-configured workflow completion.

Return reset failure when internal cancellation fails. The existing `processOnEnter` flow then blocks automatic prompt dispatch.

### Bounded predecessor completion

Add a typed lifecycle error and a 10-second timeout in `apps/backend/internal/agent/runtime/lifecycle/session.go`.

Return the error from `waitForPendingDispatchedPrompt` without clearing `dispatchedPromptPending`. Deferred cleanup releases `promptMu` and the orchestrator dispatch guard.

Classify the typed error as transient in `apps/backend/internal/orchestrator/task_operations.go`. Existing queue logic then preserves automatic prompts that did not dispatch.

Keep `lifecycle.Manager.CancelAgent` as the only path that clears the stale pending gate through bounded escalation.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.1` | `TestResetAgentContext_QuiescesActiveTurnBeforeProviderReset`, `TestResetAgentContext_CancellationConflictStopsProviderReset`, and `TestHasActiveResetTurn_ReservedPromptOnly` in `event_handlers_workflow_reset_quiescence_test.go` |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.2` | The same test asserts internal cancellation and no explicit completion path. |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3` | `TestResetAgentContext_ActiveTurnAllowsSuccessorPrompt` and `TestResetAgentContext_SerializesPromptAdmission` in `event_handlers_workflow_reset_quiescence_test.go` |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4` | `TestResetAgentContext_CancelFailureStopsProviderReset` in `event_handlers_workflow_reset_quiescence_test.go` |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.5` | `TestWaitForPendingDispatchedPrompt_TimesOutWithoutClearingGate` in `session_pending_prompt_test.go` and transient classification coverage in `errors_test.go` |
| `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6` | `TestHandleAgentEvent_UnnumberedCompleteCannotReleasePendingPrompt` in `manager_events_test.go`, plus existing generation tests in `manager_events_test.go` and `execution_store_test.go`, remain green. |

## Work orders

- [x] [Task 01: Quiesce active reset turns](task-01-quiesce-active-reset-turns.md) (`done`)
- [x] [Task 02: Bound predecessor completion waits](task-02-bound-predecessor-completion-waits.md) (`done`)

## Verification results

Implemented and verified.

- Task 01 focused regression: 7 tests passed.
- Task 02 focused regressions: 4 tests passed across the lifecycle and orchestrator packages.
- Combined race-enabled affected-package suite: 4321 tests passed across the two affected packages.
- Backend lint reported 0 issues and specification lint passed.
- Red tests confirmed the pre-fix provider-reset ordering, cancellation fail-open, stale barrier hang, and missing transient classification.
- `make -C apps/backend test` passed the lifecycle and orchestrator packages but the overall target failed in unrelated config-discovery and launcher tests because this workspace's `/root/.kandev/config.yaml` took precedence over their temporary home directories.

## Risks

- Internal cancellation can race with natural completion. The existing cancellation identity and reconciliation rules must make both outcomes idempotent.
- Reset-related ready events can re-enter workflow handling. The reset marker must remain active until provider and persistence work finishes.
- The timeout must not clear a predecessor gate. Only cancellation can safely release that ownership.

## PR fixup results

- Reset quiescence now claims cancellation exclusively and fails closed on an existing cancellation operation.
- Reset marker publication and final normal/lifecycle prompt admission now share the per-session guard. Active reset cancellation releases that guard only while waiting, then reacquires it before provider replacement.
- Added reservation-only active-turn coverage and rejected unnumbered completion events while a numbered dispatch-only prompt is pending.
- Stabilized the fake-time timeout assertion and corrected the lifecycle test file manifest.
