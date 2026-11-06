---
status: current
system: tasks
requirements:
  - REQ-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001
---

# Non-cascade delete with archived inherited children system design

## Purpose and boundaries

The change is confined to the non-cascade branch of
`HandoffService.deleteTaskTreeWithReasonAndOptions` and to the delete error
mapping in the task handlers. The guarded write itself
(`SetTaskWorkspaceMetadataIfUnchanged`) is unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001` | [Decision](#decision-normalize-archived-children), [Conflict reporting](#conflict-reporting), [Error text and logs](#error-text-and-logs) |

## Root cause

`reparentNoCascadeChildren` loads every direct child through
`ListStructuralChildrenLimited`, which includes archived rows.
`normalizeNoCascadeChildren` then set `RequireTaskNotArchived` on the guard,
which appends `AND archived_at IS NULL` to the write. For an archived inherited
child the write affected zero rows and was returned as
`errWorkspaceMetadataChangedConcurrently`, which the handlers did not map, so
REST returned `500 request failed`. The clause was added to this site by the
archive lifecycle hardening change; the original orphaned-workspace change
(#3475) did not set it here.

## Decision: normalize archived children

The [guarded write design](orphaned-workspace-guarded-metadata-write.md) lists
"Delete normalization, site 4" with `RequireTaskNotArchived` = **no**, and
explains why: the clause belongs to stamping paths, and clearing paths read
through archived children so they can retract what they must. Site 4 clears the
marker (AC-TASKS-ORPHANED-WORKSPACE-005.6). The code now matches that table.

Skipping archived children instead would be wrong in two ways:

- The child would be promoted to root while still `inherit_parent`, with no
  parent to inherit from once it is unarchived. That is the state the
  normalization exists to prevent.
- A marker on the archived child would keep naming the deleted parent. The
  repair's clearing pass joins on a present, unarchived parent, so nothing
  would ever retract it.

The other guard clauses stay: the CAS on mode and marker, and `RequireParentID`,
which still catches a concurrent reparent.

## Conflict reporting

When the guard loses, the service re-reads the child and returns
`*TaskDeleteChildWorkspaceConflictError{ChildID, Reason}`. A missing row is
`child_deleted`, a different parent is `child_reparented`, and anything else,
including a failed re-read, is `child_workspace_changed`. The error wraps
`errWorkspaceMetadataChangedConcurrently`, so existing checks still match.

`handleNotFound` maps it to `409` with the details from
`taskDeleteChildConflictDetails`; `wsDeleteTask` maps it to `CONFLICT` with the
same details. The abort happens before any child is promoted, and the existing
compensation restores any earlier child writes.

## Error text and logs

`rollbackWorkspaceEnvironmentOwnershipAfterFailure` returns its cause, joined
with any rollback failure. The two non-cascade failure branches joined that
result with the cause again, so the cause appeared twice. They now replace the
error with the helper's result. `handleNotFound` accepts extra log fields, and
the HTTP and WebSocket delete handlers pass `task_id` and `cascade`.

## Delete preflight

`TaskDeletePreflight` and the preview digest already include archived direct
children of a non-cascade delete. No change is needed: with this fix those
children no longer make the delete fail.

## Residual risks

- A child archived concurrently between the read and the write is now
  normalized rather than rejected. That matches the clearing-path rule and
  leaves the child in a valid state.
