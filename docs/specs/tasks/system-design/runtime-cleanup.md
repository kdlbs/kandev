---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-06-22
updated: 2026-09-09
owners:
  - cfl
---
# Task Runtime Cleanup System Design

## Purpose and boundaries

This design owns the technical contract for REQ-TASKS-RUNTIME-CLEANUP-001.

The active implementation authority is split into
[Archive Cleanup Evidence](archive-cleanup-evidence.md),
[Runtime Startup Registry](runtime-startup-registry.md), and
[Workspace Deletion Table Registry](workspace-deletion-table-registry.md).
This migrated record remains the behavioral requirement source; those records
own exact evidence, startup symbols, and table names.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| REQ-TASKS-RUNTIME-CLEANUP-001 | Migrated legacy design detail below |

## Migrated design source

Decision: [ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)

Branch retention amendment:
[ADR-2026-08-30-compact-integrated-managed-branches](../../../decisions/2026-08-30-compact-integrated-managed-branches.md)

## Why

Operators archive and delete completed tasks to keep the board usable and the
workspace tidy. Those actions must also release the task's runtime resources;
otherwise completed work leaves hidden ACP processes, host utility processes,
worktrees, or executor rows behind and the machine slowly runs out of memory.

## What

- Archive and delete durably inventory every runtime still associated with the
  task before removing live task, worktree, or runtime-tracking rows. Physical
  stop is attempted by the cleanup worker outside the deletion transaction.
- Cleanup ownership is based on `executors_running`, not only on active
  `task_sessions` state. A runtime row for a completed, cancelled, archived, or
  otherwise terminal session is still a cleanup target until the runtime stop
  has been attempted.
- Before deleting live runtime/session/environment/repository rows, cleanup
  persists an immutable `task_resource_cleanup_snapshots` record containing the
  exact handles and ownership proof. That retained record remains authoritative
  until stop/cleanup succeeds or a retryable failure is preserved.
- Worktree and task-environment cleanup is ownership-aware. A task cleanup may
  destroy only resources owned by that task's `TaskEnvironment`; borrowed or
  inherited worktrees remain owned by the source task.
- A `TaskEnvironment` referenced by another active task session is shared for the
  duration of that session. Cleanup can stop the current task's runtime rows, but
  must defer destructive worktree/container/sandbox teardown until no other
  active task session references the environment.
- A task owns its materialized workspace independently of its sessions. Deleting
  any session, including the task's last session, removes only that session and
  its workspace references. It does not remove task workspace directories, Git
  worktree registrations, or branches.
- A task with zero sessions retains its materialized workspace so a later session
  can reuse the same files and Git state.
- Physical workspace cleanup is initiated only by task lifecycle operations:
  archive, delete, cascade archive/delete, workspace delete, quick-chat expiry,
  or explicit task-environment reset. Session deletion is not a cleanup trigger.
- Agent shutdown escalates to the whole process group when its timeout expires
  and waits for descendants; neither agentctl nor backend shutdown reports
  completion while an owned process remains under PID 1.
- Standalone agentctl ignores terminal foreground interrupts so the backend owns
  Ctrl+C sequencing. Launchers gracefully terminate the backend before force
  kill, and backend shutdown includes agentctl's cleanup window.
- Startup reconciliation classifies stale runtime ownership and liveness.
  Confirmed-dead local rows clean idempotently; alive, unknown, remote, or
  generic failures remain durable. Expected fail-closed cases are aggregated in
  a bounded structured summary; unexpected failures retain individual warnings.
- Cleanup is idempotent only for classifiable absence. Persisted stop handles and
  runtime-aware liveness may complete confirmed-dead local rows; alive, unknown,
  remote, and lookup failures remain retryable.
- Archive/delete/cascade/workspace-delete/quick-chat expiry persist cleanup intent
  and snapshot before task mutation; a durable worker, never a detached goroutine,
  owns execution. A contended advisory stop guard does not block requested stop.
- `bounded_terminal` retries eight times with 1m, 5m, 15m, 1h, 3h, 6h, and 12h
  delays. Cascade-critical archive/unarchive retains attempt-eight diagnostics and
  continues every 12h under `cascade_retry`.
- Archive cleanup revalidates archived state before destruction. Unarchive
  cancels pending archive cleanup and preserves historical worktree/branch data.
- Attach-only resume requires the same executor owner and a live durable worktree
  row. Otherwise normal preparation recreates the directory/branch using the
  historical worktree ID when available, then reactivates or upserts the matching
  environment/repository/branch row.

## Archive cascade coordination

[Durable Archive Cascades](durable-archive-cascades.md) owns operation
reservation, immutable membership, workspace authorization, partial-mutation
recovery, unarchive, and parent-mutation admission. The architecture choice is
recorded in [Persist task cascade mutation progress](../../../decisions/2026-09-05-durable-task-cascade-mutations.md).

This cleanup design owns the caller-cancellation boundary after preparation.
The request computes one absolute deadline: the earlier caller deadline or entry
plus `TaskArchiveTimeout` (two minutes). Authorization, reservation, and cleanup
preparation remain caller-cancellable. After preparation, request-owned runtime
stop, mutation, finalization, publication, vacancy, activation, rollback, and
compensation use cancellation-independent contexts capped by
`min(original_deadline, now+step_budget)`. No “fresh” context extends it.

When the original deadline has expired, synchronous recovery stops after
persisting retry direction/error and waking the mandatory runtime; the response
returns the durable operation identity. A later worker claim has its own bounded
attempt deadline and is not part of the expired request attempt. Exact nested
budgets and expiry tests are in
[Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md#absolute-request-deadline).
Task/group jobs use the full execution-contract state machine.

Cleaner/restorer receives versioned database-scoped `CleanupFence` keys, exact
snapshot/lifecycle marker identity, and provider proof. Claim commits before
guard wait; no SQL transaction waits. Short marker/completion transactions
bracket I/O. Unarchive advances lifecycle generation before joining cleanup;
restore uses identical keys under that newer fence.

## Archive cleanup disposition

Direct and cascade archive share one disposition: stop task runtimes, remove the
physical worktree, mark its repository row deleted, and retain the owning
`task_environments` plus every `task_environment_repos` identity/path/branch/slug
and local branch ref. Duplicate snapshot references cannot delete a ref retained
by an earlier pass. Unarchive reuses that environment and branch; absent live
repository rows use normal preparation. Delete may remove owner rows only after
capturing the cleanup snapshot.

Managed branch compaction amends that disposition in
[Managed Branch Compaction](managed-branch-compaction.md).

## Data Model

Preparation with absent resources follows
[Cleanup preparation](runtime-cleanup-preparation.md).

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

### `task_environments` and `task_environment_repos`

`task_environments.task_id` is the canonical workspace owner.
`task_environment_repos` records the ordered repositories in that environment
and is the single source of physical worktree identity, path, branch, status, and
lifecycle timestamps. Task cleanup queries repository rows through the owning
environment's `task_id`; it never needs a session row.

Sessions reference shared workspaces through `task_environment_id`. A
transactional SQLite/PostgreSQL migration backfills `task_session_worktrees`
into `task_environment_repos`, removes redundant columns, and leaves no dual
path. It fails closed unless every legacy identity and path has one owner.
Under the migration lock it verifies shadow inventory, ownership, constraints,
row counts, and final schema before commit; failure rolls back. SQLite requires
the pre-upgrade snapshot, while PostgreSQL uses transactional DDL and its
advisory lock. Migration performs no filesystem/Git work; fresh databases use
the final schema.

### `task_resource_cleanup_jobs`

`task_resource_cleanup_jobs` is durable task-lifecycle intent. It has no foreign
key to live task, session, runtime, environment, group, or physical rows, so
delete cannot remove its immutable resource snapshot. It stores operation,
workspace, task, resource-key set, snapshot hash/payload, target/work direction,
lifecycle generation, state/retry/error, claim generation/owner/lease/heartbeat,
unknown-resolution evidence, and timestamps.

Task cleanup uses the exact task/group transition table in
[Archive Cascade Execution Contracts](archive-cascade-execution-contracts.md#task-and-group-cleanup-jobs).
Cascade archive/unarchive uses unbounded `cascade_retry`: after the normal seven
delays, failures retain an exhausted diagnostic and retry every 12 hours.
Non-restorable delete/shutdown may use `bounded_terminal` and enter retained
`failed` on attempt eight. Ambiguous I/O always enters `unknown`.

A prepared job is also the durable creation barrier. Every production writer to
session, runtime, environment, and physical-handle tables is registered as
admitted or column-scoped exempt in the aggregate writer inventory. Admitted
writes serialize against the task/operation barrier; writer-first state enters
the snapshot and reservation-first returns typed conflict. The barrier
transaction holds no filesystem, target-path, or Git lock.

## API Surface

No new action is required for the base contract. Dirty deletion admission is in
[Dirty Worktree Task Deletion](dirty-worktree-deletion.md).

`session.delete` keeps its existing request and response contract. Success means
the session row is gone. It does not mean the task workspace was cleaned, and it
never enqueues task resource cleanup.

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

`TaskExecutionStopper.RegisterExecutionStopOwner(sessionID, executionID, force)`
must not wait for the session guard; it may skip a contended claim and never
replaces `StopExecution`.

## State Machine

Runtime cleanup for a task follows this lifecycle:

- `tracked`: an `executors_running` row exists for the task.
- `stop_requested`: archive, delete, explicit stop, terminal-agent cleanup, or
  startup reconciliation selected the row.
- `snapshot_handed_off`: task/cascade/workspace deletion committed an immutable
  resource snapshot; the live row may be removed, but the retained handle owns
  cleanup.
- `stopped`: the runtime instance and subprocess group exited or were absent.
- `tracking_removed`: the live runtime row is deleted after stop, or after the
  snapshot handoff that preserves its authoritative handle.

Allowed transitions:

- `tracked` -> `stop_requested` by archive/delete/session stop/reconciliation.
- `stop_requested` -> `snapshot_handed_off` for task, cascade, or workspace
  deletion after its immutable snapshot commits, or -> `stopped` when shutdown
  succeeds/absence is proved.
- `snapshot_handed_off` -> `tracking_removed` after the live row is deleted.
- `stopped` -> `tracking_removed` after cleanup no longer needs the live handle.
- `stop_requested` -> `retryable_failure` on timeout or uncertain state.
- `retryable_failure` -> `stop_requested` on the next cleanup attempt.

The deleting transaction commits the snapshot, cleanup claim, and handoff
generation before removing the live runtime row. The cleanup worker owns the
stop from that claim; lease expiry permits a higher-generation takeover, and a
crash after commit cannot leave an unowned running runtime.
The durable task cleanup job follows the authoritative
[task/group table](archive-cascade-execution-contracts.md#task-and-group-cleanup-jobs).
It stores desired `target_direction` separately from the claimed
`work_direction`.

Unarchive sets target restore and advances lifecycle generation before joining a
running cleanup claim. Prepared/pending/unclaimed retry jobs cancel directly.
Running or unknown cleanup becomes `unknown` under the new fence; reconciliation
acquires the same physical guard and proves either no cleanup (`cancelled`) or
completed cleanup (`restore_pending`). It never labels possibly completed I/O
cancelled from lease state alone.

Restore claims move `restore_pending|restore_retry_wait` through `restoring` to
`restored`; ambiguous/expired work enters `unknown` until proof selects restored
or safe retry. Task restore validates retained environment/repository/branch
metadata and marks it safely materializable; normal admitted session preparation
recreates an absent worktree later. Archive requires every required nonmissing
cascade job `succeeded`; unarchive requires each `cancelled` or `restored`.
Concurrently deleted members instead retain `delete_owned` cleanup and are
reported missing rather than exposed.

## Failure Modes

- If querying `executors_running` for a task fails, archive/delete still updates
  the task only if existing product behavior requires it, but destructive runtime
  cleanup must fail closed: do not remove runtime rows or worktrees based on an
  empty or partial inventory.
- A contended owner claim is skipped; explicit stop continues and durable cleanup
  preserves retryable work.
- If stopping a runtime execution times out, the process manager escalates to a
  process-group kill and waits for confirmation. If confirmation still fails, the
  runtime row remains retryable.
- If a runtime row points at a missing in-memory execution, cleanup attempts the
  runtime-specific persisted handle when available. If no handle can be used, the
  row is preserved with a warning instead of being silently dropped.
- Keep `models.ErrExecutionRotated` (newer won); warn on other errors.
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
- If worktree or task-environment cleanup fails after runtime shutdown is
  confirmed, the runtime tracking row can still be removed because it no longer
  identifies a live process. The resource cleanup error remains retryable.
- An absent captured database row is an idempotent metadata-deletion success; it
  says nothing about a physical path. An absent physical worktree/environment
  path succeeds only when the authenticated external ownership marker and locked
  Git registration prove that exact snapshot was already removed. Generic
  filesystem or Git not-found is `unknown`, preserves evidence, and retries.
- Bounded-policy delete/shutdown jobs may enter retained terminal `failed` on
  claim eight, except DeleteTask Git registration/branch cleanup, which always
  uses unbounded generation-fenced `cascade_retry`. Other cascade jobs retain
  `last_error`, set `exhausted`, and schedule the next claim in 12 hours.
- If cleanup cannot prove that a session worktree belongs to the task being
  cleaned, destructive worktree deletion fails closed and skips that worktree.
  Stale Git state is removed only with pinned path, branch, and commit ownership.
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
- If startup reconciliation receives a typed runtime-not-found result for a local
  row with no persisted process liveness handle, it classifies liveness as
  Unknown and preserves the row. It summarizes that expected fail-closed outcome
  instead of emitting one ambiguous warning per legacy row.
- Startup diagnostics identify the runtime, session state, liveness class,
  presence of a local PID, stop-error class, disposition, and aggregate count.
  They never include resume tokens, credentials, or provider payloads.
- If cleanup intent or its resource snapshot cannot be persisted, the lifecycle
  mutation fails before destructive cleanup begins; Kandev does not rely on an
  unrecorded background goroutine.
- If an archived task is unarchived before cleanup completes, the worker cancels
  remaining archive cleanup. Already completed resource removal remains valid and
  the unarchive branch-recovery flow recreates the environment when possible.
- If an unarchived worktree environment has no live physical repository row,
  resume does not enter attach-only preparation. It enters normal worktree
  preparation, where unavailable branch recovery remains a typed worktree
  recovery failure rather than `ErrReuseWorktreeUnavailable`.
- If deleting a session fails, no physical workspace operation has been attempted;
  the task-owned worktree record remains authoritative regardless of whether the
  session mutation committed.
- If a session or worktree creation races task archive/delete preparation, the
  task lifecycle barrier wins before cleanup inventory is finalized. The creation
  is rejected or its uncommitted materialization is compensated, and the cleanup
  snapshot cannot omit a newly owned worktree.
- If task-owned worktree backfill encounters conflicting ownership or path data,
  startup fails closed with a diagnostic instead of choosing a row that could
  authorize deletion of another task's workspace.

## Persistence Guarantees

- Runtime cleanup intent survives backend restarts because live
  `executors_running` rows remain until stop is attempted; after live-row
  deletion, `task_resource_cleanup_snapshots` retains the exact handle,
  ownership, and proof envelope until cleanup succeeds or retryable failure is
  recorded.
- Runtime stop is represented in the retained snapshot before live worktree or
  task-environment rows are removed; the cleanup worker attempts the stop and
  later physical teardown from that snapshot.
- Startup reconciliation reloads retained snapshots and stale runtime rows.
  A typed-not-found, confirmed-dead local row is removed only after its
  snapshot records the result; unknown liveness remains durable.
- Pending and retryable task cleanup jobs survive restart and resume independently
  of whether optional scheduled storage maintenance is enabled.
- Terminal `failed` bounded-policy jobs survive restart for diagnosis and do not
  resume automatically; cascade-policy jobs never enter that state.
- Cleanup snapshots needed after task deletion survive without foreign-keyed task,
  session, environment, or worktree rows.
- A `task_environment_repos` row, its directory, Git registration, branch, and
  uncommitted files survive deletion of every session. Task lifecycle deletion
  snapshots those identities before removing live rows; the retained snapshot
  remains discoverable for cleanup and proof.
- Storage inventory protects paths from task environment repository rows, not
  only paths referenced by live sessions, so a zero-session task workspace is
  not classified as orphaned.
- Historical worktree rows for archived tasks remain available to unarchive branch
  recovery even after their on-disk directories are removed.
- Archive cleanup preserves the task environment owner and exact branch recovery
  state. Cleanup retries keep the same disposition.
- Orphaned OS processes without any durable `executors_running` row are outside
  normal cleanup guarantees; they may be handled by an explicit operator recovery
  tool, but automatic task cleanup must not rely on process-name scanning.

## Scenarios

- **GIVEN** a task with one stopped session, a registered worktree, and
  uncommitted files, **WHEN** the session is deleted, **THEN** the task remains
  with zero sessions and the directory, Git registration, branch, and files are
  unchanged.
- **GIVEN** a task whose last session was deleted, **WHEN** the backend restarts
  and a new session is created, **THEN** the session reuses the task-owned
  workspace and observes the preserved files.
- **GIVEN** two sessions referencing the same task worktree, **WHEN** either
  session is deleted, **THEN** the remaining session continues using the same
  directory and no filesystem or Git cleanup command runs.
- **GIVEN** a session has been deleted and the backend restarted, **WHEN** its
  task is archived, **THEN** durable asynchronous task cleanup still discovers
  the task-owned worktree by `task_id` and removes it unless another task holds
  a protected shared reference.
- **GIVEN** a session has been deleted and the backend restarted, **WHEN** its
  task is deleted, **THEN** the cleanup job snapshot retains the worktree handles
  after task-row cascades and retries filesystem or Git failures after restart.
- **GIVEN** task cleanup preparation is racing a new session/worktree launch,
  **WHEN** the lifecycle barrier is reserved, **THEN** no resource created after
  the barrier can be omitted from the cleanup inventory or survive as an
  untracked directory.

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
- **GIVEN** a missing, terminal, or failed session whose local runtime is already
  absent and whose persisted PID is confirmed dead, **WHEN** startup
  reconciliation receives a typed not-found result, **THEN** it safely removes or
  repairs the row without an individual warning, and a second reconciliation has
  no stale row to process.
- **GIVEN** a typed not-found result for an alive local row, a local row without a
  liveness handle, or a remote row, **WHEN** startup reconciliation runs, **THEN**
  it preserves the row and includes the safe disposition in a bounded startup
  summary.
- **GIVEN** a stop fails with a generic error, **WHEN** startup reconciliation
  runs, **THEN** it preserves the row and emits an individual structured warning
  rather than reclassifying the runtime as absent.
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
- **WHEN** compare-and-set returns `models.ErrExecutionRotated`, **THEN** keep
  the newer row silently.
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
- **GIVEN** a delete cleanup snapshot references a physical worktree that is
  already absent, **WHEN** the cleanup worker runs, **THEN** it succeeds only
  after the authenticated external marker and locked Git registration prove
  exact prior removal; generic path not-found is `unknown` and retries.
- **GIVEN** a delete cleanup snapshot requests deletion of a captured
  task-environment database row that is already absent, **WHEN** the cleanup
  worker runs, **THEN** the metadata step is idempotently complete and does not
  retry because of that absence.
- **GIVEN** a cleanup job fails for a genuinely retryable reason, **WHEN** fewer
  than eight attempts have run, **THEN** it enters `retry_wait` using the
  documented backoff schedule.
- **GIVEN** a bounded-policy cleanup job fails on attempt eight, **WHEN** the
  worker records it, **THEN** it enters retained terminal `failed`.
- **GIVEN** a cascade-policy job fails on attempt eight or later, **WHEN** the
  worker records it, **THEN** it retains the exhausted diagnostic and schedules
  another generation-fenced attempt after 12 hours.
- **GIVEN** an archive cleanup job is pending, **WHEN** the task is unarchived,
  **THEN** the job is cancelled and cannot delete resources created after
  unarchive.
- **GIVEN** archive cleanup removed a local worktree, **WHEN** the task is
  unarchived, **THEN** its historical worktree branch metadata remains available
  for local/remote recovery.
- **GIVEN** a direct or cascade archive removes a task worktree, **WHEN** the
  remote branch is absent, **THEN** exact local recovery state remains available.
- **GIVEN** an unarchived task environment whose worktree repository row is
  deleted, failed, or tombstoned, **WHEN** its session resumes, **THEN** the
  executor selects normal worktree preparation and recreates or reactivates the
  recoverable task-owned worktree instead of demanding attach-only reuse.

## Out of Scope

- A general-purpose OS process sweeper that kills every process named
  `codex-acp`, `claude-acp`, or `opencode`.
- UI changes for showing runtime cleanup failures.
- Per-session warnings about uncommitted or unpushed workspace state; ordinary
  session deletion preserves that state. Destructive warnings remain attached
  to task archive/delete/reset surfaces.
- A user-facing action to replay a terminal failed cleanup job.
- New user-facing archive/delete controls.
- Changing the task/session state model beyond the cleanup guarantees described
  here.

## Implementation plan

- [Backend failure containment](../../../plans/backend-failure-containment/plan.md)
- [Worktree resume after unarchive](../../../plans/worktree-resume-after-unarchive/plan.md)
- [Archive resume identity](../../../plans/archive-resume-identity/plan.md)
- [Compact integrated managed branches](../../../plans/compact-integrated-managed-branches/plan.md)
