---
id: "03-cleanup-job-phase"
title: "Reclamation phase in the durable task-resource cleanup job"
status: done
wave: 2
depends_on: ["01-reclaim-probe-and-removal", "02-profile-opt-in"]
plan: "plan.md"
requirements: "../../specs/executors/requirements/remote-task-directory-reclamation.md"
acceptance_criteria:
  - AC-SSH-TASKDIR-RECLAMATION-001.1
  - AC-SSH-TASKDIR-RECLAMATION-001.3
  - AC-SSH-TASKDIR-RECLAMATION-002.7
  - AC-SSH-TASKDIR-RECLAMATION-003.1
  - AC-SSH-TASKDIR-RECLAMATION-003.2
  - AC-SSH-TASKDIR-RECLAMATION-003.3
  - AC-SSH-TASKDIR-RECLAMATION-004.1
  - AC-SSH-TASKDIR-RECLAMATION-004.2
  - AC-SSH-TASKDIR-RECLAMATION-004.3
  - AC-SSH-TASKDIR-RECLAMATION-005.1
  - AC-SSH-TASKDIR-RECLAMATION-005.2
  - AC-SSH-TASKDIR-RECLAMATION-005.3
  - AC-SSH-TASKDIR-RECLAMATION-005.4
  - AC-SSH-TASKDIR-RECLAMATION-007.1
  - AC-SSH-TASKDIR-RECLAMATION-007.2
system_design: "../../specs/executors/system-design/remote-task-directory-reclamation.md"
---

# Task 03: Reclamation phase in the durable task-resource cleanup job

## Summary

Wire task 01's reclaimer into `executeTaskResourceCleanupJob` as a task-scoped
phase that runs after the stop phase and after `performTaskCleanup`. This is the
only caller, and the point at which removal becomes reachable. It carries the
ownership resolution and the skip-versus-error semantics.

`SSHExecutor.StopInstance` is **not** modified: its existing contract of never
removing the task directory is what makes preservation on ordinary stop and on
backend shutdown structural rather than conditional.

## Scope

- New `SSHTaskDirs []sshReclaimTarget` field on `taskResourceCleanupSnapshot`,
  populated at prepare time from `ListExecutorsRunningByTaskID` and de-duplicated
  by `(host, port, user, remote_task_dir)`.
- Ownership resolution: `TaskEnvironment.TaskID` equals the job's task, plus
  `hasActiveOtherTaskSessionsForEnvironment` reporting no other live task, plus
  no other task's `executor_running` row carrying the same host tuple and
  directory. Any error resolves to not-owned.
- The phase itself: gate on the flag, resolve ownership, validate the path,
  connect, probe, remove.
- Outcome folding: a skip records and returns no error; a failure appends to the
  job's `errs` slice so the existing backoff and `last_error` handling applies.
- Structured logging with `outcome` and `reason`.

## Exclusions

- No change to `SSHExecutor.StopInstance`.
- No retroactive sweep of directories from tasks that reached a terminal outcome
  before this shipped — the phase reads the job snapshot only.
- No new table or migration; the snapshot field is additive and absent-tolerant.

## Implementation acceptance conditions

1. With reclamation enabled, a terminal archive and a terminal delete each remove
   the remote task directory exactly once for a multi-session task, and a job
   snapshot written without the new field decodes and reclaims nothing.
2. An ordinary stop and a backend shutdown leave the directory in place and
   create no reclamation attempt; an `inherit_parent` subtask reaching a terminal
   outcome leaves its parent's shared directory in place and records `shared`.
3. A reclamation failure leaves the job retrying or `failed` with a populated
   `last_error` and never `succeeded`, retries re-run the full probe rather than
   reusing a verdict, and local worktree, row, and quick-chat cleanup still
   complete; a safety skip completes the job `succeeded` with the reason recorded.

## Verification

```bash
make -C apps/backend test ARGS='-run "TestTaskResourceCleanup|TestSSHTaskDirReclaim" ./internal/task/service/... ./internal/agent/runtime/lifecycle/...'
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/resource_cleanup_ssh_test.go` (new)

## Dependencies

Tasks 01 and 02.

## Inputs

- System design sections `Control flow`, `Ownership resolution`, and
  `Failure and recovery`.
- `resource_cleanup_jobs.go:executeTaskResourceCleanupJob` for the phase
  insertion point and the existing `errs` contract.
- `service_tasks.go:cleanupDestructiveTaskResources` for the
  `filterSharedWorktreesForTaskCleanup` ownership precedent.
- `service_tasks.go:stopTaskRuntimeTargets` for the terminal-target error
  swallowing that motivates keeping removal out of the stop phase.

## Risks

The `inherit_parent` ownership case is the one that loses a live user's
workspace. It gets a direct regression test, not coverage by inference.

## Safety

Tests use a fake reclaimer and fake repositories. No test performs a real remote
removal, opens a real SSH connection, or targets any path under a live workspace
root.

## Output contract

Set `status` to `done`, tick the box in `plan.md`, and report the phase position,
the ownership rule as implemented, the skip-reason enumeration, and tests run.
