---
id: "03-workflow-reset-feedback"
title: "Surface workflow reset failures"
status: done
wave: 3
depends_on:
  - "01-reset-cancellation-outcome"
  - "02-provider-reset-timeout"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.7
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.8
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.9
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 03: Surface workflow reset failures

## Summary

Preserve the reset error through workflow entry.
Persist it through the existing last-error metadata and event path without dispatching another workflow action.

## In scope

- Add an error-bearing reset helper and retain a boolean wrapper where it avoids unrelated caller changes.
- Report failed quiescence, timeout, provider reset, runtime restoration, and reset persistence.
- Use `persistLastAgentError` with sanitized reset-specific text and the captured execution identity.
- Persist the notice before publishing fresh waiting-state metadata.
- Use a bounded cleanup context after request expiry.
- Preserve the early return before automatic prompting and omit a successful reset divider.
- Prove session deletion through the real WebSocket handler after failure cleanup.

## Out of scope

New error UI, general agent-failure dispatch, automatic recovery, step re-entry semantics, and startup recovery.

## Acceptance

1. `TestProcessOnEnter_ResetFailurePersistsNotice` fails before the correction because reset failure produces no durable notice. It passes after persistence and publication.
2. Failed reset does not send a step prompt, show reset success, or evaluate `on_agent_error` or user cancellation completion. The error survives a fresh session read.
3. `TestWorkflowResetEscalationLeavesSessionDeletable` proves workflow entry, visible-error metadata, and successful `session.delete` within the client request timeout.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestProcessOnEnter_ResetFailurePersistsNotice$' -count=1 -v)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestProcessOnEnterResetAgentContext|TestProcessOnEnter_ResetFailure|TestResetAgentContext_' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/integration -run 'TestWorkflowResetEscalationLeavesSessionDeletable|TestOrchestratorCancelAfterAgentCrash_UnsticksSession|TestOrchestratorCancelWhenAgentHangs_UnsticksSession' -count=1)
git diff --check
```

Use table-driven failure cases and assert persisted metadata plus the published last-error event.
Cover request cancellation during error persistence, a stale pre-reset session snapshot, and persistence failure logging.
Use the simulator's existing hang-on-cancel behavior.
Extend it only with the reset-call counters and workflow setup needed by this regression.
Assert that both reset and restart remain absent after escalation.
Add a bounded concurrent deletion test to the timeout fixture to prove guard release after the provider response deadline.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_reset_failure_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_workflow_triggers_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go` (only if a reusable error-persistence seam is needed)
- `apps/backend/internal/integration/workflow_context_reset_failure_test.go` (new)
- `apps/backend/internal/integration/simulated_agent_manager_test.go`

## Dependencies

Tasks 01 and 02.

## Risks

The general failure handler can trigger workflow transitions and execution cleanup.
Call the persistence helper directly.
Never publish the old session metadata after the new notice.
A database outage can prevent durable feedback; preserve failure logs and release locks.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#persist-a-visible-failure).
- Existing `persistLastAgentError` and `publishSessionWaitingEvent`.
- `internal/integration/cancel_after_crash_test.go`.
- [UI-01](plan.md#ascii-ui-preview), verified by Task 04.

## Results

Implemented workflow reset failure containment. Workflow entry now persists a
sanitized last-agent-error notice for quiescence, provider reset, runtime
restoration, and persistence failures. It reloads session metadata before the
waiting-state event, skips automatic prompting and reset-success history, and
uses a bounded detached cleanup context after request cancellation.

Verification passed:

```text
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestProcessOnEnter_ResetFailure' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/integration -run '^TestWorkflowResetEscalationLeavesSessionDeletable$' -count=1 -v)
```

The integration test uses the real WebSocket session handler and confirms that
an escalated reset leaves the session deletable without reset, restart, or an
automatic workflow prompt. It also verifies the distinct prompt-call count,
user-message count, and sanitized persisted cancellation cause. The deterministic
unit barrier holds successor admission and deletion until failure persistence
and waiting-state publication finish, so neither can be overwritten by a stale
reset settlement.
