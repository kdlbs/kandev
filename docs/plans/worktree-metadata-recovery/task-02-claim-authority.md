---
id: "02-claim-authority"
title: "Claim recovery authority"
status: in_progress
wave: 2
depends_on:
  - "01-scope-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.4
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.5
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.4
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
---

# Task 02: Claim recovery authority

## Summary

Make automatic replacement exclusive across the physical environment lifecycle.
Retain the contributor's snapshot protocol and add durable authority to its publication.

## In scope

- A replayable SQLite/PostgreSQL claim migration and repository contract.
- Atomic busy detection across same-task and borrowed sessions, including idle
  workspace runtimes and the requesting session's runtime.
- Claim checks before runtime startup, workspace restoration, session attachment,
  cleanup, reset, and ownership transfer.
- Exact operation and generation checks for slot publication and claim release.
- Complete inventory preflight, recorded-branch validation, per-slot publication,
  crash continuation, and partial-failure preservation.

## Out of scope

- Automatic termination of an existing runtime.
- Automatic cleanup of snapshots or original checkouts.
- An alternate remote-executor recovery implementation.

## Acceptance

1. Real database tests prove exclusion against runtime startup, attachment,
   cleanup, and owner transfer. Separate connections cannot acquire conflicting authority.
2. Recovery preserves each repository slot and all original content. A lost
   recorded branch, stale owner, changed snapshot, or absent production claim
   capability cannot publish a replacement.
3. Restart and injected-failure tests prove exact claim adoption and release.
   A completed slot survives a later failure, and no partial inventory starts an agent.

## Verification

From the repository root, with an isolated PostgreSQL test DSN configured:

```bash
(cd apps/backend && go test ./internal/worktree ./internal/task/repository/sqlite ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && go test -race ./internal/worktree ./internal/task/repository/sqlite -run 'Recovery|TaskEnvironment|Detached' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -v ./internal/task/repository/sqlite -run 'TestPostgres.*Recovery' -count=1)
git diff --check
```

Use real temporary Git repositories and fault-injection seams. Use channels for
race schedules instead of sleeps. Add `TestPostgresWorktreeRecoveryClaim` with
independent connections, following `newTaskPostgresRepoPair`.

A missing DSN or skipped PostgreSQL scenario leaves this work order incomplete.
Do not record a skipped suite as successful concurrency evidence.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_recovery.go` (new)
- `apps/backend/internal/task/repository/sqlite/task_environment_recovery_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/` schema and replayable migrations
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/task_cleanup_barrier.go`
- `apps/backend/internal/orchestrator/executor/` launch and resume claim ownership
- `apps/backend/internal/agent/runtime/lifecycle/` runtime-start and workspace-restore boundaries
- `apps/backend/internal/worktree/manager.go`
- `apps/backend/internal/worktree/recovery.go`
- `apps/backend/internal/worktree/recovery_review_test.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/worktree/store_test.go`

## Dependencies

Task 01. This work order changes a persistence boundary and requires the complete
design context, not only the worktree helper.

## Risks

- A runtime can start before its durable running row exists. Guard the external
  startup reservation, not just insertion into `executors_running`.
- Recovery must not use a cleanup job that a worker can interpret as deletion authority.
- A cancellation must not release a claim while external work still continues.
- A per-slot compare-and-swap alone cannot prevent an ownership-transfer race.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/worktree-metadata-recovery.md), requirements 002 and 003.
- [Design](../../specs/tasks/system-design/worktree-metadata-recovery.md), Recovery authority through Restart and failure.
- Existing reset claims and detached-environment generation tests as transaction patterns.
- Existing `recovery_review_test.go` and `recovery_rematerialization_test.go` as file-preservation patterns.

## Results

Implemented the replayable environment claim, claim-aware repository and
worktree mutations, lifecycle claim propagation, recorded-branch validation,
and guarded publication. The full SQLite repository package passed, including
replay, competing claims, session attachment, runtime startup, cleanup, ownership
transfer, exact guarded mutation, and stale release checks. The PostgreSQL
independent-connection test is present but skipped because
`KANDEV_TEST_POSTGRES_DSN` is not configured. This work order remains in progress
until that concurrency evidence runs.
