---
id: "01-update-trigger-in-place"
title: "Update an existing routine trigger in place"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001
acceptance_criteria:
  - AC-OFFICE-ROUTINE-TRIGGER-UPDATE-001.1
system_design:
  - ../../specs/office/system-design/routine-trigger-update.md
---

# Task 01: Update an existing routine trigger in place

## Summary

Routine schedule edits were expressed as trigger create plus delete, so the
trigger identity and fire history could not be preserved and a claimed slot was
hard to change safely. This task adds `PATCH
/api/v1/office/routine-triggers/:triggerId` with a compare-and-set write:

- `cron_expression`, `timezone`, and `enabled` are the only editable fields.
- The trigger identity, routine, kind, webhook authentication, creation time,
  last fire, and past runs are preserved.
- The compare-and-set includes `next_run_at` (null or non-null): a scheduler
  claim or concurrent schedule edit that changes the stored row makes the
  comparison lose and returns 409. An enabled cron trigger observed with no next
  fire is refused before the write.
- Invalid cron expressions or timezones return 400; a missing trigger returns
  404.
- Disabling clears the next fire but does not cancel an already claimed fire.

## In scope

- `apps/backend/internal/office/repository/sqlite/routine_trigger_update.go`:
  writer-connection read and compare-and-set update.
- `apps/backend/internal/office/routines/trigger_update.go`: request application,
  validation, conflict detection, and next-fire preparation.
- `apps/backend/internal/office/routines/handler_trigger_update.go` and
  `handler.go`: PATCH route and status mapping.
- `apps/backend/internal/office/routines/dto.go`: `UpdateTriggerRequest`.
- `docs/public/office-routines.md`: operator-facing endpoint documentation.

## Out of scope

- Replacing or deleting triggers, or replaying runs.
- Scheduler claim, re-arm, or concurrency changes.
- Review-thread changes or any public PR update.

## Verification

- The non-documentation checks on this head pass, including the fork preview
  build and review checks. The PATCH route is included in the existing Office
  route scope inventory.
- This delivery package adds documentation only. Coverage is proven by the
  repository's `.github/scripts/pr-docs.cjs` validator reading this work order,
  its sibling plan, `REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001`, and the referenced design at
  the PR head.
- No new permanent test is added by this delta; a unit test for the new
  compare-and-set path is a follow-up outside this documentation-coverage unit.

## Changed files

- `apps/backend/internal/office/repository/sqlite/routine_trigger_update.go`
- `apps/backend/internal/office/routines/dto.go`
- `apps/backend/internal/office/routines/handler.go`
- `apps/backend/internal/office/routines/handler_trigger_update.go`
- `apps/backend/internal/office/routines/trigger_update.go`
- `docs/public/office-routines.md`
