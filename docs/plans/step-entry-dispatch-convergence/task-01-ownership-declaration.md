---
id: "01-ownership-declaration"
title: "Ownership declaration for step-entry action kinds"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-002
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.1
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.2
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.3
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.6
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.7
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 01: Ownership declaration for step-entry action kinds

## Summary

Add one exported table in `internal/workflow/stepentry` mapping every action
kind either dispatcher can reach to its owning dispatcher and to whether it is
marker-bearing. Both dispatchers read the table instead of keeping a private
kind list.

## In scope

- The ten-row table, with the two properties as independent columns.
- Seeding ownership from `sessionIndependentActionKinds` and marker-bearing
  from `IsEngineOwnedOnEnter`, and writing `configure_session` in by hand.
- Replacing the private kind lists in `Engine.DispatchStepEntry` and
  `dispatchOnEnterActions` with reads of the table, each walking its declared
  sequence in position order and silently skipping kinds it does not own.

## Out of scope

- Moving any kind's owner. This work order preserves current dispatch behavior
  for every kind; Task 03 moves ownership.
- Editing `sessionShapedActionKinds`. `entrydispatch_test.go` asserts both
  engine maps list only kinds `CompileOnEnterAction` emits, and the ownership
  table is deliberately wider than either map.

## Acceptance

- Every action kind either dispatcher can reach appears in the table with both
  properties set, and the membership matches the design's table exactly.
- Neither dispatcher retains a private kind list, and relative order among the
  kinds one dispatcher owns is preserved.
- A kind whose owner is the other dispatcher is skipped without a warning.

## Verification

```bash
cd apps/backend && go test ./internal/workflow/stepentry/... ./internal/workflow/engine/... ./internal/orchestrator/...
```

## Files likely touched

- `apps/backend/internal/workflow/stepentry/stepentry.go`
- `apps/backend/internal/workflow/stepentry/stepentry_test.go`
- `apps/backend/internal/workflow/engine/entrydispatch.go`
- `apps/backend/internal/workflow/engine/entrydispatch_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`

## Dependencies

None.

## Risks

- Seeding either column from the other's function inverts the table. Seeding
  ownership from `IsEngineOwnedOnEnter` silently drops `queue_run`,
  `run_code_review` and `ensure_participant_seat`; seeding marker-bearing from
  `sessionIndependentActionKinds` promises a marker for three kinds that have
  never carried one.

## Parallelism

`sequential`

## Inputs

- System design, "Ownership declaration" and "Ordering within and across
  owners".
- `internal/workflow/engine/entrydispatch.go` and its existing test.

## Results

Shipped in Build round 1 (`92755673f` + `02f5a1b1e`). The ten-row `ownershipTable`
landed in `internal/workflow/stepentry/stepentry.go`, with `Owner`/`OwnedByLedger`/
`OwnedByMarker`/`MarkerBearing`/`KnownKinds` as its exported accessors.
`Engine.DispatchStepEntry` (`entrydispatch.go`) reads the table via
`isSessionIndependentActionKind`/`isSessionShapedActionKind`, both delegating to
`stepentry.OwnedByLedger`/`OwnedByMarker`. `dispatchOnEnterActions`
(`event_handlers_workflow.go`) initially kept a private hardcoded switch-case list of
ledger-owned kinds instead of reading the table — a violation of this task's own
acceptance criterion ("Neither dispatcher retains a private kind list") that
Review round 2 caught (4/4 independent legs agreed) and Build round 4 fixed by
branching on `stepentry.OwnedByLedger(string(action.Type))` in the default case,
with a new completeness test (`TestProcessOnEnter_EveryLedgerOwnedKind_DoesNotWarn`)
iterating `stepentry.KnownKinds(stepentry.DispatcherLedger)` so a future table
addition is covered without a hand-written case. `TestOwnershipTableMatchesDesign`
(`stepentry_test.go`) also gained a `len(ownershipTable) != len(cases)` check in Build
round 4, closing a drift-detection gap test-supervisor's mutation testing exposed
(a kind added to the production table without a matching test-literal update
previously passed silently). Verification: `go test ./internal/workflow/stepentry/...
./internal/workflow/engine/... ./internal/orchestrator/...` passes; both new-this-round
checks were independently mutation-tested (bogus table entry / reverted switch-case)
and confirmed to fail before the fix and pass after.
