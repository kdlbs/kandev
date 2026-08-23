---
spec: docs/specs/tasks/requirements/session-delete-resource-cleanup.md
created: 2026-08-10
status: complete
---

# Fix Plan: Normalize Terminal Worktree History

## Overview

Issue #2505 reports a startup error during the task-worktree ownership cutover.
The repair extends the existing source precedence for terminal session history.
The cutover will keep the surviving flat environment owner for the same
repository slot, even when the terminal row has an older worktree ID.

## Root Cause

The cutover loads the surviving flat environment before legacy session rows.
`isSupersededSessionWorktree` discards a terminal row with a different worktree
ID only when `task_environment_repos` owns the same repository slot. It does not
apply this rule when the surviving flat environment owns that slot. Therefore,
`mergeSessionWorktree` tries to claim the terminal row and returns the exact
`worktree A conflicts with B` error from issue #2505.

A focused temporary test reproduced the reported error. The fixture used a
`COMPLETED` session, an older session worktree, and a newer flat environment
worktree for the same repository slot. The temporary test passed because it
expected the current startup error, and it was then removed.

## Backend

### Terminal history classification

Update
`apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`.
Extend `isSupersededSessionWorktree` with one source-precedence helper. The
helper must recognize the surviving flat environment as the owner only when all
of these conditions are true:

- The session state is `COMPLETED`, `FAILED`, or `CANCELLED`.
- The flat environment belongs to the task that owns the session environment.
- The repository ID matches the session row.
- The legacy session row uses an empty `branchSlug`. The flat environment has
  no `branchSlug` field.
- The flat environment has a non-empty worktree ID.

Keep the current canonical repository-row rule. Keep deleted-row handling and
same-identity precedence unchanged. Non-terminal rows with different physical
IDs must still stop the migration.

The existing `isSupersededSessionWorktree` predicate is also used by inventory
and owner validation. Therefore, merge and validation will use the same rule.

## Tests

- **What:** A terminal session row with an older worktree ID does not override
  the surviving flat environment for the same repository slot.
  **File:**
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`.
  **How:** Use a table-driven SQLite migration test for `COMPLETED`, `FAILED`,
  and `CANCELLED`. Assert that startup succeeds and retains the flat owner.
- **What:** A resumable session row with a different worktree ID still stops
  the migration.
  **File:** the same migration test file.
  **How:** Seed `RUNNING` and `WAITING_FOR_INPUT` states against a flat owner.
  Assert the conflict and the complete legacy rollback.
- **What:** Terminal history outside the flat repository slot remains.
  **File:** the same migration test file.
  **How:** Use a different repository ID or a non-empty `branchSlug`. Assert
  that the migration retains both worktrees.
- **What:** Existing cutover precedence, inventory, rollback, and replay
  behavior remains valid.
  **File:** the existing SQLite repository test package.
  **How:** Run the focused tests, all cutover tests, and the package tests.

## Verification Results

- TDD red: `TestCutover_IgnoresTerminalWorktreeDifferentFromFlatOwner` failed
  for `COMPLETED`, `FAILED`, and `CANCELLED` with the reported flat-owner
  worktree conflict before the production change.
- The shared supersession predicate now recognizes a surviving flat environment
  for the owning task, matching repository ID and the legacy empty branch slot,
  only for terminal session history. Canonical repository precedence remains
  unchanged, and live sessions still fail closed.
- Focused regressions passed: `8 passed`.
- All cutover tests passed: `41 passed`.
- SQLite repository package passed: `385 passed`.
- `git diff --check` passed.
- Changed files are limited to the cutover predicate, its migration tests, the
  amended repair spec, and this plan/task package. No temporary reproduction
  files remain.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [task-01-normalize-terminal-history](task-01-normalize-terminal-history.md)

No task is parallel-safe. The classification and migration tests share one
persistence boundary.

## Risks

- A broad terminal-state bypass can discard a worktree from another repository
  or branch slot. The helper must match the exact slot.
- A non-terminal bypass can hide two active physical owners. The current
  fail-closed rule must remain.
- Merge and inventory rules can diverge. Both paths must use the existing shared
  supersession predicate.

## Out of Scope

- Manual changes to an affected database.
- File-system or Git reconciliation during startup.
- Changes to task or session behavior after the one-time cutover.
- Public recovery documentation. The current backup guidance remains correct.
