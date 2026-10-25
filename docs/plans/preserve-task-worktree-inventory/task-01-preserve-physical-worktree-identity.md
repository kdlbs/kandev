---
id: "01-preserve-physical-worktree-identity"
title: "Preserve physical worktree identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
  - AC-TASKS-RUNTIME-CLEANUP-001.2
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 01: Preserve Physical Worktree Identity

## Summary

Make repository-inventory persistence merge-aware: an inventory-only launch
update must preserve the task environment's concrete worktree tuple, while a
new concrete worktree result must still replace stale values. Prove the repair
with a failing executor regression test before changing production code.

## In scope

- Add
  `TestPersistTaskEnvironmentRepos_PreservesPhysicalWorktreeOnInventoryOnlyRefresh`
  in a dedicated executor persistence test file because the existing test files
  already exceed the backend's effective test-file lint limit.
- Change the existing-row merge and refresh predicate in
  `executor_execute.go` so a missing incoming `WorktreeID` cannot clear a valid
  physical worktree tuple.
- Preserve existing concrete-refresh and legacy-flat-row migration behavior.
- Run the downstream workspace-info projection test.

## Out of scope

- Frontend or WebSocket changes.
- Database schema changes or migrations.
- Automatic repair of already-empty canonical rows.
- Live-instance database mutation.
- PR comparison-target changes.

## Acceptance

- An inventory-only projection for an existing repository/branch row leaves its
  `worktree_id`, `worktree_path`, and `worktree_branch` unchanged.
- A projection with a concrete non-empty `WorktreeID` continues to replace all
  three physical worktree fields.
- Workspace-info recovery continues to project a retained canonical worktree
  path for the session after the persistence update.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor -run 'TestPersistTaskEnvironmentRepos_(PreservesPhysicalWorktreeOnInventoryOnlyRefresh|RefreshesExistingRows|MigratesLegacyFlatRowToBranchIdentity)$'
cd apps/backend && go test ./internal/task/service -run '^TestGetWorkspaceInfoForSession_ProjectsPersistedWorktreeIdentity$'
cd apps/backend && go test ./internal/orchestrator/executor ./internal/task/service
```

The first command must fail on the new preservation assertion before production
code changes and pass afterward.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_repo_persistence_test.go`
- `docs/plans/preserve-task-worktree-inventory/plan.md`
- `docs/plans/preserve-task-worktree-inventory/task-01-preserve-physical-worktree-identity.md`

## Dependencies

None.

## Risks

- The merge must use concrete worktree identity, not path presence alone, so
  remote inventory rows cannot be misclassified as physical worktrees.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001`, especially AC `.1` and
  `.2`.
- `REQ-TASKS-RUNTIME-CLEANUP-001`, especially AC `.2`.
- `docs/specs/tasks/system-design/runtime-cleanup.md`, section
  `task_environments and task_environment_repos`.
- `docs/decisions/2026-08-08-task-owned-worktree-lifetime.md`.
- Existing concrete-refresh tests in `executor_multi_repo_test.go`.

## Results

Implemented and verified. The regression failed before the production change,
then passed after inventory-only rows stopped replacing the physical worktree
tuple. The concrete-refresh and legacy-flat-row tests, the downstream
workspace-info projection test, and both full executor/service packages pass.
