---
id: "01-compare-git-directory-identity"
title: "Compare Git directories by filesystem identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.3
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.4
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Compare Git Directories by Filesystem Identity

## Summary

Make local and worktree recovery recognize two differently cased paths as the
same repository when the filesystem reports the same underlying Git common
directory. Preserve the existing typed rejection for missing or unrelated
directories.

## In scope

- Add a failing case-alias regression for repository identity validation.
- Compare resolved Git common directories with `os.Stat` and `os.SameFile`.
- Keep missing-path and unrelated-repository behavior fail-closed.

## Out of scope

- Persisted path normalization or migration.
- Workspace recreation or cleanup.
- Frontend or API changes.

## Acceptance

- A valid repository accessed through case-aliased paths passes admission on a
  case-insensitive filesystem.
- Missing common directories and different repositories return
  `worktree.ErrReuseWorktreeUnavailable`.
- Validation performs no Git or filesystem mutation.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(LocalPreparer_ReuseRequiredValidatesRepositoryIdentity|ValidateLaunchWorkspaceAdmission.*Repository|ValidateLocalRepositoryWorkspace.*)' -count=1)
git diff --check -- apps/backend/internal/agent/runtime/lifecycle docs/plans/case-insensitive-worktree-recovery
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local_test.go`
- `docs/plans/case-insensitive-worktree-recovery/plan.md`
- `docs/plans/case-insensitive-worktree-recovery/task-01-compare-git-directory-identity.md`

## Dependencies

None.

## Risks

- Test filesystems that are case-sensitive must skip the case-alias scenario
  without skipping the unrelated-repository safety coverage.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/additional-session-workspace-reuse.md`
- `docs/specs/tasks/system-design/additional-session-workspace-reuse.md`
- Existing repository identity validation and tests.

## Results

- RED: `TestValidateLocalRepositoryWorkspaceAcceptsCaseAliasedCommonDirectory`
  failed because the valid case-aliased path returned
  `worktree.ErrReuseWorktreeUnavailable`.
- GREEN: the same regression passed after Git common directories were compared
  with `os.Stat` and `os.SameFile`.
- The targeted identity and admission test command passed.
- `git diff --check` passed for the implementation and plan package.
