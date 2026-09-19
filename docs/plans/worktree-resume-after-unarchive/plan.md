---
created: 2026-08-24
status: done
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Worktree Resume After Unarchive

## Overview

Correct resume routing for an archived and then unarchived worktree task. The
executor will reserve attach-only reuse for environments with a matching
executor and a live physical worktree row; otherwise it will use the existing
worktree recreate path. One sequential work order owns the regression test,
the narrow decision-helper change, and targeted verification.

## Confirmed root cause

`buildResumeRequest` requests workspace reuse when the existing environment was
not materialized by the resuming session. `workspaceReuseAllowed` validates a
live worktree row only for legacy environments whose `ExecutorType` is empty.
For a normal worktree environment it currently returns true solely because the
executor types match, even if every repository row is deleted, failed, or
tombstoned. This selects `worktree.Manager.reuseRequiredWorktree`, whose
attach-only contract correctly rejects the non-active or missing worktree with
`ErrReuseWorktreeUnavailable`.

## Scope

### In scope

- Require a live physical worktree repository row before enabling attach-only
  reuse for every worktree environment, including environments with a persisted
  executor type.
- Preserve executor-type matching and all non-worktree reuse behavior.
- Add table-driven regression coverage for deleted, failed, live, mismatched,
  non-worktree, and `required=false` cases.
- Exercise the existing manager recovery test that proves a released worktree
  row can be reactivated after archive cleanup.

### Out of scope

- Weakening `reuseRequiredWorktree` or allowing attach-only reuse of deleted
  paths.
- Changing archive cleanup, branch recovery, unarchive APIs, schema, or store
  persistence.
- Recovering a branch that is absent both locally and remotely.
- Changing reuse behavior for a live worktree or any non-worktree executor.

## Technical approach

In `executor_execute.go`, keep the current early return when reuse is not
required or no environment exists. For an environment with a persisted
executor type, reject a requested type mismatch first. For any requested
worktree executor, delegate to a small `hasLiveWorktreeRepo` helper. The helper
uses the existing live-row predicate: a non-empty `WorktreeID`, nil
`DeletedAt`, and a status other than failed or deleted. Legacy non-worktree
environments remain attachable as before.

When the helper returns false during resume, existing downstream behavior is
already suitable:

- `applyResumeWorktreeConfig` stamps `TaskDirName` and `RepoName`.
- `reuseExistingEnvironment` may carry a historical worktree ID but does not
  require attach-only preparation.
- `worktree.Manager.Create` can resolve a soft-deleted row by ID, recreate the
  recoverable branch and directory, and update the row to active with
  `deleted_at` cleared.
- If no historical ID exists, `SQLiteStore.CreateWorktree` uses an upsert on
  `(task_environment_id, repository_id, branch_slug)`, so a soft-deleted row
  does not cause a unique-constraint failure.

No repository or schema method needs to change.

## Tests

- **AC-TASKS-RUNTIME-CLEANUP-001.7:** Add
  `TestWorkspaceReuseAllowedRequiresLiveWorktreeRepo` in a new executor test
  file. The current `executor_environment_test.go` is already above the backend
  800-line file limit, so the sibling file avoids extending an oversized test.
  The regression case must fail before the production change because a normal
  worktree environment with only a deleted row is incorrectly attachable.
- Preserve `TestWorkspaceReuseAllowedRequiresMatchingExecutorType` and the
  `TestReuseExistingEnvironment_*` suite.
- Run `TestCreate_RestoresReleasedWorktreeAfterArchive` as existing integration
  evidence for recreate/reactivation and soft-deleted-row persistence.

## Work orders

- [done] [Task 01: Gate Worktree Reuse on Live Inventory](task-01-gate-worktree-reuse.md)

## Verification results

- RED: `go test ./internal/orchestrator/executor -run '^TestWorkspaceReuseAllowedRequiresLiveWorktreeRepo$' -race`
  failed on the four invalid inventory cases before the production change.
- GREEN: the focused regression passed with 8 cases.
- `go test ./internal/orchestrator/executor/... -run 'WorkspaceReuse|ReuseExistingEnvironment' -race`
  passed with 31 tests.
- `go test ./internal/worktree/... -run 'TestCreate_RestoresReleasedWorktreeAfterArchive' -race`
  passed with 1 test in 2 packages.
- `go test ./internal/orchestrator/... ./internal/worktree/... -race` passed with
  3,362 tests across 11 packages.
- `make -C apps/backend lint` passed with 0 issues.

## Risks

- The live-row predicate must remain identical to the canonical environment
  inventory predicate so attach eligibility and later validation cannot drift.
- A worktree branch absent locally and remotely still fails with the existing
  typed recovery error; this fix changes only the incorrect attach-only route.
- Multi-repository environments need only one live row for this coarse routing
  decision; the existing canonical inventory validator still requires exactly
  one live row for every requested repository/branch slot when attach-only
  reuse proceeds.
