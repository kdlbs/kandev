---
id: "04-skipped-marker-state"
title: "Terminal skipped marker state"
status: pending
wave: 4
depends_on: ["02-marker-positions"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-004
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.9
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 04: Terminal skipped marker state

## Summary

Add a fourth marker state, `skipped`, terminal and distinct from both `done`
and `failed`, with its reason stored in a new nullable column. Add
`SkipStepEntryMarker`, which records a skip from an absent, `in_progress`, or
`failed` row alike.

## In scope

- The `r.migrate.Apply("workflow_step_entry_markers.skip_reason", ...)` ALTER.
- `SkipStepEntryMarker(ctx, entryID, position, kind, reason, at)` as an upsert
  on `(entry_id, position)` setting `state='skipped'` and `skip_reason`, with
  the update branch guarded by `WHERE state <> 'done'`.
- The seven reason values named by AC-OFFICE-STEP-ENTRY-DISPATCH-004.9.

## Out of scope

- The startup scan that writes most skips. Task 06 owns it.
- The allocated position set. This work order claims
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.9 only for the terminal state and its
  closed reason list; Task 02 claims the stored set terminality derives from.
- The `adapter_unwired` skip site, which Task 09 owns.

## Acceptance

- A skipped position is readable as neither `done` nor `failed` after restart.
- `SkipStepEntryMarker` succeeds against an absent row, an `in_progress` row,
  and a `failed` row, and does not overwrite a `done` row.
- The reason values accepted are exactly the seven the criterion lists.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite/...
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/step_entries.go`
- `apps/backend/internal/task/repository/sqlite/step_entries_test.go`
- `apps/backend/internal/task/repository/sqlite/step_entries_postgres_test.go`

## Dependencies

Task 02, which establishes the migration site and the stored position set the
skip is written against.

## Risks

- `CREATE TABLE IF NOT EXISTS` does not add the column to an existing database,
  so the ALTER is required rather than optional.

## Parallelism

`sequential`

## Inputs

- System design, "Where terminality lives".
- The existing `CompleteStepEntryMarker` implementation.

## Results

Pending.
