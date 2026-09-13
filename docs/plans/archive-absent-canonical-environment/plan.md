---
spec: docs/specs/tasks/requirements/detached-workspace-continuity.md
created: 2026-09-14
status: pending
---

# Implementation Plan: Archive with an absent canonical environment

## Overview

Archiving a task fails permanently when a workspace group it belongs to names a
`task_environments` row that no longer exists. Stewardship transfer resolves that
environment before deciding whether a transfer is needed, and treats every
resolution failure as fatal. Because the group reference is never repaired, the
failure is permanent rather than transient: the card can never be archived by any
route.

Measured on the local install on 2026-09-14: 50 of 68 `task_workspace_groups`
rows name an absent environment, and 33 unarchived tasks are archive-blocked.

The correction is narrow. A positively absent environment carries no ownership,
so there is nothing to transfer and the archive proceeds. Any other resolution
failure stays fatal, so ownership is never abandoned on a transient error. This
is the typed-sentinel rule ADR-0009 already requires and the "positively known
absent" rule the system design already states for cleanup fencing; the transfer
path simply does not implement either.

**Explicitly not in scope, and why.** Skipping a transfer is not evidence that a
group's physical resources are gone. A group with a non-empty `materialized_path`
may still own worktrees on disk. This plan unblocks archive and leaves the stale
group reference intact for later reconciliation; it does not claim the workspace
was cleaned. Task worktree cleanup already skips session-linked worktrees whose
environment lookup fails, which leaks rather than destroys, and that is the
ADR-0009-correct direction.

No schema, HTTP contract, or WebSocket protocol change is required.

---

## Backend

### Absent-environment tolerance in stewardship transfer

- `transferWorkspaceGroupEnvironmentOwnership`
  (`apps/backend/internal/task/service/handoff_cascade.go`) resolves
  `group.MaterializedEnvironmentID` and currently fails on both the `err != nil`
  branch and the separate `env == nil` branch.
- Treat a positively absent environment as "no ownership to move": return no
  transfer and no error, so the caller records nothing and the archive proceeds.
  Positive absence means the repository's typed not-found sentinel, or a nil row
  with a nil error. Every other error stays fatal.
- The sentinel is already produced by the production repository
  (`apps/backend/internal/task/repository/sqlite/task_environment.go`), so this
  needs `errors.Is` at the call site, not a repository contract change.

### Deliberately unchanged

- The guarded transfer method, generation fencing, and the group cleanup claim
  are untouched. A skipped transfer increments no generation, which is correct:
  no stewardship changed.
- `rollbackWorkspaceEnvironmentOwnershipAfterFailure` needs no change. The caller
  appends only non-nil transfers and rollback returns immediately for an empty
  list, so a transfer that never happened carries no rollback obligation.

---

## Frontend

None.

---

## Known residue after this plan

Archiving one of these tasks will still leave the group in `cleanup_status`
`active` or `cleanup_failed`, because `single_repo` group cleanup requires
`restore_config_json.worktree_ids` and the affected rows carry none. Archive logs
that cleanup error and still reports success. Two consequences are accepted here
and tracked separately:

1. The stale `materialized_environment_id` is not repaired, so surviving group
   members that launch with an explicit `shared_group` policy still fail, and
   materialization still refuses to refresh a group that names any environment ID.
2. The producer is not fixed. Environment deletion does not reconcile the owning
   group row, so new dangling references keep appearing.

The producer defect and the backfill for the 50 existing rows are deliberately
not bundled into this repair. Which deletion path produced the current rows is
not established by available evidence, and guessing would put an unverified
migration in front of a verified one-call fix.

---

## Work orders

| Task | Title | Wave | Depends on |
| --- | --- | --- | --- |
| 01 | Tolerate an absent canonical environment on archive | 1 | — |
