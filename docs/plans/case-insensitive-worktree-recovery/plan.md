---
created: 2026-09-21
status: implemented
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Implementation Plan: Case-Insensitive Worktree Recovery

## Overview

Restore session recovery for valid worktrees whose persisted repository path
uses different letter casing from the path Git reports on a case-insensitive
filesystem. Add the failing regression first, replace lexical Git-common-dir
comparison with filesystem identity comparison, and retain fail-closed behavior
for missing or unrelated repositories.

## Confirmed root cause

`validateLocalRepositoryWorkspace` resolves the Git common directory for the
workspace and source repository, then compares the resulting path strings.
On case-insensitive macOS volumes, Git and persisted configuration can spell the
same directory with different casing. `filepath.EvalSymlinks` preserves that
spelling, so the valid worktree is rejected even though `os.SameFile` confirms
that both paths identify the same directory.

## Scope

### In scope

- Compare existing Git common directories by filesystem identity.
- Fail closed when either common directory cannot be inspected.
- Preserve rejection of unrelated repositories.
- Add a case-alias regression that fails before the correction on a
  case-insensitive filesystem.

### Out of scope

- Rewriting persisted repository paths or database rows.
- Recreating, moving, cleaning, or otherwise changing worktrees.
- Changing repository discovery or path-casing policy.
- UI or API contract changes.

## Technical approach

Update `validateLocalRepositoryWorkspace` in
`apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go` to inspect
both resolved Git common directories with `os.Stat` and compare their identities
with `os.SameFile`. A stat error or identity mismatch returns the existing
`worktree.ErrReuseWorktreeUnavailable` sentinel.

Add the regression beside the existing identity tests in
`apps/backend/internal/agent/runtime/lifecycle/env_preparer_local_test.go`.
Create one repository under a mixed-case directory, access it through a
case-aliased path, skip only when the test filesystem is case-sensitive, and
prove the validator accepts the alias. Existing tests continue to prove an
unrelated repository is rejected.

## Tests

- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1`: a valid retained
  worktree remains recoverable when path spelling differs only by case.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.2`: repository admission
  uses filesystem identity for an existing expected checkout.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.3`: missing and unrelated
  common directories remain rejected.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.4`: validation remains
  read-only and does not alter either checkout.

## Work orders

- [x] [Task 01: Compare Git directories by filesystem identity](task-01-compare-git-directory-identity.md)

## Verification results

- RED: the case-alias regression reproduced
  `worktree.ErrReuseWorktreeUnavailable` for one physical repository.
- GREEN: the focused lifecycle identity and admission tests passed after
  filesystem-identity comparison replaced lexical comparison.
- Plan/specification validation and `git diff --check` passed.

## Risks

- Converting a stat failure into equality would weaken fail-closed admission;
  the implementation must return the existing typed error instead.
- The case-alias regression is meaningful only on case-insensitive filesystems,
  so unrelated-repository coverage must remain the cross-platform safety gate.
