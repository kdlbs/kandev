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
- A cleanup operation treats a classifiable missing owned resource as already
  complete. This includes a missing worktree and a missing `TaskEnvironment`
  row captured by the durable cleanup snapshot.
- Stopping a runtime that is already gone is a successful, idempotent stop, not a
  failed stop. When a stop operation is asked to stop an execution or session
  that no longer exists, and the row it owns is a confirmed-dead local runtime,
  the cleanup treats the runtime as already stopped so the runtime row can be
  pruned or repaired under the resume-safety invariant and the durable cleanup
  job does not retry solely because the owned runtime is absent.
- "Confirmed dead" is judged in a runtime-aware way. A row is confirmed dead only
  when it is a local runtime whose local liveness handle refers to a process that
  no longer exists on this host. An alive local runtime, or a runtime whose
  liveness is Unknown (remote SSH, containerized, or a local row with no liveness
  handle), is never treated as dead on the basis of a not-found stop result; its
  row is preserved and the outcome remains retryable.
- Missing-runtime handling never blanket-ignores every session-level or
  execution-level not-found error. Only a not-found result for a runtime whose row
  is confirmed dead is reclassified as a successful stop. A not-found result for
  an alive or unknown/remote row, or a session/task lookup error that is not a
  typed not-found sentinel, remains a preserved, retryable outcome so a transient
  store error is never mistaken for an absent runtime.
- Where a runtime-specific persisted stop handle exists (for example
  `agent_execution_id`, or a remote/container handle), missing-runtime handling
  uses it to decide the stop result rather than inferring absence from a generic
  error.
- Archive, delete, cascade, workspace-delete, and quick-chat expiration persist a
  cleanup intent and resource snapshot before mutating or deleting task state.
  Cleanup is performed by a durable worker rather than a detached goroutine.
- A durable cleanup job makes at most eight attempts. Failed attempts back off
  for 1 minute, 5 minutes, 15 minutes, 1 hour, 3 hours, 6 hours, and 12 hours
  before the next claim. An eighth failed attempt becomes terminal `failed`
  instead of scheduling another retry.
- Archive cleanup revalidates that the task remains archived before every
  destructive step. Unarchiving a task cancels its pending archive cleanup so a
  delayed retry cannot remove the newly active task's resources.
- Cleanup preserves historical archived-task worktree records and branch metadata
  used by unarchive recovery. Filesystem removal does not imply recovery-history
  removal.

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

### `task_resource_cleanup_jobs`

`task_resource_cleanup_jobs` is the durable task-lifecycle cleanup intent. It has
no foreign key to `tasks`, so delete cleanup survives deletion of the owning row.
It stores the trigger, state, retry timing, last error, and a JSON snapshot of the
runtime, environment, worktree, and path handles captured before task mutation.
Only one non-terminal row exists for an operation ID; repeated event delivery
reuses the same cleanup job. `attempts` counts successful worker claims. A
terminal `failed` row retains its final error and completion timestamp for
diagnosis but is not selected by the automatic worker.

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

The durable cleanup job wraps that resource lifecycle:

- `pending` -> `running` when the cleanup worker claims the job.
- `running` -> `succeeded` when runtime and owned resource cleanup finish.
- `running` -> `retry_wait` when bounded cleanup fails and the resource snapshot
  must be retried and fewer than eight claims have run.
- `running` -> `failed` when the eighth claimed attempt fails.
- `retry_wait` -> `running` on the next scheduled retry or manual storage run.
- `pending|running|retry_wait` -> `cancelled` when an archive-triggered cleanup
  observes that its task has been unarchived.

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
- If a stop operation reports the execution or session is not found and the owned
  row is a confirmed-dead local runtime, cleanup records the stop as successful,
  prunes or repairs the row under the resume-safety invariant, proceeds with any
  now-safe shared-environment/worktree teardown, and does not count the row as a
  failed stop. The durable cleanup job therefore does not re-enter `retry_wait`
  solely because that owned runtime was confirmed absent.
- If a stop operation reports not found but the owned row is alive, or its
  liveness is Unknown (remote/containerized/no local handle), cleanup preserves
  the row and treats the outcome as retryable rather than pruning it.
- If a session or task lookup during stop fails with an error that is not a typed
  not-found sentinel, cleanup treats it as a retryable failure and preserves the
  row; it does not reinterpret the error as an absent runtime.
- If worktree or task environment cleanup fails after runtime shutdown is
  confirmed, the runtime tracking row can still be removed because it no longer
  identifies a live process. The resource cleanup error is logged and handled by
  the resource-specific retry path.
- If a worktree or captured task-environment row is already absent, cleanup
  treats that deletion step as successful and continues. Other teardown errors
  remain retryable and are not hidden by an accompanying not-found result.
- If cleanup still fails on its eighth worker claim, the job becomes terminal
  `failed`, preserves `last_error`, clears `next_attempt_at`, stamps
  `completed_at`, and is excluded from automatic due-job selection.
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
- If cleanup intent or its resource snapshot cannot be persisted, the lifecycle
  mutation fails before destructive cleanup begins; Kandev does not rely on an
  unrecorded background goroutine.
- If an archived task is unarchived before cleanup completes, the worker cancels
  remaining archive cleanup. Already completed resource removal remains valid and
  the unarchive branch-recovery flow recreates the environment when possible.

## Persistence Guarantees

- Runtime cleanup intent survives backend restarts because `executors_running`
  rows remain durable until cleanup succeeds or a retryable failure is recorded.
- Worktrees and task environment rows are not removed before runtime stop has been
  attempted for every runtime row owned by the task.
- Startup reconciliation is allowed to recover from a previous backend crash by
  reattempting cleanup for stale runtime rows.
- Pending and retryable task cleanup jobs survive restart and resume independently
  of whether optional scheduled storage maintenance is enabled.
- Terminal failed task cleanup jobs survive restart for diagnosis but do not
  resume automatically.
- Cleanup snapshots needed after task deletion survive without foreign-keyed task,
  session, environment, or worktree rows.
- Historical worktree rows for archived tasks remain available to unarchive branch
  recovery even after their on-disk directories are removed.
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
- **GIVEN** an `executors_running` row whose task session is missing and whose
  local liveness handle refers to a dead host process, **WHEN** startup
  reconciliation stops it and the stop reports the execution is not found, **THEN**
  the row is pruned or repaired under the resume-safety invariant instead of being
  preserved indefinitely.
- **GIVEN** a resumed durable cleanup job whose only remaining runtime target is a
  confirmed-dead local runtime, **WHEN** the cleanup worker stops it and the stop
  reports not found, **THEN** the job records a successful stop, completes owned
  resource teardown, and does not re-enter `retry_wait` because of that runtime.
- **GIVEN** an `executors_running` row that is still alive on this host, **WHEN**
  a stop reports not found for it, **THEN** the row is preserved and the outcome is
  treated as retryable rather than pruned.
- **GIVEN** an `executors_running` row for a remote (SSH) or containerized runtime
  whose local liveness is Unknown, **WHEN** a stop reports not found, **THEN** the
  row is preserved with its remote handle and resume token intact and is not
  pruned on the basis of a host-local not-found.
- **GIVEN** a session lookup during a stop fails with a non-not-found store error,
  **WHEN** cleanup evaluates the result, **THEN** it preserves the row and retries
  rather than treating the runtime as absent.
- **GIVEN** a confirmed-dead local row that still carries a resume token, **WHEN**
  a not-found stop is reclassified as successful, **THEN** the row is repaired in
  place (token and worktree preserved) rather than deleted, per
  `RowMustBePreserved`.
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
- **GIVEN** the backend exits after a task is deleted but before its worktree is
  removed, **WHEN** the backend restarts, **THEN** the durable cleanup job retries
  using its captured resource snapshot.
- **GIVEN** a delete cleanup snapshot references a worktree that is already
  absent, **WHEN** the cleanup worker runs, **THEN** the worktree step is treated
  as complete and the job does not retry because of that absence.
- **GIVEN** a delete cleanup snapshot requests deletion of a task-environment row
  that is already absent, **WHEN** the cleanup worker runs, **THEN** the row step
  is treated as complete and the job does not retry because of that absence.
- **GIVEN** a cleanup job fails for a genuinely retryable reason, **WHEN** fewer
  than eight attempts have run, **THEN** it enters `retry_wait` using the
  documented backoff schedule.
- **GIVEN** a cleanup job fails on its eighth attempt, **WHEN** the worker records
  the result, **THEN** it enters terminal `failed` with the final diagnostic and
  is not automatically claimed again.
- **GIVEN** an archive cleanup job is pending, **WHEN** the task is unarchived,
  **THEN** the job is cancelled and cannot delete resources created after
  unarchive.
- **GIVEN** archive cleanup removed a local worktree, **WHEN** the task is
  unarchived, **THEN** its historical worktree branch metadata remains available
  for local/remote recovery.

## Out of Scope

- A general-purpose OS process sweeper that kills every process named
  `codex-acp`, `claude-acp`, or `opencode`.
- UI changes for showing runtime cleanup failures.
- A user-facing action to replay a terminal failed cleanup job.
- New user-facing archive/delete controls.
- Changing the task/session state model beyond the cleanup guarantees described
  here.
