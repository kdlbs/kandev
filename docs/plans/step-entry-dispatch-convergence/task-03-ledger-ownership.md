---
id: "03-ledger-ownership"
title: "Converge dispatch onto the ledger dispatcher"
status: done
wave: 3
depends_on: ["01-ownership-declaration", "02-marker-positions"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-002
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-008
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.4
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.5
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.8
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.9
  - AC-OFFICE-STEP-ENTRY-DISPATCH-002.10
  - AC-OFFICE-STEP-ENTRY-DISPATCH-008.2
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 03: Converge dispatch onto the ledger dispatcher

## Summary

Move `clear_decisions` and `queue_run_for_each_participant` to the ledger
dispatcher, which claims through the existing marker methods before executing a
marker-bearing kind. The marker dispatcher stops executing them, and the ledger
dispatcher abandons the remainder of an entry's sequence when a marker-bearing
position fails or loses its claim.

## In scope

- The ledger path claiming through `ClaimStepEntryMarker` and completing
  through `CompleteStepEntryMarker` for marker-bearing kinds.
- The two-branch `idempotencyKey`, selecting its input from the ownership
  declaration's marker-bearing column rather than from whether an entry row
  exists.
- The abandon-remainder stop rule, scoped to marker-bearing positions so a
  failing `ensure_participant_seat` still lets the sequence continue.
- The production-shape regression asserting an empty queue after the expected
  run.

## Out of scope

- Ordering across owners. No wait is introduced between the synchronous ledger
  path and the marker path's goroutine.
- Extending markers to the three ledger-owned kinds that do not carry one.

## Acceptance

- One arrival at the shipped Review step enqueues exactly one fan-out.
- Failing `clear_decisions` at position 0 of the shipped Review sequence leaves
  the position-2 fan-out enqueuing nothing.
- Two entries into one step for one task produce runs with different
  idempotency keys, asserted on the keys rather than on call count.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/... ./internal/workflow/engine/... ./internal/task/repository/sqlite/...
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/step_entry_dispatch.go`
- `apps/backend/internal/workflow/engine/entrydispatch.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/step_entry_dispatch_test.go`
- `apps/backend/internal/orchestrator/review_participant_seats_acceptance_test.go`

## Dependencies

Tasks 01 and 02.

## Risks

- The stop rule is satisfied today on the path this task retires, so moving
  ownership without moving the rule silently drops it. A test asserting only
  that the failure was recorded passes against a dispatcher that continued.
- A deployment landing mid-round changes idempotency keys and can duplicate one
  round's runs once.

## Parallelism

`sequential`

## Inputs

- System design, "Decision: the ledger dispatcher is the single owner" and
  "Ordering within and across owners".
- `apps/backend/config/workflows/office-default.yml` Review and Approval steps.

## Results

Shipped in Build round 1 (`92755673f` + `02f5a1b1e`). `clear_decisions` and
`queue_run_for_each_participant` now execute exclusively via
`Engine.DispatchStepEntry`, claiming through `ExecuteMarkerBearingStepEntryAction`
before executing; the marker dispatcher (`dispatchOnEnterActions`) no longer
executes either kind. The two-branch `idempotencyKey` selects its input from the
ownership declaration's marker-bearing column. The AC-002.10 abandon-remainder
stop rule (`if abandon { break }` / `if execErr != nil { break }` in
`DispatchStepEntry`) is scoped to marker-bearing positions, so a failing
`ensure_participant_seat` still lets the sequence continue. Testing round 1 found
the production-shape empty-queue assertion (one arrival enqueues exactly one
fan-out) and the idempotency-key-inequality assertion (AC-008.2, keys not just
call count) were missing; Build round 2 (`1ee2eb149`) added both, using a
buffered channel to await the async marker-path dispatch for the empty-queue
case. Review round 1 found the stop rule vulnerable to the same position-index
divergence as Task 02; Build round 3 (`9c27351f2`) closed it via
`Action.DeclaredPosition`. Review rounds 1 and 2 both independently
re-confirmed (against merge-base `646ff0063`) that the unprotected
`markerEntryID==0` fallback on the 6 of 9 `dispatchStepEntry` call sites that do
not yet allocate an entry is pre-existing and unchanged by this diff — strictly
improved, not regressed — and is disclosed in `plan.md`'s Bounded-scope decision
as deferred to Task 04, not a defect of this task.
Verification: `go test ./internal/orchestrator/... ./internal/workflow/engine/...
./internal/task/repository/sqlite/...` passes, including `-race`.
