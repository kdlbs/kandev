---
id: "02-preserve-archive-branch"
title: "Preserve archive branch refs"
status: done
wave: 2
depends_on:
  - "01-retain-archive-environment"
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.7
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 02: Preserve Archive Branch Refs

## Summary

Carry one branch-preserving policy through every archive worktree cleanup pass.
Then prove that unarchive can reactivate a branch that has no remote ref.

## In scope

- Add a worktree batch operation with an explicit branch-removal policy.
- Keep the existing batch cleanup entry point compatible with delete cleanup.
- Make direct and cascade archive use `removeBranch=false` in every pass.
- Fail archive cleanup closed when a cleaner cannot preserve branches.
- Add real-Git coverage for a local-only branch.
- Add an archive, unarchive, and rematerialization integration regression.
- Run the related service, worktree, and executor suites with the race detector.

## Out of scope

- Pushing or recreating remote branches.
- Restoring uncommitted files from snapshot data.
- New UI recovery actions.
- Changing delete cleanup behavior.

## Acceptance

- Archive removes the physical worktree directory but keeps the local branch
  ref at the archived head commit.
- A duplicate batch cleanup pass preserves that branch.
- Cascade archive uses the same policy.
- Delete cleanup keeps its current policy.
- Unarchive resume reactivates the same environment repository row and worktree
  identity without a remote branch.
- The recreated worktree points at the preserved commit.

## TDD sequence

1. Add a failing batch-cleanup test for `removeBranch=false`.
2. Add a failing direct-archive integration test with a local-only branch.
3. Add the branch-preserving manager entry point and task cleanup wiring.
4. Confirm the archive job succeeds and the local branch survives.
5. Unarchive and run normal worktree preparation.
6. Assert that the recorded worktree becomes active at the same commit.
7. Run focused, package, race, lint, and specification checks.

## Verification

```bash
# From apps/backend:
rtk go test ./internal/worktree -run 'TestCleanupWorktrees_(PreservesBranchWhenRequested|RemovesBranchByDefault)$' -count=1
rtk go test ./internal/task/service -run '^TestArchiveUnarchiveResumeReactivatesLocalOnlyBranch$' -count=1
rtk go test ./internal/worktree -run 'CleanupWorktrees|RestoresReleasedWorktreeAfterArchive|Recreate' -count=1
rtk go test -race ./internal/task/service ./internal/worktree ./internal/orchestrator/executor

# From the repository root:
rtk make -C apps/backend lint
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
```

## Files likely touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_archive_identity_test.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_branch_policy_test.go`

## Dependencies

- Task 01 preserves the environment and repository identities that resume uses.

## Risks

- Do not infer archive policy from row-deletion flags. Use the lifecycle
  operation or an explicit cleanup disposition.
- A compatibility fallback must never call branch-deleting cleanup for an
  archive.
- The end-to-end test needs no remote ref. A remote ref would hide the defect.

## Parallelism

Sequential.

## Inputs

- `REQ-TASKS-RUNTIME-CLEANUP-001`, especially AC `.7`.
- `docs/specs/tasks/system-design/runtime-cleanup.md`, section
  `Archive cleanup disposition`.
- The environment destroyer adapter in `backendapp/worktree.go`.
- Batch cleanup in `worktree/manager_cleanup.go`.
- Worktree recreate coverage in `manager_recreate_recovery_test.go`.

## Results

Added a branch-preserving batch cleanup operation and kept the existing method
as the branch-removing delete path. Direct archive, durable cascade archive,
and the legacy cascade path now select the preserving operation. Archive
cleanup fails for retry when its cleaner cannot preserve branches.

The real-Git regression failed on the previous behavior because the second
cleanup pass deleted the local-only branch. It now archives, unarchives, and
reactivates the same environment repository row and worktree ID at the saved
commit.

Verification passed:

```text
rtk go test ./internal/worktree -run 'TestCleanupWorktrees_(PreservesBranchWhenRequested|RemovesBranchByDefault)$' -count=1
rtk go test ./internal/task/service -run '^TestArchiveUnarchiveResumeReactivatesLocalOnlyBranch$' -count=1
rtk go test ./internal/worktree -run 'CleanupWorktrees|RestoresReleasedWorktreeAfterArchive|Recreate' -count=1
rtk go test -race ./internal/task/service ./internal/worktree ./internal/orchestrator/executor
rtk make -C apps/backend lint
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rtk git diff --check
```
