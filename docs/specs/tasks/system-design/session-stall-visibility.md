---
status: current
system: tasks
created: 2026-09-20
owners:
  - kandev
requirements:
  - REQ-TASKS-SESSION-STALL-VISIBILITY-001
---

# Session Stall Visibility and Orphan Healing Design

## Context and boundaries

The [session reconciliation sweep](#control-flow) in
`internal/task/service/` is the single owner of this behavior. It already
reconciles archived tasks' sessions
(`archived_session_reconciliation.go`); the active-task pass
(`active_session_stall.go`) extends the same one-minute tick to unarchived
tasks holding active sessions. The agent runtime's in-memory execution store
is the liveness oracle, injected as `SessionExecutionRegistry`
(`service.go`); the task system never guesses liveness from DB state alone.

Archived tasks remain owned by the archived pass. Sessions with a live
execution remain owned by the agent runtime's own stall watchdog; the sweep
never reports them.

## Control flow

Each tick:

1. `ListUnarchivedTasksWithActiveSessions` returns candidate tasks.
2. Per task, `ListActiveTaskSessionsByTaskID` loads the active sessions and
   the registry reports live ones (`LiveSessionIDsForTask`); sessions
   outside that snapshot are orphaned.
3. Each orphaned session's activity clock is the newer of its row's
   `updated_at` and its newest `task_session_messages` row
   (`GetLastMessageTimeBySessionIDs`). A read failure skips the task this
   tick entirely (AC .5).
4. Silence past `tasks.stallDetectionThreshold` (default 2h; healed grace is
   twice that) classifies the session as stalled; silence past the grace
   window also classifies it as healable.
5. Stalled sessions not yet reported in this episode publish one
   `task.stalled` event; delivery is recorded only after the publish
   succeeds, so a failed publish retries next tick (AC .1).
6. A healable set covering every active session of the task re-checks
   liveness at the cancellation boundary and then cancels exactly the
   classified session IDs through `finalizeCancelledSessionIDs`, reusing the
   archived pass's transition: `CANCELLED` with `SessionOrphanedCancelReason`
   (`"orphaned session"`), clarification expiry, parked-projection cleanup,
   session-ceiling release, and one `session.state_changed` per session
   (AC .3, .4).

## Session activity clock

Turn persistence (`CreateTurn`, `CreateTurnWithStepStamp`) refreshes the
parent `task_sessions.updated_at` in the same statement batch, so an
auto-dispatched turn with no user message still resets the silence clock.
Message writes are already covered because the clock reads the newest
message directly.

## Episode dedupe

`stallNotifiedSessions` maps task ID to the sessions reported in the open
episode. Entries are pruned per session when it leaves the stalled set, so
a recovered session that stalls again reports again (AC .2). The map lives
on the sweep's single goroutine and resets when no candidates remain.

## Failure and recovery behavior

- No registry wired: the pass skips entirely (AC .6).
- Activity read failure: the task is skipped this tick (AC .5).
- Cancellation write failure: retried with the archived pass's bounded
  in-line retry, and the next tick retries again.
- A live execution appearing between classification and the cancellation
  boundary aborts the heal; the ID-scoped predicate structurally excludes
  sessions that became active after classification (AC .4).

## Configuration

`tasks.stallDetectionThreshold` / `KANDEV_TASK_STALL_DETECTION_THRESHOLD`
(default `2h`), a startup setting in the typed config catalog
(`internal/common/config/`). Invalid YAML durations fail parsing; zero or
negative YAML durations are rejected at startup; invalid or non-positive
environment values fall back to `2h`.

## Requirement mapping

| Requirement | Satisfied by |
| --- | --- |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .1, .2) | Control flow steps 4-5; Episode dedupe |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .3) | Control flow step 6 |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .4) | Cancellation boundary re-check; ID-scoped cancel |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .5) | Control flow step 3 |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .6) | Failure and recovery: no registry |
