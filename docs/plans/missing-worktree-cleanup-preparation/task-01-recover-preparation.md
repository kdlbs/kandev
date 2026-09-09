---
id: "01-recover-preparation"
title: "Recover cleanup preparation for absent worktrees"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.6
  - AC-TASKS-RUNTIME-CLEANUP-001.7
  - AC-TASKS-RUNTIME-CLEANUP-001.9
  - AC-TASKS-RUNTIME-CLEANUP-001.12
  - AC-TASKS-RUNTIME-CLEANUP-001.13
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
---

# Task 01: Recover cleanup preparation for absent worktrees

## Summary

Preparation accepts confirmed absence without losing the durable worktree
snapshot. Existing cleanup audits still protect resources that remain or reappear.

## In scope

- Classify physical paths before identity capture.
- Capture surviving branch commits through a bounded exact-ref inspection.
- Omit an identity only after a successful absence result.
- Add manager and lifecycle regressions before the production correction.
- Run the targeted backend and existing browser checks.

## Out of scope

- SQL changes, new cleanup snapshot fields, and historical-job repair.
- Changes to worker ownership rules or general `branchExists` behavior.
- Production frontend changes or new browser fixture infrastructure.

## Acceptance

1. Absent worktree directories and branches permit direct and cascade
   archive/delete. A healthy sibling repository retains its snapshot identity.
2. Preparation preserves inventory rows and snapshots until normal lifecycle
   cleanup. Broken refs, filesystem errors, Git failures, and cancellation
   cannot establish absence.
3. The worker completes fully absent resources and retains archive recovery
   metadata. Existing dirty, shared, unique-commit, and replacement safeguards pass.

## Implementation sequence

1. Mark this work order `in_progress`.
2. Add the manager and lifecycle regressions in the new test files.
3. Run the regressions against unchanged production code.
4. Record the missing-branch preparation failure.
5. Correct `CaptureCleanupHeadOIDs` with a small private helper where needed.
6. Run the commands in Verification.
7. Record results in this file and the plan.
8. Mark this work order `done` after all required checks pass.

## Regression matrix

`TestCaptureCleanupHeadOIDs_MissingWorktree` covers:

- A healthy checkout, an absent checkout with a surviving branch, and a fully
  absent checkout/branch/registration.
- A mixed batch with an absent resource first and last.
- Exact branch names versus descendant refs, packed refs, and an absent branch
  that has a tag with the same short name.
- Retained worktree records, unchanged files/refs, and the expected identity map.

`TestCaptureCleanupHeadOIDs_FailsClosed` covers:

- An inaccessible or invalid parent repository and an unavailable Git command.
- A broken ref, a ref pointing at a missing object, and unexpected diagnostic output.
- Context cancellation and a bounded inspection timeout.
- A regular file or symbolic link at the recorded checkout path.
- Filesystem errors other than absence. Probe permission enforcement before
  relying on mode bits in a privileged test environment.

`TestPrepareTaskResourceCleanup_MissingWorktree` uses `setupOfficeTest`,
`newCleanupTestWorktreeManager`, and real Git fixtures. Keep the worker stopped
during snapshot assertions. The fixture creates two repository rows in one
environment and leaves both active after external removal of one checkout.
Use `git worktree remove` and branch removal only inside disposable test roots.

The test covers both cascade triggers and inspects the durable snapshot. Both
worktree entries remain present, and the healthy repository retains its commit.
The absent branch contributes no invented commit. Preparation leaves the task
unmodified and the job prepared until the lifecycle mutation commits.

`TestTaskLifecycleCleanup_MissingWorktree` covers direct `ArchiveTask` and
`DeleteTask`, plus `ArchiveTaskTree` and `DeleteTaskTree`. Use the real
`HandoffService` wiring from the existing cascade tests.

Run the durable worker and verify the terminal result. Archive preserves the
environment, repository history, and healthy branch. Delete completes cleanup
after task-row removal. Fully absent resources must not cause `retry_wait`.
A healthy sibling still receives its normal cleanup disposition.

Add a race scenario that creates a replacement branch after preparation.
Deletion must preserve the new branch without an immutable captured identity.
Retain the existing stale-registration refusal and branch-preserving archive behavior.

## Verification

From `apps/backend`, run the new regressions before the correction:

```bash
rtk go test -tags fts5 ./internal/worktree -run '^TestCaptureCleanupHeadOIDs_' -count=1 -v
rtk go test -tags fts5 ./internal/task/service -run '^(TestPrepareTaskResourceCleanup_MissingWorktree|TestTaskLifecycleCleanup_MissingWorktree)$' -count=1 -v
```

After the correction, run the focused checks from `apps/backend`:

```bash
rtk go test -race -tags fts5 ./internal/worktree -run '^(TestCaptureCleanupHeadOIDs_|TestCleanupWorktrees)' -count=1
rtk go test -race -tags fts5 ./internal/task/service -run '^(TestPrepareTaskResourceCleanup_MissingWorktree|TestTaskLifecycleCleanup_MissingWorktree|TestPreparedCascadeCleanupSnapshotPersistsWorktreeTaskDirNames|TestArchiveAndDeleteCleanupRemainPreparedUntilMutationCommits|TestTaskResourceCleanupMissingResourcesSucceed|TestUnarchiveCancelsAndJoinsClaimedArchiveCleanup)$' -count=1
```

Before the first package command in a fresh worktree, run from `apps`:

```bash
rtk pnpm install --frozen-lockfile
```

Run the existing browser flows sequentially from `apps/web`:

```bash
rtk pnpm e2e:run --project chromium tests/kanban/card-menu-delete-archive.spec.ts
rtk pnpm e2e:run --project mobile-chrome tests/task/mobile-archive-task-redirect.spec.ts
```

The managed runner rebuilds production artifacts and cleans its isolated
runtime. Record discovered test counts and results. Do not overlap suites or
add worker overrides.

From the repository root:

```bash
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_capture.go` (new)
- `apps/backend/internal/worktree/manager_cleanup_capture_test.go` (new)
- `apps/backend/internal/task/service/resource_cleanup_missing_worktree_test.go` (new)
- `docs/plans/missing-worktree-cleanup-preparation/plan.md`
- This work order.

## Dependencies

None. Implement in the primary conversation after an explicit implementation request.

## Risks

- The runner combines stderr and stdout. Warning text must fail structured parsing.
- Git ref patterns also match descendants. Only an exact ref supplies identity.
- Early row deletion can lose archive recovery data and bypass worker audits.
- Test fixtures must create valid environment paths and link sessions to their environment.
- Existing large test files must not grow beyond repository limits.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/runtime-cleanup.md), criteria .6, .7, .9, .12, and .13.
- [Preparation design](../../specs/tasks/system-design/runtime-cleanup-preparation.md).
- [Runtime cleanup design](../../specs/tasks/system-design/runtime-cleanup.md).
- `manager_cleanup_audit.go`: `cleanupPathPresent` and `cleanupBranchIdentity`.
- `manager_cleanup_recovery_test.go`: missing paths, changed identities, and unique work.
- `resource_cleanup_jobs.go`: preparation, snapshot persistence, and activation.
- `resource_cleanup_jobs_test.go`: snapshot and cascade fixtures.
- `resource_cleanup_task_owned_test.go`: real repository and worktree-manager fixtures.
- [Git ref enumeration](https://git-scm.com/docs/git-for-each-ref).

## Results

Implemented and verified. `CaptureCleanupHeadOIDs` now classifies the recorded
path with `Lstat`, uses bounded exact-ref enumeration for an absent directory,
and omits only a confirmed-absent local branch identity. Strict parsing rejects
diagnostics, malformed records, missing objects, invalid commit IDs, and
non-commit refs. The full worktree inventory remains in the durable snapshot.

Validation completed:

- `go test -tags fts5` manager capture regressions pass.
- `go test -tags fts5` preparation and direct/cascade lifecycle regressions pass,
  including replacement-branch retry safety.
- The required worktree race matrix passes in 34 seconds.
- The required service race matrix passes in 13 seconds.
- Chromium card-menu archive/delete flow passes 5 tests.
- Mobile-Chromium archive redirect flow passes 1 test.
- `python3 scripts/lint-spec-files.py --all`, `git diff --check`, and changed-code
  `golangci-lint` pass; the latter reports 0 issues.
