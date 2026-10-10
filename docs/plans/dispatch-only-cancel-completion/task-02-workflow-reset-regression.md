---
id: "02-workflow-reset-regression"
title: "Verify workflow reset after dispatch cancellation"
status: done
wave: 2
depends_on:
  - "01-await-dispatch-completion"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.7
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 02: Verify workflow reset after dispatch cancellation

## Summary

Prove that successful cancellation permits context reset and exactly one destination prompt.
Retain the existing failure path for genuine escalation.
Use backend orchestration and integration fixtures because this package changes no rendered UI.

## In scope

- Add a channel-controlled delayed-success case to the existing active-reset fixture.
- Assert that provider reset and automatic prompt dispatch wait for cancellation completion.
- Add `TestWorkflowResetConfirmedDispatchCancelStartsStep` beside the existing escalation integration test.
- Exercise an active session entering a step with `reset_agent_context` and `auto_start_agent`.
- Assert session identity, reset count, distinct prompt content, user-message count, and absence of reset-error metadata.
- Retain silent internal cancellation and stale-generation rejection.

## Out of scope

- Relaxing `quiesceActiveResetTurn` after genuine escalation.
- New browser UI tests, provider-specific end-to-end claims, and changes to workflow semantics.

## Acceptance

1. Before confirmed cancellation completes, neither provider reset nor the destination prompt starts. After completion, reset precedes exactly one destination prompt.
2. Internal cancellation adds no user-cancellation message and evaluates no user-cancellation completion action. Stale completion cannot settle the successor.
3. Genuine escalation still blocks provider reset and prompt delivery, persists the existing failure, and permits session deletion after cleanup.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'TestResetAgentContext_|TestProcessOnEnter_.*Reset|TestHasActiveResetTurn_' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/integration -run 'TestWorkflowReset(ConfirmedDispatchCancelStartsStep|EscalationLeavesSessionDeletable)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new consumer test can pass before Task 01 because the orchestrator already honors successful cancellation.
Task 01 supplies the required failing regression at the defective lifecycle boundary.
Do not claim that a simulator exercises the real lifecycle cancellation wait.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow_reset_quiescence_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_reset_failure_test.go`
- `apps/backend/internal/integration/workflow_context_reset_failure_test.go`
- `docs/plans/dispatch-only-cancel-completion/plan.md`
- `docs/plans/dispatch-only-cancel-completion/task-01-await-dispatch-completion.md`
- `docs/plans/dispatch-only-cancel-completion/task-02-workflow-reset-regression.md`

## Dependencies

Task 01.

## Risks

- A simulator-only success test can hide the lifecycle defect. Keep both layers of evidence.
- Duplicate automatic prompts can pass a state-only assertion. Assert content and counts.
- Delayed completion can trigger another workflow action unless prompt generation remains authoritative.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/workflow-step-agent-start-ownership.md), requirement `002`.
- [Design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#dispatch-only-cancellation-completion).
- `newActiveResetTestService` and `TestResetAgentContext_QuiescesActiveTurnBeforeProviderReset`.
- `TestProcessOnEnter_SuccessfulResetDispatchesAutoStartPrompt`.
- `TestWorkflowResetEscalationLeavesSessionDeletable`.
- Task 01 results.

## Results

The channel-controlled orchestrator regression proves provider reset and the destination prompt do not start until cancellation completes. After release, it observes reset before prompt, exactly one destination prompt and user message, and no user-cancellation message.

`TestWorkflowResetConfirmedDispatchCancelStartsStep` covers the successful workflow move through context reset and prompt dispatch. The simulator fixture supplies the executor inventory row and prompt-admission callback that the production lifecycle manager normally provides. The existing escalation scenario still blocks reset and prompt dispatch and permits session deletion.

Verification passed:

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'TestResetAgentContext_|TestProcessOnEnter_.*Reset|TestHasActiveResetTurn_' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/integration -run 'TestWorkflowReset(ConfirmedDispatchCancelStartsStep|EscalationLeavesSessionDeletable)' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/integration -run 'TestWorkflowReset(ConfirmedDispatchCancelStartsStep|EscalationLeavesSessionDeletable)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```
