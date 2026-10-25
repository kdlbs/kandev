---
spec: docs/specs/tasks/requirements/session-delete-resource-cleanup.md
created: 2026-08-09
status: implemented
---

# Fix Plan: Preserve Physical Identity Precedence During Worktree Cutover

## Overview

Repair the remaining startup regression in the one-time task-worktree ownership
cutover introduced by PR #2456 and partially repaired by PR #2463. The cutover
already loads sources from strongest to weakest, but it still verifies stale
session path and branch metadata against a higher-precedence row for the same
physical `worktree_id`. The repair makes that precedence consistent in merge and
inventory validation while preserving fail-closed behavior for different
physical identities.

Confirmed root cause: `mergeSessionWorktree` calls `verifyWorktree` for a
non-historical session even after `targetForWorktree` resolves the identical
physical worktree from `task_environment_repos` or the surviving flat
`task_environments` row. `legacyWorktreeInventory` likewise retains that stale
session metadata unless the row satisfies the narrower deleted/terminal
classification. A valid legacy database therefore reports branch conflicts and
aborts startup despite having one unambiguous worktree owner.

## Backend

### Source-aware physical identity precedence

- Update
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
  and
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_targets.go`
  so a session-worktree row is classified as a lower-precedence duplicate when
  a canonical repository row or the surviving flat environment already carries
  the same task and physical `worktree_id`.
- Apply the same classification while merging sources and while constructing
  the legacy inventory. This prevents the merge from rejecting stale metadata
  and prevents validation from reintroducing the discarded metadata variant as
  a missing inventory item.
- Keep the existing deleted/terminal compatibility rules. Continue failing
  closed when live rows claim different physical worktree identities for the
  same repository slot or when ownership cannot otherwise be resolved.

### Upgrade safety

- Keep the pre-migration snapshot, single transaction, shadow-table checks,
  schema rollback, and replay behavior unchanged.
- Do not read or mutate Git worktrees during migration. Resolve precedence only
  from the persisted physical identity and the established source ordering.

## Tests

- **What:** A resumable `WAITING_FOR_INPUT` session with stale branch metadata
  cannot override a canonical `task_environment_repos` row carrying the same
  `worktree_id`.
  **File:**
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`.
  **How:** Seed the three legacy representations with one physical ID and
  different session metadata; run the full repository initializer and assert
  that cutover completes with canonical path/branch and unchanged session state.
- **What:** A legacy session row cannot override the surviving flat environment
  when no canonical repository row exists and both carry the same physical ID.
  **File:** same migration test file.
  **How:** Seed flat and session sources with branch drift and assert that the
  normalized repository row retains the flat metadata. Cover resumable and
  terminal session states so precedence is not state-dependent.
- **What:** Different live physical worktree IDs remain an unresolved conflict.
  **File:** same migration test file.
  **How:** Retain and run `TestCutover_ConflictingWorktreesFailClosed`, including
  its assertions that the legacy schema and data survive rollback.
- **What:** All existing SQLite cutover compatibility, failpoint, replay, and
  inventory cases remain valid.
  **File:** same migration test file.
  **How:** Run the focused cutover test group and then the repository package.

## Verification Results

- RED: the new canonical-versus-resumable and flat-versus-session fixtures
  failed with the same stale branch conflict reported by the affected database.
- `go test ./internal/task/repository/sqlite -run
  'TestCutover_(PrefersCanonicalMetadataForResumableSession|PrefersFlatMetadataForSameWorktree|ConflictingWorktreesFailClosed)'
  -count=1` - passed.
- `go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1` -
  passed.
- `go test ./internal/task/repository/sqlite -count=1` - passed.
- A throwaway `-tags fts5` repository-initializer test against a copied affected
  database completed the cutover, removed the legacy ownership table, and
  produced 295 normalized repository rows. The harness and copies were removed;
  the live database and workspaces were untouched.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [task-01-repair-identity-precedence](task-01-repair-identity-precedence.md) - done

No task is parallel-safe because the normalization logic and its migration
fixtures share the same persistence boundary.

## Risks

- Treating path or branch equality as ownership would preserve the current
  false-positive failure. The bypass must require an identical non-empty
  physical `worktree_id` from a higher-precedence source.
- Broadly ignoring session conflicts would conceal competing worktrees. Existing
  different-ID fail-closed coverage must remain unchanged.
- Merge and inventory classification can drift apart and fail later in
  validation. Both paths must use the same source-aware predicate.

## Out Of Scope

- Manual edits to user databases or ownership rows.
- Filesystem or Git reconciliation during startup.
- Changes to task/session lifecycle behavior after the one-time cutover.
- Additional public recovery documentation; the current operations and
  Kubernetes pages already describe snapshot-based rollback and forbid manual
  row deletion.

## PR Fixup Results

- Resolved the invalid request to remove the repair spec amendment: the spec is
  `draft`, and the repository `/fix` workflow requires the affected behavioral
  spec to be amended before implementation.
- Added explicit `RUNNING` coverage to the flat-environment precedence fixture
  and simplified the classifier to perform one session lookup.
- `GOCACHE=/tmp/kandev-go-cache go test
  ./internal/task/repository/sqlite -run
  'TestCutover_(PrefersCanonicalMetadataForResumableSession|PrefersFlatMetadataForSameWorktree|ConflictingWorktreesFailClosed)'
  -count=1` - passed.
