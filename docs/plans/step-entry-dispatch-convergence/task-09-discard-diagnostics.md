---
id: "09-discard-diagnostics"
title: "Discarded step-entry action diagnostics"
status: pending
wave: 9
depends_on: ["01-ownership-declaration", "04-skipped-marker-state"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-006
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-006.1
  - AC-OFFICE-STEP-ENTRY-DISPATCH-006.2
  - AC-OFFICE-STEP-ENTRY-DISPATCH-006.3
  - AC-OFFICE-STEP-ENTRY-DISPATCH-006.4
  - AC-OFFICE-STEP-ENTRY-DISPATCH-006.5
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 09: Discarded step-entry action diagnostics

## Summary

Warn at every point a declared entry action is discarded, naming workflow id,
step id, step name, and action type. Replace the `ErrActionNotYetWired`
callback with an unwired check ahead of the claim, which for a marker-bearing
position writes a terminal `skipped` marker with reason `adapter_unwired`.

## In scope

- The warning helper and its calls from all four discard points.
- The unwired check ahead of the claim, producing the warning and no `failed`
  marker.
- The terminal `skipped` marker at a marker-bearing position only, with
  non-marker-bearing positions keeping warning-and-no-marker behavior.
- The empty-entry-sequence case.

## Out of scope

- Warning on a kind skipped because another dispatcher owns it. That is not a
  discard.
- The fan-out's own per-participant records, which Task 10 owns.

## Acceptance

- A marker-bearing position with an unwired adapter reaches a terminal
  `skipped` marker with reason `adapter_unwired`, and a second process start
  does not re-select the entry.
- No `failed` marker is written for an unwired adapter.
- A kind owned by the other dispatcher produces no warning.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/... ./internal/workflow/engine/...
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_on_enter_warn_test.go`
- `apps/backend/internal/workflow/engine/engine.go`
- `apps/backend/internal/workflow/engine/types.go`

## Dependencies

Tasks 01 and 04.

## Risks

- Asserting only the warning passes against the no-marker implementation that
  strands the entry forever: a position with no row is not stranded, takes the
  ordinary insert on the next pass, and the unwired check fires again and again
  writes nothing.

## Parallelism

`sequential`

## Inputs

- System design, "Diagnostics".
- The four discard points named by AC-OFFICE-STEP-ENTRY-DISPATCH-006.2.

## Results

Pending.
