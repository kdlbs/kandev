---
id: "05-marker-reclaim"
title: "Marker reclaim path"
status: pending
wave: 5
depends_on: ["04-skipped-marker-state"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-004
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-008
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.7
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.8
  - AC-OFFICE-STEP-ENTRY-DISPATCH-008.4
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 05: Marker reclaim path

## Summary

Add `ReclaimStepEntryMarker`, a state-predicated compare-and-set that takes over
a marker left `in_progress` or `failed`, reporting from the affected row count.
The live-dispatch claim is left unchanged, so the two paths stay distinct.

## In scope

- `ReclaimStepEntryMarker(ctx, entryID, position, operationID, claimedAt)
  (bool, error)`, updating only `state IN ('in_progress','failed')`.
- Tests distinguishing the two paths on the same row: `ClaimStepEntryMarker`
  returns `claimed=false` against both an `in_progress` and a `failed` marker,
  while `ReclaimStepEntryMarker` returns `reclaimed=true` for both and `false`
  for `done` and `skipped`.

## Out of scope

- Any change to `ClaimStepEntryMarker`. Losing to an existing row must still
  mean "do not execute".
- The scan that calls reclaim. Task 06 owns it.

## Acceptance

- A `done` or `skipped` row is never taken over, so a completed position is
  never executed twice.
- A position with no row at all is not treated as stranded and takes the
  ordinary `ClaimStepEntryMarker` insert.
- The claim-loss test covers both `failed` and `in_progress`, asserting zero
  runs enqueued for each.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite/... ./internal/orchestrator/...
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/step_entries.go`
- `apps/backend/internal/task/repository/sqlite/step_entries_test.go`
- `apps/backend/internal/orchestrator/step_entry_dispatch_cas_loser_test.go`

## Dependencies

Task 04, whose `skipped` state reclaim must refuse.

## Risks

- A scan built on `ClaimStepEntryMarker` and `CompleteStepEntryMarker` alone
  would abandon every entry it was written to rescue while still satisfying
  AC-OFFICE-STEP-ENTRY-DISPATCH-008.4. A test that only exercises the scan
  end to end cannot tell a working reclaim from a scan that found nothing.
- Reclaim is safe without a lease only because the scan runs before the engine
  serves triggers. That ordering is load-bearing and is asserted in Task 06.

## Parallelism

`sequential`

## Inputs

- System design, "Reclaim: how a stranded position is re-executed".

## Results

Pending.
