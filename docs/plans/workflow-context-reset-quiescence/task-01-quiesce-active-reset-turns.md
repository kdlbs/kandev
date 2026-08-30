---
id: "01-quiesce-active-reset-turns"
title: "Quiesce active reset turns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Quiesce Active Reset Turns

## Summary

Stop and reconcile an active turn before workflow context replacement. Keep this operation separate from explicit user cancellation.

## In scope

- Detect an active turn after the reset marker becomes active.
- Use internal cancellation before provider context replacement.
- Keep the reset marker active through cancellation and reset settlement.
- Stop automatic prompt dispatch when cancellation fails.
- Cover natural completion races and provider reset ordering.

## Out of scope

- Generic stale-prompt timeout behavior.
- Explicit user cancellation semantics.
- Automatic repair of an existing stuck session.

## Acceptance

- Provider reset starts only after the active turn is reconciled.
- Internal cancellation does not evaluate user-configured workflow completion.
- A cancellation error prevents provider reset and automatic prompt dispatch.

## Verification

```bash
go test ./internal/orchestrator -run 'TestResetAgentContext_(QuiescesActiveTurnBeforeProviderReset|ActiveTurnAllowsSuccessorPrompt|CancelFailureStopsProviderReset)' -count=1
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_reset_quiescence_test.go`

## Dependencies

None.

## Risks

- A natural completion can race with internal cancellation.
- A ready event can arrive while the reset marker is active.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002`
- `docs/specs/tasks/system-design/workflow-step-agent-start-ownership.md`
- `docs/decisions/2026-08-30-context-reset-quiesces-active-turn.md`
- Existing silent cancellation in `event_handlers_clarification.go`

## Results

Implemented.

- Added active-turn detection using durable and in-memory turn ownership records.
- Routed reset quiescence through the internal silent cancellation coordinator before provider reset.
- Preserved the reset marker through cancellation and provider replacement, and failed closed when cancellation failed.
- Added ordering, successor-prompt, and cancellation-failure regressions in `event_handlers_workflow_reset_quiescence_test.go`.
- Red: the pre-fix tests observed provider reset without cancellation and allowed reset after cancellation failure.
- Green: the exact verification command passed 3 tests.
