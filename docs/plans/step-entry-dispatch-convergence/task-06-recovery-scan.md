---
id: "06-recovery-scan"
title: "Startup step-entry recovery scan"
status: pending
wave: 6
depends_on: ["03-ledger-ownership", "05-marker-reclaim"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-004
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.1
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.2
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.3
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.4
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.5
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.6
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.11
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.12
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 06: Startup step-entry recovery scan

## Summary

Run one scan at backend start, after the repositories are constructed and
before the runs scheduler and engine dispatcher serve, that selects
non-terminal entries from the store alone, retires those the skip rules
exclude, and re-dispatches the rest through the owning dispatcher a live
arrival uses.

## In scope

- The scan's placement in `startSchedulingRuntime`, ahead of the runs
  scheduler start, and its ordering by `(task_id, step_id, entry_seq)`.
- Selection as a join over `workflow_step_entries` and its markers, loading no
  workflow declaration, so an entry with an unreadable declaration is still
  selected and reaches the `unresolvable` rule.
- The skip rules, including the 24-hour window bound and the reading of an
  `in_progress` marker as failed with no lease.
- The `columnExists` probe for `marker_positions` and `skip_reason`, emitting
  one ERROR and skipping the scan for that process start when it fails.
- Logging and skipping every failure inside the scan.

## Out of scope

- Repairing an unparseable `marker_positions` value. Such an entry is terminal
  by definition and one ERROR names it.
- Any lease or wall-clock threshold on `in_progress`.

## Acceptance

- The scan never blocks startup, including when a probe fails.
- An entry whose step declaration is removed or corrupted still receives a
  terminal marker for every position in its stored set.
- An entry allocated more than 24 hours earlier has its unfinished positions
  retired `idempotency_window_expired` with an ERROR and enqueues no run.

## Verification

```bash
cd apps/backend && go test ./internal/office/service/... ./internal/backendapp/... ./internal/task/repository/sqlite/...
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/office/service/step_entry_recovery.go`
- `apps/backend/internal/office/service/step_entry_recovery_test.go`
- `apps/backend/internal/task/repository/sqlite/step_entries.go`

## Dependencies

Tasks 03 and 05. The scan re-dispatches through the owning dispatcher and
resumes through the reclaim path.

## Risks

- Deriving terminality from the current declaration instead of the stored set
  re-selects every production entry at every process start, forever, because
  the shipped Review and Approval steps declare a non-marker-bearing position.
- A scan that quietly skips `unresolvable` entries looks identical to one with
  nothing to do, so the test must remove or corrupt a declaration.
- A probe failure degrades silently from the operator's view except for one
  ERROR line.

## Parallelism

`sequential`

## Inputs

- System design, "Startup recovery" and "Migrated columns are probed before
  use".
- `internal/office/service/scheduler_integration.go` for the existing sweep
  shape, noting that its recovery runs per tick rather than at start.

## Results

Pending.
