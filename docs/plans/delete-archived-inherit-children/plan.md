---
created: 2026-10-20
status: implemented
requirements:
  - REQ-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001
system_design:
  - ../../specs/tasks/system-design/delete-archived-inherit-children.md
legacy_specs: []
---

# Implementation plan: Non-cascade delete with archived inherited children

## Overview

Let a non-cascade delete succeed when an inherited child is archived, and turn a
real concurrent change into a typed `409` conflict. One work order covers the
service guard, the typed error, the handler mapping, the duplicated error text,
and the log fields.

## Confirmed cause

- The non-cascade child list includes archived children.
- The normalization guard set `RequireTaskNotArchived`, so the write matched no
  row for an archived inherited child.
- The lost guard surfaced as `errWorkspaceMetadataChangedConcurrently`, which the
  handlers mapped to `500 request failed`.
- The guarded write design already says this site must not set that clause.

## Scope

### In scope

- Drop `RequireTaskNotArchived` from the delete normalization guard.
- `TaskDeleteChildWorkspaceConflictError` with a reason, mapped to REST `409`
  and WebSocket `CONFLICT`.
- Single occurrence of the cause in the delete error text.
- `task_id` and `cascade` on the delete error logs.

### Out of scope

- Cascade delete, the delete preflight, and the marker stamping paths.

## Work orders

- [completed] [Task 01: Normalize archived inherited children](task-01-normalize-archived-inherit-children.md)

## Verification strategy

A service test on a real SQLite repository reproduces the `500` path with an
archived inherited child, with and without an orphan marker. A fake-repository
test injects a real guard loss and checks the typed error and the error text. A
handler test checks the `409` mapping. All fail on the base commit and pass
after.

## Verification results

Recorded in the task work order.

## Risks

- A child archived between the read and the write is now normalized instead of
  rejected. That leaves it in a valid state and matches the design's clearing
  rule.

## Independent upstream port

The 2026-10-23 port preserves the original source and history. Its real archived
child cases reproduced the old guard failure, then passed with the candidate
and selected existing controls under race detection. The work order separates
those results from historical integrated checks. No standing test is imported.
