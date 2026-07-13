---
status: draft
created: 2026-06-22
owner: cfl
---

# Task Runtime Cleanup

## Why

Operators archive and delete completed tasks to keep the board usable and the
workspace tidy. Those actions must also release the task's runtime resources;
otherwise completed work leaves hidden ACP processes, host utility processes,
worktrees, or executor rows behind and the machine slowly runs out of memory.

## What

- Archiving a task stops every runtime execution that is still durably associated
  with that task before the task's worktrees or runtime tracking rows are removed.
- Deleting a task stops every runtime execution that is still durably associated
  with that task before task records, worktrees, or runtime tracking rows are
  removed.
- Cleanup ownership is based on `executors_running`, not only on active
  `task_sessions` state. A runtime row for a completed, cancelled, archived, or
  otherwise terminal session is still a cleanup target until the runtime stop has
  been attempted.
- A cleanup path MUST NOT delete the last durable runtime handle for a task until
  either the matching runtime execution has stopped successfully or the failure is
  preserved for retry/diagnosis.
- Worktree and task-environment cleanup is ownership-aware. A task cleanup may
  destroy only resources owned by that task's `TaskEnvironment`; borrowed or
  inherited worktrees remain owned by the source task.
- A `TaskEnvironment` referenced by another active task session is shared for the
  duration of that session. Cleanup can stop the current task's runtime rows, but
  must defer destructive worktree/container/sandbox teardown until no other
  active task session references the environment.
- Agent subprocess shutdown kills the whole agent process group when graceful
  shutdown does not finish within the configured stop timeout.
- Agent subprocess shutdown does not treat the command leader exiting as
  sufficient while descendants remain alive in the same process group.
- Agentctl instance shutdown never leaves an agent subprocess group alive as a
  child of `init`/PID 1.
- Standalone agentctl is isolated from terminal foreground interrupts so the
  backend remains the owner of Ctrl+C shutdown sequencing.
- Top-level launchers, including `make start-debug` through `kandev start`, send
  the backend a graceful termination signal before any force kill so backend
  shutdown can stop agents and agentctl instances.
- Backend shutdown waits long enough for standalone agentctl's instance cleanup
  window before reporting shutdown complete.
- Backend startup reconciles stale runtime rows for archived/deleted/missing task
  state and attempts safe cleanup instead of treating the rows as live sessions.
- Cleanup is idempotent: repeating archive/delete cleanup, startup reconciliation,
  or explicit session stop does not fail because the process, task, session, or
  worktree was already removed.

## Data Model

### `executors_running`

`executors_running` remains the durable runtime ownership table and the source of
cleanup handles for task runtime teardown.

- `session_id` identifies the task session that originally launched the runtime.
- `task_id` identifies the owning task and is the primary lookup key for archive
  and delete cleanup.
- `agent_execution_id` is the preferred stop handle for in-memory runtime
  shutdown through the lifecycle manager.
- `runtime`, `container_id`, `agentctl_url`, `agentctl_port`, `pid`, and
  `metadata` provide fallback handles for runtime-specific cleanup and
  diagnostics.
- `status`, `error_message`, and timestamps remain available for cleanup
  diagnostics when stop fails and the row must remain retryable.

Rows may temporarily reference archived tasks or terminal sessions while cleanup
is in progress. Rows must not reference missing task/session state indefinitely.

### `task_sessions`

`task_sessions.state` remains the user-facing session state. Terminal states do
not imply runtime resources have been released. Cleanup code must not use terminal
session state as a reason to skip runtime teardown when an `executors_running`
row exists.

## API Surface

No new user-facing HTTP or WebSocket action is required. Existing task archive,
task delete, session stop, and backend startup behavior gain stronger cleanup
guarantees.

Internal contracts:

```go
type ExecutorRunningRepository interface {
    ListExecutorsRunningByTaskID(ctx context.Context, taskID string) ([]*models.ExecutorRunning, error)
    ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
}
```

`TaskExecutionStopper.StopExecution(ctx, executionID, reason, force)` remains the
primary stop operation when `agent_execution_id` is available. Fallback cleanup is
runtime-specific and must be bounded by context.

## State Machine

Runtime cleanup for a task follows this lifecycle:

- `tracked`: an `executors_running` row exists for the task.
- `stop_requested`: archive, delete, explicit stop, terminal-agent cleanup, or
  startup reconciliation has selected the row for cleanup.
- `stopped`: the runtime instance and its subprocess group have exited or were
  already absent.
- `tracking_removed`: the `executors_running` row is deleted after the stop
  result is known.
- `retryable_failure`: stop could not be confirmed before the timeout. The row
  remains durable with enough context to retry and diagnose.

Allowed transitions:

- `tracked` -> `stop_requested` by archive/delete/session stop/reconciliation.
- `stop_requested` -> `stopped` when runtime shutdown succeeds or the runtime is
  confirmed absent.
- `stopped` -> `tracking_removed` after worktree/environment cleanup has been
  attempted and the runtime row is no longer needed as the durable stop handle.
- `stop_requested` -> `retryable_failure` on timeout or uncertain runtime state.
- `retryable_failure` -> `stop_requested` on the next cleanup attempt.

## Failure Modes

- If querying `executors_running` for a task fails, archive/delete still updates
  the task only if existing product behavior requires it, but destructive runtime
  cleanup must fail closed: do not remove runtime rows or worktrees based on an
  empty or partial inventory.
- If stopping a runtime execution times out, the process manager escalates to a
  process-group kill and waits for confirmation. If confirmation still fails, the
  runtime row remains retryable.
- If a runtime row points at a missing in-memory execution, cleanup attempts the
  runtime-specific persisted handle when available. If no handle can be used, the
  row is preserved with a warning instead of being silently dropped.
- If worktree or task environment cleanup fails after runtime shutdown is
  confirmed, the runtime tracking row can still be removed because it no longer
  identifies a live process. The resource cleanup error is logged and handled by
  the resource-specific retry path.
- If cleanup cannot prove that a session worktree belongs to the task being
  cleaned, destructive worktree deletion fails closed and skips that worktree.
- If an agentctl process exits unexpectedly, its owned agent subprocess group is
  killed before agentctl shutdown completes.
- If the user sends Ctrl+C to a standalone Kandev process tree, agentctl does
  not receive the terminal interrupt directly; it is stopped through backend
  lifecycle shutdown, parent liveness, or an explicit backend signal.
- If the user sends Ctrl+C while running through `make start-debug`, the launcher
  forwards a graceful stop to the backend and waits before escalating, rather
  than immediately killing the backend process group.
- If startup reconciliation finds rows for archived tasks, deleted tasks, missing
  sessions, or terminal sessions with no live runtime, it removes only rows that
  are positively confirmed safe to remove.

## Persistence Guarantees

- Runtime cleanup intent survives backend restarts because `executors_running`
  rows remain durable until cleanup succeeds or a retryable failure is recorded.
- Worktrees and task environment rows are not removed before runtime stop has been
  attempted for every runtime row owned by the task.
- Startup reconciliation is allowed to recover from a previous backend crash by
  reattempting cleanup for stale runtime rows.
- Orphaned OS processes without any durable `executors_running` row are outside
  normal cleanup guarantees; they may be handled by an explicit operator recovery
  tool, but automatic task cleanup must not rely on process-name scanning.

## Scenarios

- **GIVEN** a task has a `WAITING_FOR_INPUT` session and an
  `executors_running` row, **WHEN** the task is archived, **THEN** the runtime is
  stopped by `agent_execution_id` before the row and worktrees are removed.
- **GIVEN** a task has a `COMPLETED` session but still has an
  `executors_running` row, **WHEN** the task is deleted, **THEN** cleanup still
  selects that row and attempts runtime shutdown.
- **GIVEN** runtime shutdown succeeds for every row owned by a deleted task,
  **WHEN** cleanup finishes, **THEN** the task's `executors_running` rows and
  worktrees are removed.
- **GIVEN** runtime shutdown times out for one row owned by an archived task,
  **WHEN** cleanup finishes, **THEN** the row remains retryable with a diagnostic
  error and worktree deletion does not erase the only runtime handle.
- **GIVEN** the backend restarts with an `executors_running` row for an archived
  task, **WHEN** startup reconciliation runs, **THEN** it attempts cleanup for the
  row instead of treating the archived task as active.
- **GIVEN** agentctl is stopped while an ACP child process ignores stdin EOF,
  **WHEN** the stop timeout expires, **THEN** the ACP process group is killed and
  no ACP child is reparented to PID 1.
- **GIVEN** an ACP wrapper process exits after spawning a native child in the
  same process group, **WHEN** agentctl completes shutdown, **THEN** the
  remaining process-group descendants are terminated before shutdown is reported
  complete.
- **GIVEN** standalone Kandev owns an active agentctl instance, **WHEN** the user
  stops Kandev with Ctrl+C, **THEN** backend shutdown supervises agentctl
  cleanup and waits for the instance stop window instead of letting agentctl exit
  directly from the terminal interrupt.
- **GIVEN** Kandev is running under `make start-debug`, **WHEN** the user stops it
  with Ctrl+C, **THEN** the launcher gives the backend a graceful shutdown window
  before any force kill so active ACP process groups are reaped.
- **GIVEN** the `executors_running` query fails during archive cleanup, **WHEN**
  cleanup evaluates destructive actions, **THEN** it logs the failure and does not
  delete runtime tracking rows based on incomplete information.
- **GIVEN** a child task reuses its parent's `TaskEnvironment`, **WHEN** the child
  is archived or deleted, **THEN** cleanup stops the child's runtime rows without
  deleting the inherited parent worktree.
- **GIVEN** a parent task owns a `TaskEnvironment` that an active child session
  still references, **WHEN** parent cleanup runs, **THEN** destructive
  environment and worktree teardown is deferred until the child no longer holds
  the environment.

## Out of Scope

- A general-purpose OS process sweeper that kills every process named
  `codex-acp`, `claude-acp`, or `opencode`.
- UI changes for showing runtime cleanup failures.
- New user-facing archive/delete controls.
- Changing the task/session state model beyond the cleanup guarantees described
  here.
