---
id: "01-reset-cancellation-outcome"
title: "Preserve reset cancellation outcome"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.7
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Preserve reset cancellation outcome

## Summary

Workflow reset must distinguish provider stop confirmation from local cancellation reconciliation.
Preserve the existing semantics for other cancellation callers.

## In scope

- Capture the raw lifecycle cancellation result on its owning coordinator operation.
- Expose a synchronized provider outcome to the exclusive reset path after local reconciliation completes.
- Reject escalation in `quiesceActiveResetTurn` without calling provider reset or restart.
- Preserve missing-execution cleanup and explicit, joined, peer, clarification, and queue behavior.
- Keep cancellation projection and turn identity cleanup correct on every exit.

## Out of scope

Provider request deadlines, visible failure reporting, startup recovery, and process replacement.

## Acceptance

1. The named regression fails before the correction: reset currently returns success after `ErrCancelEscalated`. After correction, it returns failure with no reset call.
2. Local reconciliation still settles the captured turn. Explicit and joined callers retain success, visible cancellation, and configured completion semantics.
3. Reset cleanup releases the marker and guard. A delayed old completion cannot finish a successor generation.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestResetAgentContext_EscalatedCancelStopsProviderReset$' -count=1 -v)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestResetAgentContext_|TestHasActiveResetTurn_|TestCancelAgent_|TestCancelAgentSilent_|TestCancellationSourcesShareLifecycleOwnership|TestUserCancelCompletion_|TestQueueAndInterruptForPeerMessage_' -count=1)
git diff --check
```

The first invocation supplies the required RED result before production edits.
Use the existing `newActiveResetTestService` fixture and `orderedResetAgentManager`.
Include direct and wrapped escalation, ordinary failure, confirmed stop, missing execution, and a joined explicit caller.
Use deterministic channels for concurrent completion tests.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_reset_quiescence_test.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`

## Dependencies

None.

## Risks

Do not set the common `cancelOperation.err` to escalation after successful reconciliation.
That error controls joined callers and registered actions.
Do not make every `cancellationKindInternal` caller require strict reset semantics.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#preserve-the-provider-cancellation-outcome).
- [Plan evidence](plan.md#evidence-and-confidence).
- Existing cancellation identity, projection, and exclusive-claim tests.

## Results

Implemented the cancellation outcome boundary. The cancellation coordinator now
retains the raw provider cancellation result, while explicit and joined
cancellation callers keep their existing reconciliation result. Workflow reset
reads that outcome after the owned cancellation completes and fails closed on
direct or wrapped `lifecycle.ErrCancelEscalated` without calling provider reset.
Missing execution and ordinary cancellation failure behavior remain covered.

The workflow regression now configures reset and automatic step start with a
distinct prompt. Escalation records no prompt call or user message, while the
successful-reset control records one dispatch of that prompt.

Verification passed:

```text
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestResetAgentContext_EscalatedCancelStopsProviderReset$' -count=1 -v)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestResetAgentContext_|TestHasActiveResetTurn_|TestCancelAgent_|TestCancelAgentSilent_|TestCancellationSourcesShareLifecycleOwnership|TestUserCancelCompletion_|TestQueueAndInterruptForPeerMessage_' -count=1)
git diff --check
```
