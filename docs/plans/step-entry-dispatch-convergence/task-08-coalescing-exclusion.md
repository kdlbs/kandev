---
id: "08-coalescing-exclusion"
title: "Entry-triggered runs bypass coalescing"
status: pending
wave: 8
depends_on: ["03-ledger-ownership"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-005
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-005.1
  - AC-OFFICE-STEP-ENTRY-DISPATCH-005.2
  - AC-OFFICE-STEP-ENTRY-DISPATCH-005.3
  - AC-OFFICE-STEP-ENTRY-DISPATCH-005.4
  - AC-OFFICE-STEP-ENTRY-DISPATCH-005.5
  - AC-OFFICE-STEP-ENTRY-DISPATCH-005.6
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 08: Entry-triggered runs bypass coalescing

## Summary

Mark runs enqueued by a step-entry action so they neither merge into a queued
run nor are chosen as a merge target. The flag is carried both in memory, for
`shouldCoalesceRun`, and on the run row, because `CoalesceRun`'s subquery reads
persisted columns only.

## In scope

- `EntryTriggered bool` on `QueueRunRequest` in both `internal/runs/service`
  and `internal/workflow/engine`, which must stay identical.
- The engine setting it for every run enqueued by a step-entry action, and
  `shouldCoalesceRun` returning false when it is set.
- The `r.migrate.Apply("runs.entry_triggered", ...)` ALTER in the office
  repository, the insert path writing it, and `CoalesceRun`'s subquery gaining
  `AND entry_triggered = 0`.
- The `columnExists` probe, falling back to pre-requirement coalescing with one
  ERROR when the column is absent.

## Out of scope

- Any change to the idempotency key or to the task-comment prefix. Idempotency
  suppression is unaffected.

## Acceptance

- Two entry-triggered runs for one `(agent_profile_id, reason)` inside the
  window both remain runnable.
- A queued entry-triggered row is not selected by `CoalesceRun` as the target
  of a later ordinary request.
- With the column absent, run enqueue still succeeds and the exclusion is
  inactive.

## Verification

```bash
cd apps/backend && go test ./internal/runs/service/... ./internal/office/repository/sqlite/... ./internal/workflow/engine/...
```

## Files likely touched

- `apps/backend/internal/runs/service/service.go`
- `apps/backend/internal/runs/service/service_test.go`
- `apps/backend/internal/workflow/engine/types.go`
- `apps/backend/internal/office/repository/sqlite/base.go`
- `apps/backend/internal/office/repository/sqlite/runs_test.go`

## Dependencies

Task 03, which establishes which dispatcher enqueues an entry-triggered run.

## Risks

- Asserting only that an entry run does not coalesce passes against a
  service-side-only implementation, which is the half that is easy to ship
  alone. The target direction must be covered.
- SQL naming a missing column fails the whole statement, so an unprobed
  `entry_triggered` breaks every run enqueue rather than only the exclusion.

## Parallelism

`sequential`

## Inputs

- System design, "Coalescing exclusion" and "Migrated columns are probed before
  use".
- `runs.outcome` in `internal/office/repository/sqlite/base.go` as the
  precedent for both the ALTER site and the probe shape.

## Results

Pending.
