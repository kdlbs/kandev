---
created: 2026-09-09
status: done
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
legacy_specs: []
---

# Implementation Plan: Missing Worktree Cleanup Preparation

## Overview

[Issue #3531](https://github.com/kdlbs/kandev/issues/3531) reports blocked task
archive and deletion after external worktree removal. The issue is assigned to
`carlosflorencio`.

One sequential work order corrects preparation and proves the complete task
lifecycle. The implementation and validation are complete. The investigation used revision
`ecd3efc01b819615e1eb3219d774dfb0e3a44cfd`.

The [preparation design](../../specs/tasks/system-design/runtime-cleanup-preparation.md)
defines the missing-resource boundary. The task system owns cleanup preparation and
the durable resource snapshot. Workspace ownership and deletion safeguards
remain governed by the existing design and ADRs.

## Confirmed root cause

`CaptureCleanupHeadOIDs` already checks the worktree path with `os.Stat`.
For an absent directory, it resolves `refs/heads/<branch>^{commit}` from the
parent repository. If the branch is also absent, Git exits 128.
`captureWorktreeCleanupHeadOIDs` propagates the error through
`PrepareTaskResourceCleanupWithOptions` before task mutation.

Preparation reserves a `prepared` cleanup job before identity capture. The
capture error prevents replacement of its placeholder snapshot. This explains
the reported `prepared` jobs with zero worker attempts.

The current failure does not require Git to run inside the missing directory.
The fallback branch lookup alone reproduces the reported error.

## Reproduction evidence

A temporary test used the real task service, SQLite repository, worktree
manager, and disposable Git repositories. It created an active worktree row,
removed the checkout through Git, and retained the database inventory.

Command, from `apps/backend`:

```bash
rtk go test -tags fts5 ./internal/task/service -run '^TestRepro3531_PrepareMissingWorktree$' -count=1 -v
```

- `branch_present` passed preparation after directory removal.
- `branch_absent` failed after removal of the local branch as well.
- The failure contained `capture worktree cleanup identities` and `exit status 128`.
- A second diagnostic proved that `CleanupWorktrees` already accepts the fully
  absent resource without a captured commit identity.

A separate Git probe established these results on Git 2.47.3:

| Probe result | Exit | Combined output |
| --- | --- | --- |
| Exact branch present | 0 | Ref name, object ID, and `commit` |
| Exact branch absent | 0 | Empty |
| Broken ref | 0 | Warning text |
| Ref points to missing object | 128 | Fatal diagnostic |

The probe used `for-each-ref` with
`--format=%(refname)%00%(objectname)%00%(objecttype)` and the full branch ref.
Its four diagnostic cases passed. A quiet `show-ref --verify` probe returned
the same exit code for a missing ref and a broken ref, so it is unsuitable here.

Temporary tests and their fixture data were removed after diagnosis. No user
instance or database was modified. The permanent regression must first fail
against unchanged production code during implementation.

## Scope

### In scope

- Preparation for absent worktrees with either surviving or absent branches.
- Complete snapshots for tasks with healthy and absent repository worktrees.
- Direct and cascade archive/delete through the real services and cleanup worker.
- Error classification and existing ownership, branch, and path safeguards.

### Out of scope

- Recreating missing worktrees or recovering externally deleted commits.
- Automatic database repair, schema changes, or removal of historical jobs.
- Relaxing cleanup audits or treating generic Git errors as absence.
- New UI controls, translations, runtime flags, or executor behavior.

## Technical approach

The correction belongs in
`apps/backend/internal/worktree/manager_cleanup.go:CaptureCleanupHeadOIDs`,
with strict ref and object-ID parsing in the adjacent
`manager_cleanup_capture.go` helper file.
Use the existing `cleanupPathPresent` classification for physical paths.
Keep existing handling for incomplete legacy inventory metadata.

For absent paths with a recorded branch, use a bounded exact-ref probe through
`runBoundedGitInspect`. The structured `for-each-ref` form above supports old
Git versions without the newer `show-ref --exists` option.
Git patterns can include descendant refs. Compare the returned full ref name
exactly, as specified in the [Git documentation](https://git-scm.com/docs/git-for-each-ref).

Parse only complete records with a nonempty object ID and object type `commit`.
Reject unexpected output, including warnings on stderr combined by the runner.
A successful response without the exact ref establishes absence.
An exact branch result supplies its immutable commit identity.
An absent branch omits only its identity-map entry and emits a bounded warning
with the task/worktree IDs and the absence reason.

Do not use `Manager.branchExists` to establish absence in preparation.
That helper currently maps generic command failures to `false` outside timeout
handling. Changing its wider behavior is outside this repair.

Keep the full `Worktrees` snapshot and the existing service error propagation.
`manager_cleanup_audit.go` remains the authority for destructive worker actions.
No early row deletion, broad Git prune, or new cleanup status is required.

Related decisions:

- [Fail-closed cleanup](../../decisions/0009-fail-closed-gc-semantics.md).
- [Task-owned worktree lifetime](../../decisions/2026-08-08-task-owned-worktree-lifetime.md).

This repair applies those boundaries without a new architectural decision.

## Tests

| Acceptance criteria | Permanent evidence |
| --- | --- |
| .9, .13 | `manager_cleanup_capture_test.go`: `TestCaptureCleanupHeadOIDs_MissingWorktree` |
| .9, .12 | `manager_cleanup_capture_test.go`: `TestCaptureCleanupHeadOIDs_FailsClosed` |
| .6, .13 | `resource_cleanup_missing_worktree_test.go`: `TestPrepareTaskResourceCleanup_MissingWorktree` |
| .6, .7, .9, .13 | Same file: `TestTaskLifecycleCleanup_MissingWorktree` |
| .9, .13 | Same file: `TestTaskLifecycleCleanup_MissingWorktree_ReplacementBranchStaysRetryable` |

All criterion suffixes refer to `AC-TASKS-RUNTIME-CLEANUP-001`.
The work order defines the case matrix, worker assertions, and exact commands.
New test files avoid the existing oversized `resource_cleanup_jobs_test.go`.

## E2E tests

The backend integration matrix reproduces the missing-resource defect through
real lifecycle services and durable cleanup. Existing browser flows provide
end-to-end evidence for unchanged controls:

- `tests/kanban/card-menu-delete-archive.spec.ts`, project `chromium`: archive
  and delete remove the card. Maps to .13 with the backend matrix.
- `tests/task/mobile-archive-task-redirect.spec.ts`, project `mobile-chrome`:
  cascade archive preserves usable navigation. Maps to .13 with the backend matrix.

These browser tests are smoke coverage, not independent reproductions of the
missing-worktree state. No new layout or touch behavior requires a mobile design.

## Work orders

- [done] [Task 01: Recover cleanup preparation for absent worktrees](task-01-recover-preparation.md)

## Verification results

Diagnosis: real service reproduction failed as expected, with the surviving-branch
control passing. Existing execution accepted the fully absent resource.
The structured-ref diagnostic passed all four cases.

Design validation passed: `lint-spec-files.py --all`, all 30 specification-linter
tests, and `git diff --check`.

Implementation and its regression/E2E checks are complete. The manager capture
matrix and lifecycle regressions pass in normal and race-enabled runs. The
existing browser smoke flows pass with 5 Chromium tests and 1 mobile-Chromium
test. The changed-code Go lint reports 0 issues; the specification linter and
diff check also pass.

## Documentation impact

Internal requirements and design describe the preparation contract explicitly.
Public docs need no change during this design turn. The repair adds no operator
action or public interface, and restores the existing cleanup intent.

## Risks

- Treating a warning or generic Git failure as absence can hide damaged metadata.
- Dropping the worktree snapshot entry can lose registration and row cleanup.
- A directory or branch can reappear between preparation and execution.
  Existing worker identity audits must remain active.
- Stale registrations without verifiable identity remain retryable by design.
- Historical prepared-job reconciliation remains unchanged.
- The WSL host was not reproduced. The same failure reproduced on Linux through
  the platform-independent Go/Git path.
