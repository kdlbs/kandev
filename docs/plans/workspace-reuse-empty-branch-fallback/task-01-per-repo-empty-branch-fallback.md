---
id: "01-per-repo-empty-branch-fallback"
title: "Scope the empty-branch fallback per repository"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.3
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Scope the empty-branch fallback per repository

## Summary

Change `canonicalInventoryMatches` so the legacy empty-branch fallback applies
per repository instead of globally, and recognize a non-empty branch slug as
branch-scoped even when the row has no worktree ID. A repository that already
has a `main` row must not also match a stale empty-branch row, restoring an
exact match of one row per branch slot.

## In scope

- Add `repositoryHasBranchScopedRow(rows, repositoryID)`.
- Update `canonicalInventoryMatches` to use the per-repository predicate.
- Add the two regression tests named in the plan.

## Out of scope

- Worktree-ID reuse fallback changes.
- Decommissioning stale legacy repo rows.
- Schema changes or new persistence models.

## Acceptance

- `canonicalInventoryMatches` counts exactly one row for a
  `{repo, main}` slot when that repository has a `main` row (no worktree) plus a
  legacy empty-branch worktree row, under `useWorktree=false`.
- `validateReuseEnvironmentInventory` returns `nil` for that inventory instead of
  `ErrWorkspaceReuseUnsafe`.
- The legacy-only case (single empty-branch row, no scoped rows) still matches.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor -run 'TestCanonicalInventoryMatches|TestValidateReuseEnvironmentInventory' -count=1
cd apps/backend && go test ./internal/orchestrator/executor -count=1
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_test.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse_inventory_test.go`

## Dependencies

None

## Risks

- Do not broaden or narrow the guard's mismatched-inventory refusal beyond the
  per-repository fallback scoping; `MismatchedRowsStillRefused` must still fail.
- Keep `repositoryHasBranchScopedRow` status-agnostic to match the existing
  predicate and avoid unrelated behavior changes.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/system-design/additional-session-workspace-reuse.md` section
  "Launch admission".
- Existing predicates `hasBranchScopedEnvironmentRepoRows` and the
  `canonicalInventoryMatches` guard in `executor_environment_reuse.go`.
- Existing `canonicalInventoryMatches` tests in `executor_environment_test.go`.

## Results

All acceptance criteria are met. `go test ./internal/orchestrator/executor -count=1` passes.
`python3 scripts/lint-spec-files.py --all` passes.
