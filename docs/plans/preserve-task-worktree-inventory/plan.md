---
created: 2026-08-24
status: done
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Preserve Task Worktree Inventory

## Overview

Prevent an inventory-only launch projection from erasing the canonical physical
worktree identity stored in `task_environment_repos`. The repair starts with a
focused executor regression test, then changes the existing-row merge so only a
concrete worktree result can replace physical worktree fields. This preserves
restart recovery and the Changes panel's branch-diff source without changing any
HTTP, WebSocket, or frontend contract.

The confirmed live-instance failure is:

1. A workspace-only execution reuses the task's correct worktree.
2. Agent promotion returns repository inventory without a concrete
   `WorktreeID`.
3. `persistTaskEnvironmentRepos` matches the canonical row by repository and
   branch identity.
4. `refreshTaskEnvironmentRepo` copies blank `worktree_id`, `worktree_path`, and
   `worktree_branch` fields over the existing physical identity.
5. After restart, workspace recovery cannot resolve `session.Worktrees`, falls
   back to the seed repository, and reports an empty diff even though the linked
   PR worktree remains intact.

This violates
`AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1`,
`AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2`, and the accepted ownership
rule in
[ADR-2026-08-08-task-owned-worktree-lifetime](../../decisions/2026-08-08-task-owned-worktree-lifetime.md):
`task_environment_repos` is the sole physical-worktree source of truth and only
task lifecycle operations may remove that identity.

## Scope

### In scope

- Preserve a valid canonical worktree ID, path, and branch when a launch update
  carries repository inventory but no concrete physical worktree.
- Continue replacing stale physical fields when a successful launch carries a
  non-empty concrete `WorktreeID`.
- Add focused regression coverage for both preservation and concrete refresh.
- Run the existing workspace-info projection test that proves restart recovery
  reads the persisted worktree identity.

### Out of scope

- Frontend rendering or Changes-panel state changes.
- New database columns, migrations, compatibility readers, or automatic
  filesystem discovery.
- Automatic repair of rows already corrupted by the regression. The accepted
  ownership ADR requires missing or ambiguous inventory to fail closed rather
  than silently select or create another physical owner; live-data repair needs
  a separately authorized, exact operational action.
- PR association or comparison-target behavior; its `no_match` result is a
  downstream symptom of the lost workspace identity.

## Technical approach

### Existing-row merge

Update `refreshTaskEnvironmentRepo` and its refresh predicate in
`apps/backend/internal/orchestrator/executor/executor_execute.go` so an incoming
row without `WorktreeID` is treated as inventory-only. Such a row may update
non-physical inventory metadata such as position and error state, but it cannot
clear or replace an existing physical worktree tuple.

When the incoming row contains `WorktreeID`, retain the current concrete-refresh
behavior and update `worktree_id`, `worktree_path`, and `worktree_branch`
together. Explicit cleanup and environment-reset paths remain the only owners
that remove physical identity.

### Regression boundary

Add the regression in the dedicated
`apps/backend/internal/orchestrator/executor/executor_environment_repo_persistence_test.go`
file because the existing persistence test files already exceed the backend's
effective test-file lint limit. Seed an active canonical row with a concrete
worktree, pass the inventory-only shape produced by `environmentReposForLaunch`,
and assert the stored physical tuple is unchanged. Keep the existing
concrete-refresh test green to prove the repair does not freeze stale worktree
data.

Use the existing
`TestGetWorkspaceInfoForSession_ProjectsPersistedWorktreeIdentity` service test
as downstream recovery evidence: when the canonical row remains intact,
workspace recovery selects the task worktree instead of the repository snapshot.

## Tests

- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1`: a canonical repository
  row remains complete after an inventory-only promotion update.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2`: promotion does not replace
  or rematerialize the existing physical workspace.
- `AC-TASKS-RUNTIME-CLEANUP-001.2`: a non-lifecycle session operation does not
  discard the task-owned worktree identity.

The new test is
`TestPersistTaskEnvironmentRepos_PreservesPhysicalWorktreeOnInventoryOnlyRefresh`
in `executor_environment_repo_persistence_test.go`. It failed before the
correction because the current merge blanked all three physical fields.

## E2E tests

No new Playwright test is planned. The UI consumes the backend's live git-status
projection and did not fail in the captured frontend diagnostics. The defect is
the durable backend worktree tuple, and the focused persistence plus workspace
projection tests exercise the boundary that determines the Changes source after
restart.

## Work orders

- [x] [Task 01: Preserve physical worktree identity](task-01-preserve-physical-worktree-identity.md)

## Verification results

- RED confirmed with the new regression before the production change: the
  inventory-only refresh cleared the stored physical worktree tuple.
- `go test ./internal/orchestrator/executor -run
  'TestPersistTaskEnvironmentRepos_(PreservesPhysicalWorktreeOnInventoryOnlyRefresh|RefreshesExistingRows|MigratesLegacyFlatRowToBranchIdentity)$'`
  passed.
- `go test ./internal/task/service -run
  '^TestGetWorkspaceInfoForSession_ProjectsPersistedWorktreeIdentity$'` passed.
- `go test ./internal/orchestrator/executor ./internal/task/service` passed
  (1,710 tests across both packages).

## Risks

- Treating every blank incoming field as destructive would reproduce the bug;
  `WorktreeID` must remain the discriminator between inventory-only and concrete
  physical updates.
- Preserving fields must not block a later successful recreate from replacing a
  stale worktree tuple; the existing concrete-refresh regression remains part of
  the required check.
- Already-corrupted environments remain unsafe until explicitly repaired or
  reset. This package prevents further corruption but does not guess ownership
  from filesystem state.
