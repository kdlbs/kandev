---
id: "01-gate-worktree-reuse"
title: "Gate worktree reuse on live inventory"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.7
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 01: Gate Worktree Reuse on Live Inventory

## Summary

Make the executor select attach-only worktree reuse only when the existing task
environment contains a live physical worktree row. Archived and unarchived
environments with deleted inventory will enter the existing recreate path.

## In scope

- Add a failing unit regression for normal worktree environments whose only
  physical row is deleted or failed.
- Extract and use a live-worktree-row helper in `workspaceReuseAllowed`.
- Preserve executor mismatch, legacy, non-worktree, and `required=false`
  behavior.
- Run the existing worktree recreate/reactivation integration test.

## Out of scope

- Changes to worktree attach validation, archive cleanup, branch recovery,
  persistence, migrations, frontend behavior, or public documentation.
- New recovery behavior when the historical branch is absent locally and
  remotely.

## Acceptance

- A requested worktree executor returns false for attach-only reuse when every
  environment repository row is deleted, failed, tombstoned, or lacks a
  worktree ID, regardless of whether `ExecutorType` is legacy-empty or set.
- A matching worktree environment with a live row remains reusable, while
  executor mismatch and `required=false` return false and matching non-worktree
  executors remain reusable.
- Existing environment reuse and released-worktree recreation tests pass.

## Verification

Run the new regression before the production change and confirm it fails for
the deleted-row assertion. After the minimal fix, run:

```bash
# From apps/backend:
rtk go test ./internal/orchestrator/executor/... -run 'WorkspaceReuse|ReuseExistingEnvironment' -race
rtk go test ./internal/worktree/... -run 'TestCreate_RestoresReleasedWorktreeAfterArchive' -race
rtk go test ./internal/orchestrator/... ./internal/worktree/... -race

# From the repository root:
rtk make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_workspace_reuse_test.go`
- `docs/plans/worktree-resume-after-unarchive/plan.md`
- `docs/plans/worktree-resume-after-unarchive/task-01-gate-worktree-reuse.md`

## Dependencies

None.

## Risks

- A divergent live-row predicate could route an unsafe inventory into
  attach-only preparation. Reuse the existing failed/deleted status constants
  and exact tombstone checks.
- The existing environment may contain a historical ID. Normal preparation must
  remain allowed to use it for branch recovery; do not clear it merely because
  attach-only reuse is disabled.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-RUNTIME-CLEANUP-001` and
  `AC-TASKS-RUNTIME-CLEANUP-001.7`.
- `docs/specs/tasks/system-design/runtime-cleanup.md`.
- Existing `workspaceReuseAllowed`, `canonicalInventoryMatches`,
  `reuseExistingRepositoryWorktrees`, `applyResumeWorktreeConfig`, and
  `worktree.Manager.Create` behavior.

## Results

- Added `TestWorkspaceReuseAllowedRequiresLiveWorktreeRepo` covering deleted,
  failed, tombstoned, missing-ID, live, mismatch, non-worktree, and optional
  reuse cases.
- `workspaceReuseAllowed` now gates attach-only worktree reuse on a live
  physical environment row while preserving executor matching and legacy
  non-worktree reuse.
- Corrected the multi-repository inventory test fixture to provide the same
  hydrated environment rows as the production repository implementation.
- Verification passed: the focused executor/worktree race tests, the complete
  orchestrator/worktree race suite (3,362 tests), and backend lint (0 issues).
