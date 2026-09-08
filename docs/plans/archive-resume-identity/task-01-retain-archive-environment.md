---
id: "01-retain-archive-environment"
title: "Retain the archive environment owner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.6
  - AC-TASKS-RUNTIME-CLEANUP-001.7
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 01: Retain the Archive Environment Owner

## Summary

Make direct archive preserve the task environment row, matching cascade
archive. Prove the result with a durable cleanup job that can finish
successfully.

## In scope

- Add a failing direct-archive regression with a production-like environment
  destroyer.
- Change direct archive to store `DeleteEnvironmentRow=false`.
- Assert that the cleanup job succeeds.
- Assert that `task_environments` and `task_environment_repos` remain.
- Keep the worktree repository row soft-deleted with its identity fields.
- Keep cascade archive and delete tests green.

## Out of scope

- Branch-ref retention. Task 02 owns that policy.
- Resume routing changes.
- Schema or frontend changes.

## Acceptance

- Direct archive cleanup does not delete the owning environment row.
- Its repository rows retain worktree ID, path, branch, and branch slug.
- The durable cleanup job reaches `succeeded` with the destroyer wired.
- Direct and cascade archive have the same environment-row disposition.
- Delete cleanup keeps its current behavior.

## TDD sequence

1. Add `TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity` in a dedicated
   service test file.
2. Wire a real SQLite repository and a production-like environment destroyer.
3. Confirm the test fails because the environment row is absent.
4. Change the direct archive cleanup snapshot.
5. Confirm the focused and related cleanup tests pass.

## Verification

```bash
# From apps/backend:
rtk go test ./internal/task/service -run '^TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity$' -count=1
rtk go test ./internal/task/service -run 'TestArchive(TaskCleanupPreservesTaskEnvironmentIdentity|CleanupPreservesHistoricalWorktreeBranchMetadata|TaskTree_InvokesResourceCleanerPerTask)$' -count=1
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/resource_cleanup_archive_identity_test.go`

## Dependencies

None.

## Risks

- The test must inspect the cleanup job state. Row survival alone can mean that
  cleanup failed before it reached the bad deletion.
- Keep the archived worktree row as historical inventory. Do not mark it live
  while its directory is absent.

## Parallelism

Sequential.

## Inputs

- `REQ-TASKS-RUNTIME-CLEANUP-001`, especially AC `.6` and `.7`.
- `docs/specs/tasks/system-design/runtime-cleanup.md`, section
  `Archive cleanup disposition`.
- Direct archive in `service_tasks.go` and cascade archive in
  `handoff_cascade.go`.
- Existing durable cleanup tests in `resource_cleanup_jobs_test.go`.

## Results

Implemented direct-archive environment retention and added a production-wired
SQLite/worktree regression. The regression failed on the previous behavior
because the successful cleanup removed the environment row, then passed after
`ArchiveTask` persisted `DeleteEnvironmentRow=false`.

Verification passed:

```text
rtk go test ./internal/task/service -run '^TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity$' -count=1
rtk go test ./internal/task/service -run 'TestArchive(TaskCleanupPreservesTaskEnvironmentIdentity|CleanupPreservesHistoricalWorktreeBranchMetadata|TaskTree_InvokesResourceCleanerPerTask)$' -count=1
```
