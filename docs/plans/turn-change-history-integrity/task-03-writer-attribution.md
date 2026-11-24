---
id: "03-writer-attribution"
title: "Derive overlaps from canonical writer intervals"
status: done
wave: 3
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.9
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-003.5
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-003.7
  - AC-TASKS-TURN-CHANGES-003.8
  - AC-TASKS-TURN-CHANGES-003.9
  - AC-TASKS-TURN-CHANGES-004.1
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
---

# Task 03: Derive overlaps from canonical writer intervals

## Summary

Stop treating unfinished capture/export rows as active writers.
Repair stale overlap attribution from exact canonical turn boundaries.

## In scope

- Add stable paged interval reads joined to canonical turns for admitted checkouts.
- Compute half-open writer intersections, including aliasing worktree identities.
- Reconcile completed pending rows before overlap admission and during the task sweep.
- Use known terminal times rather than reconciliation time.
- Add a narrow runtime boundary-state callback so orphan settlement cannot race an
  active capture operation for another or matching generation.
- Resume only accepted immutable evidence; settle a lost boundary unavailable.
- Correct both finalized and pending overlap records and publish revised summaries.
- Preserve real overlaps, active siblings, turn state, completion gates, and shutdown semantics.

## Out of scope

New capture endpoints, history normalization, export-page implementation, and live-data edits.

## Acceptance

1. A completed pending capture produces no overlap with a later nonintersecting turn.
   Genuine intersections remain visible; touching interval endpoints do not overlap.
2. Mixed active/completed candidates, more than 500 candidates, and delayed terminal
   events reconcile without closing a live writer or changing another generation.
3. A lost completed boundary settles unavailable with its canonical time.
   Repair does not call executor capture, complete a turn, or advance a workflow.

## TDD and regression

Add proposed task/changes/coordinator_writer_intervals_test.go and
task/repository/sqlite/turn_change_writer_intervals_test.go.
RED: TestTurnChangeWriterIntervalsExcludeCompletedPending uses the incident shape:
canonical turn completed, capture pending, and a later turn on the same checkout.

Also require TestTurnChangeWriterIntervalsPreserveRealOverlap,
TestTurnChangeWriterIntervalsTouchingBoundaries,
TestTurnChangeOrphanReconcileMixedState,
TestTurnChangeOrphanReconcileDoesNotRecapture,
and TestTurnChangeOrphanReconcilePagedCandidates.
Include a completed row beside a live row and a boundary callback returning unknown.
Verify old finalized cards receive corrected revisions.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -tags fts5 ./internal/task/changes ./internal/task/repository/sqlite ./internal/task/service ./internal/backendapp -run 'TestTurnChangeWriterIntervals|TestTurnChangeOrphanReconcile|TestPostgresTurnChangeWriterIntervals' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -race -tags fts5 ./internal/task/changes ./internal/agent/runtime/lifecycle ./internal/backendapp -run 'TestTurnChange.*(Overlap|Writer|Orphan|Boundary)|TestStopPreservesActiveTurnBeforeExecutorCleanup|TestTurnChangeCapturePreservesShutdownDeliveryOutcome' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

Use the real configured PostgreSQL DSN for parity cases.
Run scripts/lint-go-changed for actual changed Go files.

## Files likely touched

Existing: apps/backend/internal/task/changes/coordinator.go;
apps/backend/internal/task/repository/turn_changes.go;
apps/backend/internal/task/repository/sqlite/turn_changes.go;
apps/backend/internal/task/service/archived_session_reconciliation.go;
apps/backend/internal/task/service/service_turn_changes.go;
apps/backend/internal/backendapp/orchestrator.go;
apps/backend/internal/backendapp/turn_changes_runtime.go;
apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go;
apps/backend/internal/agent/runtime/lifecycle/turn_changes.go.

Extract focused interval-reader/reconciler files beside these large sources.
Canonical turn timestamps remain owned by existing turn storage.

## Dependencies

None on provenance. Execute third for a sequential review.
Its boundary-state seam is required before work order 04 separates writer and export jobs.

## Risks

Canonical completion and delayed capture can race.
Use the exact operation/generation guard and conditional writes.
Do not replace a known terminal time with the sweep clock or infer liveness from session state.

## Parallelism

sequential

## Inputs

- History integrity design: Writer intervals; Orphan reconciliation; Runtime integration.
- Existing coordinator overlap/recovery tests and lifecycle cancellation/shutdown regressions.
- Original work order 05's shutdown and durable ownership follow-ups.

## Results

Replaced unfinished-capture liveness with canonical half-open writer intervals joined to turn completion and terminal claims. Admission and terminal settlement now reconcile overlaps for both pending and finalized change sets, remove empty intersections, preserve genuine overlaps, and bump revisions. Startup and periodic reconciliation require a completed canonical turn and a narrow runtime operation guard; unknown ownership fails closed. Tests cover completed pending captures, real overlaps, touching boundaries, mixed active/completed rows, and more than 100 bounded candidates.
