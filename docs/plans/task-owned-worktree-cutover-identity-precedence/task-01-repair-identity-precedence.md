---
id: "01-repair-identity-precedence"
title: "Repair cutover physical identity precedence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/session-delete-resource-cleanup.md"
---

# Task 01: Repair Cutover Physical Identity Precedence

## Acceptance

- A higher-precedence repository or surviving flat environment row wins stale
  path/branch metadata from a session row only when both carry the same non-empty
  physical `worktree_id` for the task.
- Merge and inventory validation apply the same source-aware duplicate
  classification for resumable, terminal, and deleted legacy sessions.
- Different live physical identities remain fatal and the cutover transaction
  preserves the full legacy schema and data on failure.
- The migration remains database-only, replay-safe, and covered for SQLite and
  PostgreSQL through the existing shared normalization path.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover_(PrefersCanonicalMetadataForResumableSession|PrefersFlatMetadataForSameWorktree|ConflictingWorktreesFailClosed)' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite -count=1
```

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_targets.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`
- `docs/specs/tasks/requirements/session-delete-resource-cleanup.md`
- `docs/plans/task-owned-worktree-cutover-identity-precedence/plan.md`
- `docs/plans/task-owned-worktree-cutover-identity-precedence/task-01-repair-identity-precedence.md`

## Dependencies

None.

## Parallelism

Sequential. The source precedence predicate, merge behavior, inventory
validation, and regression fixtures share one migration boundary.

## Inputs

- The schema normalization and failure-mode sections of the repair spec.
- Existing `mergeSessionWorktree`, `isSupersededHistoricalWorktree`,
  `legacyWorktreeInventory`, and `targetForWorktree` behavior.
- Existing canonical-precedence, terminal-history, failpoint, replay, SQLite,
  and PostgreSQL cutover tests.
- Read-only affected database evidence: all 16 reported conflicts reuse the
  exact higher-precedence `worktree_id`; 9 are canonical repository versus
  `WAITING_FOR_INPUT` session drift and 7 are flat environment versus terminal
  session drift.

## Output Contract

Report the predicate and precedence changes, exact test commands and results,
any state or identity assumptions, cleanup of temporary reproduction artifacts,
and synchronized task/plan status.

## Results

- Added a task-scoped index of physical IDs already owned by canonical
  repository rows or the surviving flat environment. Legacy session rows use
  the same superseded-source predicate during merge and inventory validation.
- Preserved the existing strict verifier for session-only duplicates and the
  existing fail-closed behavior for different live physical IDs.
- Added SQLite migration fixtures for canonical-versus-resumable and
  flat-environment-versus-session branch drift, covering `WAITING_FOR_INPUT`
  and `CANCELLED` session states.
- RED: `go test ./internal/task/repository/sqlite -run
  'TestCutover_(PrefersCanonicalMetadataForResumableSession|PrefersFlatMetadataForSameWorktree)'
  -count=1` failed with the expected stale branch conflicts.
- GREEN: `go test ./internal/task/repository/sqlite -run
  'TestCutover_(PrefersCanonicalMetadataForResumableSession|PrefersFlatMetadataForSameWorktree|ConflictingWorktreesFailClosed)'
  -count=1` passed.
- `go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1`
  passed.
- `go test ./internal/task/repository/sqlite -count=1` passed.
- Copied the affected SQLite database to `/tmp` and ran the repository
  initializer against the copy with `go test -tags fts5`; the cutover completed,
  removed the legacy ownership table, and produced 295 normalized repository
  rows while retaining at least the 129 pre-existing canonical rows.
- The first copied-database run without a temporary `GOCACHE` was blocked by
  sandbox cache permissions; the next run without `-tags fts5` failed because
  the developer SQLite driver lacked FTS5. The tagged rerun passed.
- Removed the temporary copied databases and throwaway reproduction test. No
  filesystem, Git worktree, or live user database was mutated.
- PR fixup added explicit `RUNNING` coverage for flat-environment precedence
  and replaced duplicate session-map lookups with one lookup before the
  supersession decision.
- The first focused fixup test could not write the sandboxed default Go cache;
  `GOCACHE=/tmp/kandev-go-cache go test
  ./internal/task/repository/sqlite -run
  'TestCutover_(PrefersCanonicalMetadataForResumableSession|PrefersFlatMetadataForSameWorktree|ConflictingWorktreesFailClosed)'
  -count=1` passed.
