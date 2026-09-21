---
status: current
system: tasks
requirements:
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-001
---

# Restart-Orphaned Session Terminalization System Design

## Purpose and boundaries

The tasks system owns the task-session lifecycle, including the transition of
a session into a terminal state. This design covers the periodic sweep that
terminalizes `STARTING`/`RUNNING` sessions of unarchived tasks that no live
in-memory agent execution backs — the residue of a backend process that died
mid-turn.

It extends the existing reconciliation loop
(`internal/task/service/archived_session_reconciliation.go`) with a second
pass; it does not introduce a new loop, new persistence, or adapter changes.
Adjacent contracts this design uses but does not own: the agent runtime's
in-memory execution store (executors system), session-ceiling reservations
(tasks system, reservation contract), and the archived-task reconciliation
pass that owns archived tasks' stranded sessions.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-RESTART-ORPHAN-SESSIONS-001` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- **Reconciliation loop** (`Service.StartArchivedSessionReconciliationLoop`):
  ticks every minute and runs both passes — the archived pass first, then the
  orphan pass.
- **Orphan pass** (`Service.runOrphanedSessionReconciliation`): lists stale
  candidates, filters by live-execution absence, cancels each, runs
  post-cancellation effects.
- **Session repository** (`sqlite.Repository`): candidate query
  `ListStaleRunningSessionsOnUnarchivedTasks` and per-session cancel
  `CancelRunningTaskSessionByID`. Not part of the `SessionRepository`
  interface; reached through the narrow `orphanedSessionRepository`
  capability so test doubles stay unchanged.
- **Execution-liveness seam** (`TaskExecutionLivenessChecker`): reports
  whether a session has a live in-memory execution. Satisfied by the
  lifecycle adapter (`HasLiveExecution`), which answers from the in-memory
  store only and never lazily creates an execution.
- **Shared post-cancellation effects** (`Service.notifyCancelledSessions`):
  extracted from the archive cancellation path and reused verbatim:
  clarification expiry, parked-projection clearing, ceiling release, and the
  `session.state_changed` publish.

## Data and contracts

- Candidate query joins `task_sessions` to `tasks` and selects rows where
  `ts.state IN ('STARTING','RUNNING')`, `ts.updated_at < staleBefore`, and
  `t.archived_at IS NULL`, ordered by `updated_at` ascending.
- `staleBefore` is `now - (constants.AgentLaunchTimeout + 5m grace
  allowance)`: one full launch budget plus the start-deadline allowance, read
  (not copied) from `constants.AgentLaunchTimeout` so an operator-raised
  preparation budget is honored. The same cutoff is passed into the cancel
  statement and re-asserted as `updated_at < staleBefore` there, so a row
  refreshed between the candidate read and the write no longer matches.
- `CancelRunningTaskSessionByID(sessionID, reason, staleBefore)` is an
  `UPDATE ... RETURNING` over rows still in `STARTING`/`RUNNING` whose
  `updated_at` still predates the cutoff; it returns `nil` when the session
  raced to another state or was refreshed, and the write detaches from the
  caller context with a 10-second timeout. The RETURNING clause selects
  `is_primary` alongside the other event-payload fields, so the published
  cancellation event carries the durable primary flag.
- Cancellation reason constant: `models.SessionOrphanedCancelReason`
  (`"orphaned by backend restart"`), distinct from the archive reasons;
  `IsArchiveCancelReason` returns false for it.

## Control flow

1. Every tick, pass 2 runs after the archived pass.
2. If no execution-liveness checker is wired, the pass is inert: absence
   from the store is the only dead signal, and a nil checker can never prove
   a session unbacked.
3. List candidates with the stale-before cutoff computed from the launch
   budget.
4. For each candidate, re-check `HasLiveExecution(session.ID)` at the moment
   of the write — a launch that raced the grace window since the read must
   not be reaped.
5. Cancel via `CancelRunningTaskSessionByID`; the statement re-asserts both
   the state set (`STARTING`/`RUNNING`) and the staleness cutoff
   (`updated_at < staleBefore`), so a row refreshed by an in-flight launch
   between the liveness check and this write matches nothing; on `nil`
   (raced or refreshed) skip; on error warn and let the next tick retry.
6. Run the shared post-cancellation effects with the orphan reason.

## Failure and recovery

- A failed candidate read logs an error and returns; the next tick retries.
- A failed cancel logs a warn and continues to the next candidate; the next
   tick retries that session. The sweep never gives up for as long as the
  process runs.
- The cancel write is context-detached (`context.WithoutCancel` + 10s bound),
  so the terminal transition survives a client disconnect and cannot block a
  sweep pass indefinitely on a locked writer.
- Pass 2 is a no-op on clean trees: fresh rows are filtered by the grace
  window, live-backed rows by the liveness check, archived tasks' rows by the
  archived-pass filter, and terminal rows by the candidate query.

## Persistence

No schema change. The sweep writes only `task_sessions.state`,
`error_message`, `completed_at`, and `updated_at`. Survivors re-tracked by
startup recovery are in the in-memory store and stay untouched, so the sweep
composes with agent-survival re-tracking rather than racing it.

## Security

No new trust boundary. The sweep runs in-process with the task service's own
credentials and touches only `task_sessions` rows of unarchived tasks.

## Observability

- `task-session reconciliation loop started (every 1 minute)` at loop start.
- `orphaned-session reconciliation: found candidates` / `terminalized
  unbacked session` (Info, with task and session IDs, previous state).
- `failed to list candidates` / `failed to cancel session` (Error/Warn with
  the underlying error).

## Related decisions

- Issue #3711 and its approved fix plan: extend the existing sweep rather
  than drain-on-shutdown, turn-state persistence, heartbeats, or manual SQL.
