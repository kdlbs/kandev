---
id: "03-task-owned-durable-cleanup"
title: "Clean task-owned worktrees durably"
status: done
wave: 3
depends_on: ["02-lifecycle-creation-barrier"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/session-delete-resource-cleanup.md"
decision: "../../decisions/2026-08-08-task-owned-worktree-lifetime.md"
---

# Task 03: Clean Task-Owned Worktrees Durably

## Acceptance

- Archive, delete, cascade archive/delete, workspace delete, quick-chat expiry,
  and explicit environment reset inventory `task_environment_repos` through the
  owning environment's task ID.
- Task cleanup still finds a worktree after its last session was deleted and the
  backend restarted.
- The durable cleanup snapshot survives deletion of task and session rows.
- Archive marks the task archived and schedules asynchronous cleanup; delete
  removes the task and schedules asynchronous cleanup.
- Filesystem and Git worktree failures enter the existing bounded retry schedule
  and resume after restart.
- Cleanup preserves resources borrowed or actively referenced by other tasks or
  sessions.
- TaskEnvironment ownership transfer changes its owner once; canonical
  repository worktrees follow without a second ownership write.
- `CleanupWorktrees`/`removeWorktree` have task-lifecycle callers only; no
  `ReclaimSessionWorktree` or `session_delete` trigger/job exists.
- Storage inventory, automation retention, and orphan GC use canonical live
  owner records rather than session-derived rows.

## TDD Sequence

1. RED: delete the only session, recreate the service/repository, then
   archive/delete the task and prove inventory still contains the worktree.
2. RED: add archive and delete integration tests that observe asynchronous
   cleanup, Git registration removal, branch policy, and task-row outcomes.
3. RED: inject a transient filesystem/Git failure, recreate the worker, advance
   the retry time, and prove the durable snapshot succeeds on a later claim.
4. RED: cover a worktree borrowed by another active task/session and an ownership
   transfer; prove cleanup skips or transfers it rather than deleting it.
5. GREEN: change task inventory and cleanup to canonical environment repository
   rows and snapshot those handles before lifecycle mutation.
6. GREEN: migrate storage inventory, automation retention, and orphan-GC reads.
7. REFACTOR: remove session-derived physical cleanup APIs and assert the allowed
   caller boundary in focused tests.

## Verification

```bash
cd apps/backend && go test ./internal/task/service/... -run 'Archive|Delete|Cleanup|Cascade|Worktree'
cd apps/backend && go test ./internal/worktree/... -run 'Cleanup|Shared|Task'
cd apps/backend && go test ./internal/backendapp/... ./internal/automation/... ./internal/office/infra/...
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/service_cleanup_inventory_test.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/backendapp/storage_inventory.go`
- `apps/backend/internal/automation/run_retention.go`
- `apps/backend/internal/office/infra/gc.go`

## Dependencies

- Task 02 establishes the lifecycle creation barrier around inventory capture.

## Inputs

- Existing durable cleanup worker, retry schedule, archive revalidation, and
  shared-environment session checks.
- Existing task/cascade/workspace/quick-chat lifecycle entry points.

## Output contract

Report every task-lifecycle caller audited, the canonical inventory query, the
restart/retry evidence, shared-resource outcomes, task-row outcomes for archive
and delete, Git registration/branch results, and proof that no session-delete
physical cleanup entry point remains.
