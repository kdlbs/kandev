---
id: "01-normalize-terminal-history"
title: "Normalize terminal worktree history"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/session-delete-resource-cleanup/spec.md"
---

# Task 01: Normalize Terminal Worktree History

## Acceptance

- Terminal session history cannot override the surviving flat environment for
  the same repository and empty branch slot.
- `RUNNING` and `WAITING_FOR_INPUT` sessions with different physical worktree
  IDs still stop the cutover and preserve the legacy database.
- Merge, inventory, rollback, and replay tests pass with one shared
  supersession rule.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover_(IgnoresTerminalWorktreeDifferentFromFlatOwner|RejectsLiveWorktreeDifferentFromFlatOwner|ConflictingWorktreesFailClosed)' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite -count=1
```

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`
- `docs/specs/session-delete-resource-cleanup/spec.md`
- `docs/plans/task-owned-worktree-cutover-terminal-history/plan.md`
- `docs/plans/task-owned-worktree-cutover-terminal-history/task-01-normalize-terminal-history.md`

## Dependencies

None.

## Parallelism

Sequential. The source classification and migration tests share one
persistence boundary.

## Inputs

- The schema-normalization and failure-mode sections of the repair spec.
- The root-cause section of `plan.md`.
- Existing `isSupersededSessionWorktree`, `mergeSessionWorktree`, and
  `legacyWorktreeInventory` behavior.
- Existing terminal-history, physical-identity, rollback, and replay tests.
- Issue #2505 and the temporary reproduction result in `plan.md`.

## Output Contract

Report the classification change, files changed, and exact test results. Report
all temporary-file cleanup. Update this task and `plan.md` in the same
conversation.

## Results

- **RED:**
  `go test ./internal/task/repository/sqlite -run
  'TestCutover_IgnoresTerminalWorktreeDifferentFromFlatOwner' -count=1`
  failed for all three terminal states with `worktree wt-flat-old conflicts
  with wt-flat-current`.
- **Implementation:** extended `isSupersededSessionWorktree` with a shared
  flat-environment source-precedence helper. It requires a surviving
  task-owned flat environment, matching repository ID, the legacy empty branch
  slot, a non-empty flat worktree ID, and a terminal session state. Canonical
  repository-row precedence and live-session fail-closed behavior remain
  unchanged.
- **Focused regressions:**
  `go test ./internal/task/repository/sqlite -run
  'TestCutover_(IgnoresTerminalWorktreeDifferentFromFlatOwner|RejectsLiveWorktreeDifferentFromFlatOwner|ConflictingWorktreesFailClosed)' -count=1`
  passed with `8 passed`.
- **Cutover suite:**
  `go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1`
  passed with `41 passed`.
- **Package suite:** `go test ./internal/task/repository/sqlite -count=1`
  passed with `385 passed`.
- **Hygiene:** `git diff --check` passed. No temporary reproduction files were
  created during implementation.
