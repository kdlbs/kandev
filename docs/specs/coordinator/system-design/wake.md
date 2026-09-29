---
id: coordinator-wake-design
title: Wake on its tasks' events design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-30
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
| `REQ-COORDINATOR-WAKE-001` | [Store](#store), [Recorder](wake-recording.md#recorder) |
| `REQ-COORDINATOR-WAKE-002` | [Backstop](wake-backstop.md#backstop) |
| `REQ-COORDINATOR-WAKE-003` | [Admission](#admission), [Delivery](#delivery) |
| `REQ-COORDINATOR-WAKE-004` | [Flag and settings](#flag-and-settings), [Admission](#admission) |
| `REQ-COORDINATOR-WAKE-005` | [Delivery](#delivery), [Turn end](#turn-end), [Transcript](#transcript) |
| `REQ-COORDINATOR-WAKE-006` | [Autonomy read](#autonomy-read), [Screens](#screens) |

## Flag and settings

- `features.coordinatorPhase3` (environment
  `KANDEV_FEATURES_COORDINATOR_PHASE3`) is registered in
  `internal/runtimeflags/registry.go` with `RestartRequired: true` and in root
  `profiles.yaml` as `prod: "false"`, `dev: "false"`, `e2e: "true"`, per the
  `/runtime-feature-flags` checklist. Phase 3 is **effective** only when it,
  `features.coordinatorPhase2` and `features.coordinator` are all on
  ([integration](integration.md#effective-condition));
  `internal/backendapp/coordinator.go`
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
  (`cost_ceiling_usd` as a decimal string with two places, or `null`) while
  phase 3 is effective, and carries neither key otherwise, the way phase 2's
  fields are omitted while its flag is off. The activity row's
  `unattended_turn_id` follows the same rule: present (string or `null`) only
  while phase 3 is effective; task 01 owns that DTO field and its omission,
  task 05 the stamping. The web computes "effective" as the AND of the
  three `useFeature` reads (`coordinator`, `coordinatorPhase2`,
  `coordinatorPhase3`) in one hook, and the typed client marks the new fields
  optional.

The PATCH field rules (check order, presence, idempotency, post-commit
`Kick` and `autonomy_changed`) are in
[integration](integration.md#autonomy-patch-and-deletion).

## Store

Both tables live in `internal/coordinator/store_phase2_schema.go`'s
`phase2TablesSQL` (their indexes in `phase2IndexesSQL`), created with
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
| `episode_key` | text not null | per the requirement's episode table, never empty; see [Episode keys](wake-recording.md#episode-keys) |
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
`superseded`, `updated_at` older than 30 days and a task that cannot become an
[own task](wake-recording.md#retention-and-own-tasks), and turn rows with
`finished_at` older than 90 days. Coordinator delete and `workspace.deleted`
delete both tables' rows for the coordinator in the same transaction that
deletes its proposals.

Deletion of every phase 3 table, and the retention pass, are in
[integration](integration.md#autonomy-patch-and-deletion).

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

`Store.WithWakeLock(ctx, coordinatorID, fn func(tx coordinatorExec) error)
error` (task 01) is the one helper for the three writes. Contract:

- It opens its own write transaction, the way `withCoordinatorLock` does
  (a `BEGIN IMMEDIATE` connection on SQLite, `BeginTxx` on PostgreSQL), and
  hands `fn` the transaction handle; `fn` never opens another and never calls
  `WithWakeLock` again (a nested call on PostgreSQL would wait on a lock its
  own caller holds).
- Order inside the transaction, on both dialects: first the advisory lock
  (PostgreSQL only; a no-op on SQLite), then `lockCoordinatorRow` with
  `FOR UPDATE`, then `fn`. The wake lock is always taken before the
  coordinator row lock, never after, and a holder that needs the row reads it
  only after the lock. The autonomy-off PATCH cannot use the helper's own
  transaction, because phase 2's PATCH already owns one; it takes the same
  advisory lock as the first statement of that transaction, before
  `lockedCoordinatorRow`, by calling the shared `takeWakeLock(ctx, tx,
  coordinatorID)` step the helper is built on.
- A coordinator id with no row returns `ErrNotFound` before `fn` runs, so no
  wake, turn or supersede is written for a deleted coordinator.
- `fn` returning an error, or a cancelled `ctx`, rolls the transaction back
  and returns that error; a commit failure is returned
  as `commit wake lock: %w` and the writes are not applied.
- Concurrency guarantee: two callers for one coordinator run `fn`
  one after the other on both dialects, and the second sees the first's
  committed writes. Two callers for two different coordinators are not
  serialised by the lock on PostgreSQL (the advisory keys differ). On SQLite
  the single writer connection serialises every write transaction, so
  different coordinators also run one after the other there and the design
  makes no independence claim for SQLite.

## Backstop

The level-triggered backstop, which repeats wake recording every 60 seconds and runs the
per-coordinator duties, is specified in [wake backstop](wake-backstop.md#backstop).

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
| 6 | the task's primary session exists and is usable: not `FAILED`, `CANCELLED`, `COMPLETED` or `CREATED` | `conversation_unavailable`, detail `session_not_started` for `CREATED` |
| 7 | that session is `WAITING_FOR_INPUT` or `IDLE` (the direct prompt's promptable set), has no pending action, no queued message and no open turn | `conversation_busy` |
| 8 | the newest turn row with `finished_at` set, whatever its outcome (including `send_failed` and `interrupted`), has `finished_at` at least 5 minutes before now on the service clock (inclusive), or there is none | `cooldown` |

A freshly opened copilot is `CREATED` (`OpenConversation` uses
`AutoStart=false`) and the direct prompt refuses that state
(`ErrSessionNotPromptable`), so check 6 holds it. A manager's first message
starts the session, and the resulting `WAITING_FOR_INPUT` state change is a
delivery trigger. Any other or unknown session state fails check 7 with
`conversation_busy` (fail closed). The strip copy for `session_not_started`
stays task 06's.

Reads go through two coordinator-side interfaces, not orchestrator types:

- A session snapshot reader returning, for a task id: whether the task exists
  and its archived flag; the primary session id and state; whether an action
  is pending (a pending clarification bundle via `WakeSources.PendingQuestionID`
  or any pending permission via `PendingPermissionIDs`); whether a message is
  queued (the orchestrator message queue's per-session read, any entry); and
  the active turn (`ActiveTurnReader.GetActiveTurn`). A failed queue read fails
  check 7 with `conversation_busy`.
- A message finder ([Finding the turn's message](#finding-the-turns-message)).

The eight reads are sequential, not a snapshot. A change between them is
covered by step 3 of Delivery and the unique index, not by `Admit`.

`Admit` returns `(ok, reason, detail)`. `detail` is empty except for
containment's failing condition name, `session_not_started`, and `read_error`.
A missing coordinator row returns `autonomy_off` with detail
`coordinator_not_found`; a failed coordinator-row read returns `autonomy_off`
with detail `read_error`. Every other read error inside a check fails that
check with its reason and detail `read_error`. `Admit` writes nothing.

## Delivery

`Deliver(coordinatorID)` is serialised per coordinator by an in-process keyed
mutex, and across processes by the partial unique index. `Kick` schedules a
`Deliver` on a coalescing worker: one worker goroutine per coordinator. A kick
while a `Deliver` runs sets one pending flag and re-runs once afterward; any
number of kicks in that window coalesce to that one re-run. The worker has
Start and Stop on a `WaitGroup` and goleak coverage. A `Deliver` error is
logged and left to the next trigger or backstop tick. A `cooldown` or
`conversation_busy` hold schedules no timer: the next backstop tick (60
seconds) or turn-end kick delivers. Triggers: a committed wake insert, a
`task_session.state_changed` to `WAITING_FOR_INPUT` for a current conversation
session, a committed PATCH of `autonomy_enabled` or `cost_ceiling_usd`, a
ceiling-releasing turn end, and each backstop tick.

1. `Admit`; on failure return. Delivery never creates, archives or repoints a
   conversation task, and never starts any session other than the admitted
   one (`AC-COORDINATOR-WAKE-003.1`).
2. Read every `pending` wake ordered by `created_at`, `id`; the cap bounds
   this to 200 rows plus at most 20 returned by a failed turn. For each,
   re-check its [episode](#episode-recheck) from stored state with the same
   readers the backstop uses. Each supersede is its own conditional statement,
   `UPDATE coordinator_wakes SET status='superseded', updated_at=? WHERE id=?
   AND status='pending'`; a statement that changes no row lost a race and drops
   that wake from this delivery. A failed watch-set read aborts the whole
   delivery before any supersede, leaving every wake `pending`. A per-wake
   episode read error leaves that wake `pending` and excluded, and the next
   held wake takes its slot, up to 20. Keep the first 20 that still hold. With none, return.
   This read is the evaluation point of "still holds"
   (`AC-COORDINATOR-WAKE-005.1`): an episode ending after it is still
   delivered, and the turn message tells the agent to read current state.
3. In one transaction under the [wake lock](#wake-lock): re-read the
   coordinator row and roll back if `autonomy_enabled` is not 1; insert the
   turn row (`outcome` null, `message_id` null, `start_ceiling_subcents`
   copied from the re-read row's `cost_ceiling_subcents`, which autonomy on
   guarantees is set), where a unique violation
   means another delivery holds the open turn, so roll back; `UPDATE
   coordinator_wakes SET status='delivered', turn_id=? WHERE id IN (...) AND
   status='pending'`, and roll back if it changed no row; then read back
   `SELECT ... FROM coordinator_wakes WHERE turn_id=? ORDER BY created_at, id`
   in the same transaction and set `wake_count` to the rows read back (zero
   rows rolls back); commit. Those rows, and only those, are the turn's wakes
   for the transcript. The unattended turn starts at this commit, so
   an autonomy-off PATCH that commits first makes this step roll back, and one
   that commits after it finds a running turn (`AC-COORDINATOR-WAKE-004.3`).
4. Build the turn message ([Transcript](#transcript)) and send it through the
   orchestrator's direct prompt path: a new exported entry point beside
   `orchestrator.Service.PromptTask` (which takes no options and stores no
   message; the message handler stores a manager's message first) that runs
   `promptTask` with `dispatchOnly=true` and the unexported
   `promptTaskOptions` (`orchestrator/task_operations.go`). Its exported
   options value carries `metadata.coordinator_wake_turn_id` set to the turn
   row id and `onAccepted`, and no tool or binding option: the session keeps
   its bound tool list, [integration](integration.md#tool-list)). The entry
   point generates a UUID message id and stores the message itself through
   the `afterDispatchAdmission` seam of `promptTaskOptions`, with
   `MessageCreator.CreateUserMessageIdempotent` (so the id is known and a
   retry is idempotent). That seam runs once after final dispatch admission
   succeeds and before any provider I/O, and an error there rolls the claim
   back and dispatches nothing. So a send refused before or at admission stores
   no message, and "no stored message" still means "never dispatched". The
   turn id stored on the message is the prompt's claimed (reserved) turn id,
   which the entry point resolves from the reservation, because the seam runs
   before `bindPromptTurnID` and the session's active turn may not yet be
   bound. A store failure is wrapped in one exported sentinel that `Deliver`
   classifies as a refusal before dispatch. The residual is a crash between
   the store and the first provider I/O: the message exists, so the turn is
   treated as sent. The stored message has `author_type` `user` (there is no
   system author type) and is marked only by that metadata key. Delivery
   never uses the message queue (`orchestrator/messagequeue`), so a wake
   message is never queued behind another turn: admission check 7 ran in
   step 1, and a manager message that made the session busy since then makes
   this send fail with `ErrAgentPromptInProgress` or
   `ErrSessionNotPromptable`. The options also carry the `onAccepted(turnID)`
   hook of `promptTaskOptions` (`orchestrator/task_operations.go`), run at the
   agentctl acceptance boundary before the prompt runs. It sets
   `session_turn_id` with `UPDATE ... SET session_turn_id = ? WHERE id = ? AND
   outcome IS NULL AND session_turn_id IS NULL`, so the column binds at
   acceptance. A request that beats the binding, or a failed binding (warn log),
   is matched by session and denied ([containment](containment.md#unattended-permissions)).
5. On success, set `message_id` to the generated message id (the message is
   stored before dispatch) and, if `onAccepted` did not, `session_turn_id`
   with the same conditional update. On a send error, [find the turn's message](#finding-the-turns-message):
   when found, set `message_id` from it and record `session_turn_id` as on
   success, because the prompt was sent. A find read error is distinct from
   not-found and leaves the turn untouched for the backstop. When it
   is not found and the error is a refusal before dispatch
   (`ErrAgentPromptInProgress`, `ErrSessionNotPromptable`, an invalid
   request, or the store-failure sentinel of `afterDispatchAdmission`), in one transaction set the turn `outcome='send_failed'`,
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

The task no longer being a watched own task (archived, deleted, ephemeral,
or its workflow outside the watch set) ends every kind. A read error for one wake leaves it `pending`, excludes it from
this delivery, and does not supersede it.

### Finding the turn's message

The lookup is a coordinator-side message finder: given a session id, a metadata
key and value, and `since`, it returns `{ID, TurnID}` or nil. It reads the
conversation session's messages created at or after the turn's `started_at`
and returns the one whose `metadata.coordinator_wake_turn_id` equals the turn
id, oldest first by (`created_at`, `id`); more than one match logs a warning
and takes the first. A read error is distinct from not-found and leaves the
turn untouched that tick. It is the only test of
whether a send reached the conversation; nothing re-sends a turn whose message
exists. Delivery never queues, so stored messages are the only place to look.

The startup pass, and each backstop tick, examine turn rows with `outcome IS
NULL` and `message_id` null. The startup pass examines only open rows whose
`started_at` is before its own start time, so a live in-flight turn is never
settled `interrupted`. When the lookup finds the message, they record
`message_id` and `session_turn_id` (conditional, as in Delivery) and leave the row to [Turn end](#turn-end).
When it does not, the startup pass settles the row `interrupted`, and a
backstop tick settles it `send_failed` once two minutes have passed since
`started_at` and the session is not `RUNNING` or `STARTING`; either way its
wakes return to `pending` with `turn_id` null. The settle is conditional on
`outcome IS NULL AND message_id IS NULL`.

## Turn end

An unattended turn is bound to its session turn (`session_turn_id`), not to
the session's state. The orchestrator returns a session to
`WAITING_FOR_INPUT` and then immediately drains a queued message
(`drainQueuedMessageForPromptableSession`, defined in
`orchestrator/event_handlers_workflow.go` and called from
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
The backstop re-derives a missed settle, only for rows whose `session_turn_id`
is non-null (a row with it null is handled only by message recovery and the
two-minute `send_failed` rule, as in [spend](spend.md)): an open turn whose `session_turn_id`
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
manager unless one has allowed automatic creation of tasks.
```

The message carries no orders, goal, context or tool names: the turn has the
conversation's instructions and bound tool list
([integration](integration.md#instructions-orders-and-goal)).

N equals the turn's `wake_count`, and the list has exactly the rows read back
at step 3, oldest first. Step 4 reads each wake's task for the identifier and
title; a task that cannot be read (deleted since step 2, or a failed read)
is listed by its stored identifier with an empty title, and the turn still
sends. Titles are quoted, truncated to 80 characters (an empty title renders
as `""`) and stripped of newlines; they are still board
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

It runs `Admit` read-only through a no-count path: step 2 calls
`containment.Check` directly, not `CheckForAdmission`, so a read never moves the
counter, the state-change log or its previous key
([containment](containment.md#observability)). `admission` is present only when autonomy is on.
When `admission.reason` is `cooldown`, `admission` also carries `until`, the
newest settled turn row's `finished_at` plus 5 minutes (RFC 3339 UTC); it is
absent for every other reason.
`last_turn.stop_state` is `null`, or `"stop_failing"` per
[spend](spend.md#stopping).
`coordinator.updated` gains optional `autonomy_changed: true`, published on
every wake insert, delivery, turn settle and autonomy PATCH that changes
`autonomy_enabled` or the ceiling, so clients re-read.

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
- The two transient reasons are not holds on the strip. For
  `conversation_busy` the strip shows "Autonomy: Active (Waiting for the
  conversation)", and for `cooldown` "Autonomy: Active (Between turns until
  <time>)", where `<time>` is `admission.until` in the viewer's locale short
  time format. Neither offers **Open settings**, and neither produces an item
  (`AC-COORDINATOR-WAKE-006.2`). With `admission.ok` true the strip shows
  "Autonomy: Active" alone. Every other reason shows "Autonomy: Held (<reason
  text>)" from the catalog above.

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
| Conversation busy, unavailable or not started (`CREATED`) | Wakes wait; nothing is created or started |
| Message store fails at the dispatch boundary | Claim rolled back, nothing dispatched, `send_failed`, wakes back to `pending` |

## Security

- Only the PATCH route writes `autonomy_enabled`; it requires
  `workspace.manage` and the MCP guard refuses every settings action for a
  coordinator principal.
- The turn message is stored with `author_type` `user` and marked by
  `metadata.coordinator_wake_turn_id` alone, because the message model has no
  system author; the web renderer and the turn lookup key on that marker, so
  it is not confused with a manager's message there. The no-turn-start table in
  `internal/coordinator/no_turn_start_test.go` gains exactly one allowed path,
  `Deliver` with admission passed, and every other row stays.

## Observability

Counters `coordinator_wake_recorded_total{kind}`,
`coordinator_wake_dropped_total{reason}` (closed set: `cap`, `autonomy_off`,
`not_own`, `not_found`, `read_error`, `write_error`),
`coordinator_wake_delivered_total`, `coordinator_wake_superseded_total`,
`coordinator_unattended_turn_total{outcome}`,
`coordinator_admission_held_total{reason}` and
`coordinator_backstop_skipped_total`, as expvar plus structured zap logs. No
task, coordinator or session id is a label.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
- [Global run scheduler ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md)
