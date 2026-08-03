---
id: "03-confirmed-dead-runtime-cleanup"
title: "Prune confirmed-dead missing-session runtime rows"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/runtime-cleanup.md"
parallel-safe: false
---

# Task 03: Prune Confirmed-Dead Missing-Session Runtime Rows

## Root Cause

- `Executor.Stop` (`orchestrator/executor/executor_interaction.go:21-27`) maps
  every session-lookup error to `ErrExecutionNotFound`, and `StopExecution`
  (`:173-192`) wraps every stop error — including `lifecycle.ErrExecutionNotFound`
  — as `ErrExecutionNotFound`, so callers cannot tell "already gone" from a real
  failure.
- `handleMissingSessionOnStartup` (`orchestrator/service.go:1725-1748`) preserves
  the row on any `StopAgentWithReason` error, including a legitimate not-found for
  a dead process.
- `executeTaskResourceCleanupJob`
  (`task/service/resource_cleanup_jobs.go:396,413-419`) counts every failed stop
  (not-found included) into `failedStops`, returns an error, and the job
  re-enters `retry_wait` forever.

## Acceptance

- A not-found stop for a **confirmed-dead local** row (runtime-aware liveness ==
  Dead) is recorded as a successful stop; the row is pruned or repaired under
  `RowMustBePreserved` (token/worktree preserved when required).
- Such a row is NOT counted as a `failedStops` entry, so the durable cleanup job
  does not retry solely because the owned runtime is absent.
- **Alive** and **Unknown/remote** (SSH/containerized/no local handle) rows are
  preserved on a not-found stop and the outcome stays retryable.
- Non-not-found session/task lookup or stop errors stay retryable; not every
  session/execution not-found is blanket-ignored.
- Runtime-specific persisted handles (e.g. `agent_execution_id`) are used to
  decide the stop result where available.

## Regression Test (RED first)

- `orchestrator/reconcile_restart_test.go`: missing-session row that is
  confirmed-dead-local + not-found stop → row pruned/repaired; alive → preserved;
  unknown/remote (SSH) → preserved with handle+token; non-not-found stop error →
  preserved retryable.
- `task/service/service_tasks_stop_test.go` and
  `task/service/resource_cleanup_jobs_test.go`: confirmed-dead not-found stop is
  not a `failedStops` entry and the job completes (no retry); alive/unknown remain
  failures/preserved; terminal-session case behaves per resume-safety.

## Verification

```bash
cd apps/backend && go test -tags fts5 -run 'TestReconcile|MissingSession|StopExecution|Stop' ./internal/orchestrator ./internal/orchestrator/executor
cd apps/backend && go test -tags fts5 -run 'TaskResourceCleanup|Stop|Cleanup' ./internal/task/service
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/reconcile_restart_test.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_tasks_stop_test.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
- `apps/backend/internal/task/models/resume_safety.go` (read-only reference)

## Dependencies

None on Tasks 01/02, but internally sequential: the error-classification change
in the executor precedes the startup and worker consumers.

## Parallelism

`sequential`; spans `orchestrator`, `orchestrator/executor`, and `task/service`
and must land as one reviewable change so callers and classification stay
consistent. Not parallel-safe.

## Inputs

- Amended spec: `docs/specs/tasks/runtime-cleanup.md` (confirmed-dead
  not-found-as-stopped bullets, failure modes, and scenarios).
- Existing runtime-aware liveness: `lifecycle.RowProcessLiveness` /
  `models.ProcessLiveness` and `RowMustBePreserved`
  (`task/models/resume_safety.go`).
- Existing sentinels: `lifecycle.ErrExecutionNotFound`,
  `executor.ErrExecutionNotFound`, `runtimeapi.ErrNotFound`,
  `models.ErrTaskSessionNotFound`, `models.ErrExecutorRunningNotFound`.
- Confirmed "resumed task resource cleanup job failed: 1 runtime stop operations
  failed" in the logs (job 7e3d58bd..., task 76cbddb5...).

## Output contract

Report the RED failures for each case (dead/alive/unknown-remote/missing-session/
terminal), the classification and consumer changes, files changed, exact
verification results, evidence the cleanup job no longer retries a confirmed-dead
runtime, residual risks, and update this task plus `plan.md` status.
