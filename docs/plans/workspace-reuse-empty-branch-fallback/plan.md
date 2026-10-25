---
created: 2026-09-11
status: complete
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Implementation Plan: Workspace Reuse Empty-Branch Fallback Scope

## Overview

Make the canonical-inventory match for a repository/branch slot scoped per
repository so a legacy empty-branch row no longer double-counts against an
already branch-scoped slot. This removes a false `workspace_reuse_unsafe`
refusal for inherited `local` environments that carry both a scoped branch row
and a stale empty-branch worktree row.

## Scope

### In scope

- Scope the empty-branch fallback in `canonicalInventoryMatches` per repository.
- Recognize a non-empty-branch row as branch-scoped regardless of worktree ID.
- Add a regression test at the `canonicalInventoryMatches` and
  `validateReuseEnvironmentInventory` boundaries.

### Out of scope

- Deleting, cleaning, or rewriting stale worktree repo rows on executor drift.
- Changing the worktree-ID reuse fallback (`reuseExistingRepositoryWorktrees`).
- Any schema migration or new workspace ledger.

## Confirmed root cause

`canonicalInventoryMatches` gates the legacy empty-branch fallback on the global
`hasBranchScopedEnvironmentRepoRows` predicate. That predicate requires
`RepositoryID != "" && WorktreeID != "" && sanitize(BranchSlug) != ""`, so a
`local` executor environment whose only scoped row is
`{branch: "main", worktree: ""}` is treated as unscoped and enables the fallback.
A stale legacy empty-branch worktree row (`{branch: "", worktree_id: set}`) then
also matches the `main` slot, yielding `matches == 2`; the guard rejects because
it requires `== 1`. The runtime surfaces this as "canonical workspace repository
inventory has no matching entry for repository ... branch main", which is
misleading: the slot over-matches, it is not missing.

## Technical approach

- Add `repositoryHasBranchScopedRow(rows, repositoryID) bool` that returns true
  when any row for `repositoryID` carries a non-empty sanitized branch slug,
  independent of worktree ID or status.
- In `canonicalInventoryMatches` (`apps/backend/internal/orchestrator/executor/executor_environment_reuse.go:115`),
  replace `!hasBranchScopedEnvironmentRepoRows(rows)` with
  `!repositoryHasBranchScopedRow(rows, spec.RepositoryID)`.
- Leave the existing `hasBranchScopedEnvironmentRepoRows` predicate and the
  worktree-ID reuse path untouched.
- Prefer `newMockRepository()` / `newTestExecutor()` fixtures already used by
  `executor_environment_reuse_inventory_test.go` and `executor_environment_test.go`.

## Tests

- `TestCanonicalInventoryMatches_ScopedLocalBranchSuppressesLegacyFallback`:
  spec `{repo-1, main}`, rows `[{repo-1, main}, {repo-1, "", worktree set}]`,
  `useWorktree=false`, expect `1` match (fails before the correction with `2`).
- `TestValidateReuseEnvironmentInventory_ScopedBranchPlusLegacyEmptyRowAttaches`:
  same rows via mock repo, `WorkspaceReuseRequired=true`, expect `nil` error.
- Existing `TestCanonicalInventoryMatches_AcceptsLegacyUnscopedRowWhenNoScopedRowsExist`
  and `TestValidateReuseEnvironmentInventory_MismatchedRowsStillRefused` must
  keep passing.

## Work orders

- [x] [Task 01: Scope the empty-branch fallback per repository](task-01-per-repo-empty-branch-fallback.md)

## Verification results

`go test ./internal/orchestrator/executor -run 'TestCanonicalInventoryMatches|TestValidateReuseEnvironmentInventory' -count=1` passes.
`go test ./internal/orchestrator/executor -count=1` passes.
`python3 scripts/lint-spec-files.py --all` passes.

## Risks

- The deployed backend is pinned to `8b64aed17` (v0.94.0-40) while upstream
  `origin/main` has advanced to `8e38e2a0` (v0.94.0-50). The changed file
  `executor_environment_reuse.go` is identical on both, but the PR base and the
  deploy target must be reconciled by the operator at pull-request time.
- `repositoryHasBranchScopedRow` intentionally ignores row status, matching the
  existing predicate's behavior; do not introduce status-aware scoping in this
  change without a separate requirement.
- The worktree-ID reuse fallback must not be altered, or it could regress
  branch-scoped worktree attachment for the Worktree executor.
