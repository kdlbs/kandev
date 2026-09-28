---
id: coordinator-wake-design
title: Wake on its tasks' events design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-WAKE-001
  - REQ-COORDINATOR-WAKE-002
  - REQ-COORDINATOR-WAKE-003
  - REQ-COORDINATOR-WAKE-004
  - REQ-COORDINATOR-WAKE-005
  - REQ-COORDINATOR-WAKE-006
---

# Wake on its tasks' events System Design

## Purpose and boundaries

This design turns episodes on a coordinator's own tasks into durable wake
rows, and pending wake rows into at most one unattended turn at a time in the
coordinator's existing conversation. It owns the wake store, the recorder, the
backstop, admission and delivery, the unattended turn record, and the autonomy
read route. [Containment](containment.md) and [spend](spend.md) supply two
admission checks. The task system keeps owning `task.stalled`, session state,
pending actions and errors; this design reads them and adds no task field.

It does not use the automation runner, the Office wakeup dispatcher or the
shared `runs` queue. Those serve a different principal and bring the
replacement fork of analysis finding 5.2 (`prepareAutomationTask` creating a
replacement when the continuation session is merely busy). The coordinator has
exactly one conversation, and the rules below never create another.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-WAKE-001` | [Store](#store), [Recorder](#recorder) |
| `REQ-COORDINATOR-WAKE-002` | [Backstop](#backstop) |
| `REQ-COORDINATOR-WAKE-003` | [Admission](#admission), [Delivery](#delivery) |
| `REQ-COORDINATOR-WAKE-004` | [Flag and settings](#flag-and-settings), [Admission](#admission) |
| `REQ-COORDINATOR-WAKE-005` | [Delivery](#delivery), [Turn end](#turn-end), [Transcript](#transcript) |
| `REQ-COORDINATOR-WAKE-006` | [Autonomy read](#autonomy-read), [Screens](#screens) |

## Flag and settings

- `features.coordinatorPhase3` (environment
  `KANDEV_FEATURES_COORDINATOR_PHASE3`) is registered in
  `internal/runtimeflags/registry.go` with `RestartRequired: true` and in root
  `profiles.yaml` as `prod: "false"`, `dev: "false"`, `e2e: "true"`, per the
  `/runtime-feature-flags` checklist. Phase 3 is **effective** only when both
  it and `features.coordinator` are on; `internal/backendapp/coordinator.go`
  computes that once at startup and builds none of this design's subscribers,
  tickers or routes otherwise, so their routes return 404.
- The `coordinators` table gains, by additive `ALTER`:

  | Column | Type | Notes |
  | --- | --- | --- |
  | `autonomy_enabled` | integer not null default 0 | 0 or 1 |
  | `cost_ceiling_subcents` | bigint null | see [spend](spend.md#ceiling) |

- The coordinator PATCH body gains `autonomy_enabled` (boolean) and
  `cost_ceiling_usd` (string decimal or JSON `null` to clear), accepted only
  when phase 3 is effective and ignored as unknown fields otherwise. Neither
  field changes `config_revision` or clears `conversation_task_id`. The PATCH
  transaction validates them against the row it read: turning autonomy on with
  no ceiling in the resulting row, or clearing the ceiling while the resulting
  row has autonomy on, is 400 naming `cost_ceiling`.
- When a PATCH turns autonomy off, its transaction first takes the
  coordinator's [wake lock](#wake-lock), then writes the row and runs
  `UPDATE coordinator_wakes SET status='superseded', updated_at=? WHERE
  coordinator_id=? AND status='pending'`. An open unattended turn is not
  touched.
- Every coordinator GET and list response carries both fields
  (`cost_ceiling_usd` as a decimal string with two places, or `null`).

## Store

Both tables live in `internal/coordinator/store.go`'s schema, created with
`CREATE TABLE IF NOT EXISTS` on both dialects and covered by the store's
upgrade conformance test.

`coordinator_wakes`:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID |
| `coordinator_id` | text not null | |
| `workspace_id` | text not null | |
| `task_id` | text not null | the own task |
| `kind` | text not null | `question`, `permission`, `stall`, `error`, `completed` |
| `episode_key` | text not null | per the requirement's episode table; stall keys are RFC 3339 UTC with nanoseconds |
| `status` | text not null | `pending`, `delivered`, `superseded` |
| `turn_id` | text null | the `coordinator_unattended_turns.id` it was delivered in |
| `created_at`, `updated_at` | timestamp not null | UTC |

Unique index `(coordinator_id, task_id, kind, episode_key)`; index
`(coordinator_id, status, created_at, id)`.

`coordinator_unattended_turns`:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID; the "run" id improvements cite |
| `coordinator_id` | text not null | |
| `conversation_task_id` | text not null | |
| `session_id` | text not null | |
| `message_id` | text null | set after the turn's message is stored |
| `session_turn_id` | text null | the task session turn (`task_session_turns.id`) the message started; set with `message_id` |
| `wake_count` | integer not null | |
| `denied_permissions` | integer not null default 0 | [containment](containment.md#unattended-permissions) |
| `start_ceiling_subcents` | bigint not null | the coordinator's ceiling when the turn started; the ceiling check falls back to it when the ceiling is cleared after autonomy is turned off ([spend](spend.md#stopping)) |
| `stop_requested_at` | timestamp null | set by the ceiling stop before it cancels ([spend](spend.md#stopping)) |
| `outcome` | text null | null while open; then `completed`, `failed`, `cancelled`, `stopped_at_ceiling`, `send_failed`, `interrupted` |
| `cost_subcents` | bigint null | set at turn end ([spend](spend.md#per-turn-cost)) |
| `started_at` | timestamp not null | |
| `finished_at` | timestamp null | |

A partial unique index `(coordinator_id) WHERE outcome IS NULL` makes the
open turn a single row per coordinator on both dialects; this is the claim
`AC-COORDINATOR-WAKE-005.2` rests on. Index `(coordinator_id, started_at, id)`.

Retention: the startup pass deletes wake rows with status `delivered` or
`superseded` and `updated_at` older than 30 days, and turn rows with
`finished_at` older than 90 days. Coordinator delete and `workspace.deleted`
delete both tables' rows for the coordinator in the same transaction that
deletes its proposals.

## Wake lock

Three writes read a coordinator's state and then write on it: recording a
wake (count, then insert), delivery step 3 (autonomy, then the turn row), and
the autonomy-off PATCH. Each runs as one write transaction that first takes
the coordinator's wake lock: on PostgreSQL `SELECT
pg_advisory_xact_lock(hashtextextended('coordinator_wake:' || ?, 0))` with the
coordinator id, the same shape `office/repository/sqlite/participants.go`
uses; on SQLite the single writer connection already serialises write
transactions. The lock is released at commit or rollback, so the three writes
see each other's committed results and never interleave.

## Own tasks

`ListOwnTasks(ctx, coordinatorID)` selects `task_id` from
`coordinator_proposals` where `coordinator_id = ?`, `status = 'approved'` and
`task_id IS NOT NULL`, joins `tasks` on id, and keeps rows with
`archived_at IS NULL` and `is_ephemeral = false`, ordered by task id. The
reverse lookup the recorder needs, `CoordinatorsOwningTask(ctx, taskID)`,
uses the same predicates plus `coordinators.autonomy_enabled = 1`; a task can
be owned by at most one coordinator because a proposal's external id is
unique, but the method returns a list and the recorder loops.

## Recorder

`internal/coordinator/wake_recorder.go` subscribes, only while phase 3 is
effective:

| Event | Kind | Episode key source |
| --- | --- | --- |
| `session.pending_action_changed` with `pending_action` `clarification` | `question` | the task repository's pending clarification bundle for the session: its `pending_id` |
| `session.pending_action_changed` with `pending_action` `permission` | `permission` | the task repository's pending permission message for the session: its `pending_id` |
| `coordinator_stalls` upsert (hooked in `stalls.go` after a row is written) | `stall` | the row's `last_event_at`, only while the row is [current](#stall-currency) |
| `task_session.error_changed` with `active: true` | `error` | the payload's `stamp` |
| `task.state_changed` with `state` `COMPLETED` | `completed` | `completed` |

For each event the recorder resolves `CoordinatorsOwningTask`, skips a session
that is not the task's primary session, re-reads the condition from stored
state, and when it holds calls `RecordWake`. `RecordWake` takes the
[wake lock](#wake-lock), counts the coordinator's pending rows and, below 200,
runs `INSERT ... ON CONFLICT (coordinator_id, task_id, kind, episode_key) DO
NOTHING` in the same transaction. At 200 or more it inserts nothing and
increments `coordinator_wake_dropped_total{reason="cap"}`. Because the count
and insert run under the lock, concurrent recording never passes 200; only
wakes returned to `pending` by a failed or interrupted turn can, and they are
not inserts (`AC-COORDINATOR-WAKE-001.4`). A committed insert calls `Kick(coordinatorID)`
([Delivery](#delivery)). A read or insert error is logged at warn and dropped;
the backstop recovers it.

## Backstop

`internal/coordinator/wake_backstop.go` runs one goroutine with a 60-second
ticker, started after the startup pass and stopped (joined) before the store
closes. Its duties come in two groups. **Turn and setting duties** run for a
coordinator whatever its `autonomy_enabled` reads. **Wake duties** run only
while autonomy is on. Turning autonomy off therefore stops new wakes and
deliveries at once, but it never abandons a turn that is already open or an
undo lowering that is already owed (`AC-COORDINATOR-WAKE-004.3`).

Each tick:

1. Builds the visit set, the union of three queries, deduplicated and ordered
   by coordinator id:
   - coordinators with `autonomy_enabled = 1`;
   - coordinators with a `coordinator_unattended_turns` row that has
     `outcome IS NULL`, or `finished_at` in the last 10 minutes;
   - coordinators with at least one `coordinator_proposals` row with
     `claimed_automatically = 1` ([automatic](automatic.md#lowering)).

   A query that fails logs at warn and increments
   `coordinator_backstop_skipped_total`, and the tick continues with the
   other queries. A coordinator deleted between the list and its visit is
   skipped.
2. For each coordinator in the visit set, re-reads the coordinator row and
   runs the turn and setting duties in this order:
   1. message recovery for its open turn with `message_id` null
      ([Finding the turn's message](#finding-the-turns-message));
   2. missed-settle re-derivation for its open turn ([Turn end](#turn-end)),
      then the per-turn cost recompute for its turns settled in the last 10
      minutes ([spend](spend.md#per-turn-cost));
   3. the ceiling check `CheckCeiling` of [spend](spend.md#stopping) for its
      open turn, if any, including a row with `stop_requested_at` set;
   4. the lower-on-undo retry of [automatic](automatic.md#lowering).

   A read or write error in one duty logs at warn, increments
   `coordinator_backstop_skipped_total`, and does not skip the later duties
   or coordinators.
3. Only when the re-read row has `autonomy_enabled = 1`, runs the wake duties:
   1. reads `ListOwnTasks` and, per task, the current episodes from stored
      state: the primary session's pending clarification bundle and pending
      permission message, the task's `coordinator_stalls` row when
      [current](#stall-currency), the primary session's active error, and the
      task state. It calls `RecordWake` for each. A read error for one
      coordinator logs at warn, increments
      `coordinator_backstop_skipped_total`, and moves on to the next
      coordinator.
   2. Calls `Deliver(coordinatorID)`.

With autonomy off, an open turn therefore keeps the 60-second ceiling bound of
`AC-COORDINATOR-SPEND-003.2`, the `stop_failing` state of
`AC-COORDINATOR-SPEND-003.4`, and the recovery of a missed settle. No wake is
recorded for it and nothing is delivered after it ends. Once its turns are
settled and past the 10-minute recompute, and it has no automatic claim, the
coordinator leaves the visit set.

### Stall currency

A `coordinator_stalls` row is never deleted when its task resumes; it stays
until the 30-day prune. Every reader in this design (recorder, backstop and
delivery step 2) therefore treats a stall row as a condition only while it is
current: the task's persisted `TaskStatusSummary.last_activity_at` is absent
or not later than the row's `detected_at`. This is the test Needs you applies
(`AC-COORDINATOR-NEEDS-YOU-001.2`, [needs-you design](needs-you.md)). A task
that resumes before delivery makes its stall wake's condition end, and
delivery supersedes it (`AC-COORDINATOR-WAKE-005.6`). A later stall upserts a
new `last_event_at`, which is a new episode key.

The recorder and the backstop call the same `RecordWake` with the same keys,
which is why a restart, a redelivered event and a backstop pass converge on
one row (`AC-COORDINATOR-WAKE-001.2`). `task.stalled` is emitted again for a
still-stalled task after a restart; the stall row's `last_event_at` does not
change, so its key does not either.

## Admission

`Admit(ctx, coordinatorID) (ok bool, reason string, detail string)` reads the
coordinator row and runs, in order, stopping at the first failure:

| Order | Check | Reason |
| --- | --- | --- |
| 1 | `autonomy_enabled = 1` | `autonomy_off` |
| 2 | `containment.Check` passes ([containment](containment.md#check)) | `containment`, detail = the failing condition name |
| 3 | spend is measurable ([spend](spend.md#measurement)) | `spend_unmeasured` |
| 4 | spend below the ceiling | `ceiling_reached` |
| 5 | `conversation_task_id` set and that task exists, is not archived | `no_conversation` |
| 6 | the task's primary session exists and is not `FAILED`, `CANCELLED` or `COMPLETED` | `conversation_unavailable` |
| 7 | that session is `WAITING_FOR_INPUT`, has no pending action, no queued message and no open turn | `conversation_busy` |
| 8 | the newest settled turn row's `finished_at` is at least 5 minutes ago, or there is none | `cooldown` |

A `CREATED` session that never started counts as idle for check 7. Every read
error inside a check fails that check with its reason. `Admit` writes nothing.

## Delivery

`Deliver(coordinatorID)` is serialised per coordinator by an in-process keyed
mutex, and across processes by the partial unique index. `Kick` schedules a
`Deliver` on a bounded worker (one per coordinator at a time; a kick while one
is scheduled is coalesced). Triggers: a committed wake insert, a
`task_session.state_changed` to `WAITING_FOR_INPUT` for a current conversation
session, a committed PATCH of `autonomy_enabled` or `cost_ceiling_usd`, a
ceiling-releasing turn end, and each backstop tick.

1. `Admit`; on failure return. Delivery never creates, archives or repoints a
   conversation task, and never starts any session other than the admitted
   one (`AC-COORDINATOR-WAKE-003.1`).
2. Read every `pending` wake ordered by `created_at`, `id`; the cap bounds
   this to 200 rows plus at most 20 returned by a failed turn. For each,
   re-check its [episode](#episode-recheck) from stored state with the same
   readers the backstop uses; mark every one whose episode ended
   `superseded`, and keep the first 20 that still hold. With none, return.
   This read is the evaluation point of "still holds"
   (`AC-COORDINATOR-WAKE-005.1`): an episode that ends after it, before or
   during the turn, is still delivered, and the turn message tells the agent
   to read current state.
3. In one transaction under the [wake lock](#wake-lock): re-read the
   coordinator row and roll back if `autonomy_enabled` is not 1; insert the
   turn row (`outcome` null, `message_id` null, `start_ceiling_subcents`
   copied from the re-read row's `cost_ceiling_subcents`, which autonomy on
   guarantees is set), where a unique violation
   means another delivery holds the open turn, so roll back; `UPDATE
   coordinator_wakes SET status='delivered', turn_id=? WHERE id IN (...) AND
   status='pending'`, and roll back if it changed no row; set `wake_count` to
   the rows it changed; commit. The unattended turn starts at this commit, so
   an autonomy-off PATCH that commits first makes this step roll back, and one
   that commits after it finds a running turn (`AC-COORDINATOR-WAKE-004.3`).
4. Build the turn message ([Transcript](#transcript)) and send it through the
   orchestrator's direct prompt path (`orchestrator.Service.PromptTask`,
   extended with an options value carrying the coordinator's system author
   and `metadata.coordinator_wake_turn_id` set to the turn row id). Delivery
   never uses the message queue (`orchestrator/messagequeue`), so a wake
   message is never queued behind another turn: admission check 7 ran in
   step 1, and a manager message that made the session busy since then makes
   this send fail with `ErrAgentPromptInProgress` or
   `ErrSessionNotPromptable`.
5. On success, set `message_id` and `session_turn_id` from the stored
   message. On a send error, [find the turn's message](#finding-the-turns-message):
   when found, record it as on success, because the prompt was sent. When it
   is not found and the error is a refusal before dispatch
   (`ErrAgentPromptInProgress`, `ErrSessionNotPromptable`, or an invalid
   request), in one transaction set the turn `outcome='send_failed'`,
   `finished_at`, and return its wakes to `pending` with `turn_id` null; the
   next trigger holds at admission with `conversation_busy` until the session
   is idle. Any other error (timeout, cancelled context, transport) leaves the
   turn open with `message_id` null for the backstop
   (`AC-COORDINATOR-WAKE-005.3`). Because nothing is queued, a message the
   lookup cannot find within two minutes was never dispatched, which is what
   makes the two-minute `send_failed` settle safe.

### Episode recheck

A pending wake still holds only when stored state shows the same episode, not
merely a condition of the same kind:

| Kind | Still holds when |
| --- | --- |
| `question` | the primary session's pending clarification bundle has `pending_id` equal to `episode_key` |
| `permission` | a pending permission message on the primary session has `pending_id` equal to `episode_key` |
| `stall` | the task's stall row is [current](#stall-currency) and its `last_event_at` equals `episode_key` |
| `error` | the primary session's active error has `stamp` equal to `episode_key` |
| `completed` | the task's state is `COMPLETED` |

The task no longer being an own task (archived, deleted, ephemeral) ends
every kind. A read error for one wake leaves it `pending`, excludes it from
this delivery, and does not supersede it.

### Finding the turn's message

The lookup reads the conversation session's messages created at or after the
turn's `started_at` and returns the one whose
`metadata.coordinator_wake_turn_id` equals the turn id. It is the only test of
whether a send reached the conversation; nothing re-sends a turn whose message
exists. Delivery never queues, so stored messages are the only place to look.

The startup pass, and each backstop tick, examine turn rows with `outcome IS
NULL` and `message_id` null. When the lookup finds the message, they record
`message_id` and `session_turn_id` and leave the row to [Turn end](#turn-end).
When it does not, the startup pass settles the row `interrupted`, and a
backstop tick settles it `send_failed` once two minutes have passed since
`started_at` and the session is not `RUNNING` or `STARTING`; either way its
wakes return to `pending` with `turn_id` null. The settle is conditional on
`outcome IS NULL AND message_id IS NULL`.

## Turn end

An unattended turn is bound to its session turn (`session_turn_id`), not to
the session's state. The orchestrator returns a session to
`WAITING_FOR_INPUT` and then immediately drains a queued message
(`drainQueuedMessageForPromptableSession` in
`orchestrator/event_handlers_agent.go`), so a session that is `RUNNING` again
may be running a manager's attended turn.

A subscriber on `turn.completed` settles the open turn row whose
`session_turn_id` equals the completed turn's id. The outcome comes from the
session's state when the settle runs: a row with
`stop_requested_at` set settles `stopped_at_ceiling`; otherwise `FAILED`
settles `failed`, `CANCELLED` settles `cancelled`, and any other state (including `RUNNING` on a drained queued message) settles
`completed`. The settle is `UPDATE ... WHERE id=? AND outcome IS NULL`,
computes `cost_subcents` ([spend](spend.md#per-turn-cost)), and then calls
`Kick` so a wake recorded during the turn is delivered after the cooldown.
The backstop re-derives a missed settle: an open turn whose `session_turn_id`
turn has `completed_at` set, or whose session's active turn
(`GetActiveTurnBySessionID`) is a different turn or none, is settled by the
same rule.

Everything that acts on "the open unattended turn" while it runs (the
permission denial of [containment](containment.md#unattended-permissions) and
the ceiling stop of [spend](spend.md#stopping)) first checks that the
request's or the session's active turn id equals the row's
`session_turn_id`. A turn of the same session with another id is attended and
is never denied or stopped by this design.

## Transcript

The message body is agent-facing English, not UI copy:

```text
Unattended turn. No person started this turn or is watching it.
Events since your last turn (N):
- question on <identifier> "<title, 80 characters>"
- stall on <identifier> "<title>"
These were current when this turn started and may have changed since; read
current state before acting. Propose what should happen. Proposals wait for a
manager.
```

Titles are quoted, truncated and stripped of newlines; they are still board
content ([ADR residual](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md#residual-risk)).
The web transcript renderer recognises `metadata.coordinator_wake_turn_id`
and renders the message as the "Woken by N events" entry with an expandable
list and, when non-zero, the denied-permission count, using the turn row from
the autonomy read. Copy goes through `t()` in six locales.

## Autonomy read

`GET /api/v1/workspaces/:id/coordinators/:cid/autonomy` (`workspace.read`)
returns:

```json
{
  "autonomy_enabled": true,
  "admission": {"ok": false, "reason": "containment", "detail": "auth_enabled"},
  "pending_wakes": 3,
  "oldest_pending_at": "2026-09-29T09:00:00Z",
  "last_turn": {"id": "...", "started_at": "...", "outcome": "completed", "cost_subcents": 5100, "denied_permissions": 0, "stop_state": null},
  "containment": {"conditions": [{"name": "executor_isolated", "met": true, "detail": ""}]},
  "spend": {"window_subcents": 64000, "mean_daily_subcents_7d": 58000, "measurable": true, "degraded": false, "ceiling_subcents": 100000}
}
```

It runs `Admit` read-only; `admission` is present only when autonomy is on.
`last_turn.stop_state` is `null`, or `"stop_failing"` per
[spend](spend.md#stopping).
`coordinator.updated` gains optional `autonomy_changed: true`, published on
every wake insert, delivery, turn settle and autonomy PATCH, so clients
re-read.

## Screens

- `apps/web/app/coordinator/use-coordinator-inputs.ts` gains the autonomy
  input, same `{value, loadedAt, error}` shape, re-read on mount, Try again
  and `coordinator.updated` with `autonomy_changed`.
- The autonomy strip (UI-01) renders above the count strip while
  `autonomy_enabled`: "Autonomy: Active" or "Autonomy: Held (<reason text>)",
  "Last woke <age>" or "Not woken yet", "<n> pending", and the spend pill from
  [spend](spend.md#screens). A read error shows "Autonomy state unavailable"
  with Try again (`AC-COORDINATOR-WAKE-006.4`). While autonomy is off, the
  strip renders only when `last_turn.stop_state` is `"stop_failing"`, and
  then shows "Autonomy: Off" with the stop warning and Stop of
  [spend](spend.md#stopping), so a turn left open by turning autonomy off
  still surfaces a failing stop (`AC-COORDINATOR-SPEND-003.4`).
- `classify` in `apps/web/lib/coordinator/attention.ts` gains an optional
  `autonomy` input and emits one item of kind `autonomy` when the reason is in
  the persistent set of `AC-COORDINATOR-WAKE-006.2` and `pending_wakes > 0`,
  reference time `oldest_pending_at`, kind rank 4, id `autonomy:<cid>`. It
  counts in the coordinator's attention count only.
- Held reason texts: `containment` "Containment not in place: <condition>";
  `spend_unmeasured` "Spend cannot be measured"; `ceiling_reached` "Cost
  ceiling reached"; `no_conversation` "Open the copilot once to give it a
  conversation"; `conversation_unavailable` "The conversation stopped. Open
  the copilot to restart it". The item offers **Open settings** to managers.

## Failure and recovery

| Failure | Result |
| --- | --- |
| Event lost or subscriber error | Backstop stores the wake within 60 s |
| Process stops after step 3 of delivery | Startup pass finds the message (turn continues) or sets `interrupted`, wakes back to `pending` |
| Send refused before dispatch | `send_failed`, wakes back to `pending`, retried on the next trigger after the cooldown |
| Send outcome unknown | Message found: the turn continues. Not found after two minutes: `send_failed`, wakes back to `pending` |
| Autonomy turned off during delivery | Step 3 re-reads the flag under the wake lock and rolls back |
| Autonomy turned off while a turn is open | The turn runs on; the backstop keeps its ceiling check, recovery and settle until it ends; no wake is recorded or delivered |
| Two deliveries race | Partial unique index admits one turn row |
| Conversation busy or unavailable | Wakes wait; nothing is created |

## Security

- Only the PATCH route writes `autonomy_enabled`; it requires
  `workspace.manage` and the MCP guard refuses every settings action for a
  coordinator principal.
- The turn message is authored by the system, not by a user, so it cannot be
  confused with a manager's message; the no-turn-start table in
  `internal/coordinator/no_turn_start_test.go` gains exactly one allowed path,
  `Deliver` with admission passed, and every other row stays.

## Observability

Counters `coordinator_wake_recorded_total{kind}`,
`coordinator_wake_dropped_total{reason}`,
`coordinator_wake_delivered_total`, `coordinator_wake_superseded_total`,
`coordinator_unattended_turn_total{outcome}`,
`coordinator_admission_held_total{reason}` and
`coordinator_backstop_skipped_total`, as expvar plus structured zap logs. No
task, coordinator or session id is a label.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
- [Global run scheduler ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md)
