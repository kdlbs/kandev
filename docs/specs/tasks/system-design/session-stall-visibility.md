---
status: draft
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
   outside that snapshot need lifecycle classification. A missing execution
   alone does not prove a stall.
   For `WAITING_FOR_INPUT`, read the active turn through `GetActiveTurn`.
   A nil turn identifies an idle conversation and excludes it from detection
   and healing. A read error skips the task for this tick.
   `IDLE` remains outside the active-session query.
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
6. A recoverable set covering every active session rechecks liveness and
   settles abandoned work to waiting state under the
   [interruption recovery design](restart-orphaned-session-terminalization.md).
   Neither this pass nor the restart pass writes `CANCELLED` for execution loss.

Keep every active session in the task-wide eligibility denominator. An idle
sibling blocks this task-wide action. The restart pass remains per-session.
Carry the observed activity and turn identity through the conditional recovery
write. A completed or replaced turn rejects a stale recovery snapshot.

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
- Recovery write failure: retried with the archived pass's bounded
  in-line retry, and the next tick retries again.
- A live execution appearing between classification and the recovery
  boundary aborts the transition; the ID-scoped predicate structurally excludes
  sessions that became active after classification (AC .4).

## Configuration

`tasks.stallDetectionThreshold` / `KANDEV_TASK_STALL_DETECTION_THRESHOLD`
(default `2h`), a startup setting in the typed config catalog
(`internal/common/config/`). Invalid YAML durations fail parsing; zero or
negative YAML durations are rejected at startup; invalid or non-positive
environment values fall back to `2h`.

## Recovery when opening a task

Extend `GetTaskSessionStatus` in `internal/orchestrator/task_operations.go` with
an exact orphan-reason exception for `CANCELLED`. Use
`models.SessionOrphanedCancelReason`; do not classify arbitrary cancellation
messages as recoverable. Keep archive cancellation reasons distinct.

Use the existing archive-recovery branch as the implementation pattern:

- With a supported resume token, validate the profile and return
  `is_resumable=true`, `needs_resume=true`, and a distinct orphan recovery reason.
- Without a usable token, use existing same-session initialization eligibility.
  Preserve stored messages and workspace identity. Do not claim provider context
  was restored when the provider cannot resume it.
- Missing profiles and unavailable prerequisites retain actionable recovery
  errors. Do not mark an impossible launch as automatic recovery.
- Existing live executions return the running status without a launch request.

The frontend continues through `use-session-resumption-operations.ts` and
`session.launch` with `intent=resume`. Opening sends no prompt and never invokes
the new-session action. Keep the automatic admission flag, existing request
deduplication, archive fences, preference gate, and deferred-owner checks.
On successful startup, clear the current cancellation error through the normal
session transition. Preserve historical diagnostic events.

Use existing silent workspace restoration and explicit retry after a failed
resume. A failed attempt must not remount into an endless automatic retry.
No schema migration or live-data rewrite is required: old orphan reasons are
recognized on read.

## Desktop and phone behavior

Use the existing task conversation recovery surface. Desktop retains its chat
panel. Phone retains `session-mobile-layout.tsx`, its session picker, one chat
scroll owner, fixed navigation, and safe-area handling. No new dialog or banner
is introduced. Both surfaces show ordinary startup progress followed by the
same transcript and composer. Existing recovery controls remain on actual failure.

The [implementation plan](../../../plans/orphaned-session-open-recovery/plan.md)
contains the state preview and desktop/phone regression matrix. Reuse translated
copy. Any required new copy must enter all five locale catalogs.

## Compatibility and decision

This draft corrects the classification shipped in PR #3832. The new recovery
exception also covers cancellations from PR #3833. Keep both reconciliation
passes, but replace their orphan cancellation with recoverable interruption
settlement. Archive and explicit-stop cancellation remain unchanged.
The [session-open decision](../../../decisions/2026-09-18-session-open-resumes-conversation.md)
already separates provider recovery from prompt dispatch and workflow ownership.
This local extension needs no additional architecture decision.

## Requirement mapping

| Requirement | Satisfied by |
| --- | --- |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .1, .2) | Control flow steps 4-5; Episode dedupe |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .3) | Control flow step 6 |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .4) | Recovery boundary re-check; conditional session transition |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .5) | Control flow step 3 |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .6) | Failure and recovery: no registry |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .7) | Lifecycle classification and recovery boundary |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .8, .9) | Recovery when opening a task |
| REQ-TASKS-SESSION-STALL-VISIBILITY-001 (AC .10) | Desktop and phone behavior |
