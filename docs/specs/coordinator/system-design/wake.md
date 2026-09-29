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
| `REQ-COORDINATOR-WAKE-005` | [Delivery](#delivery), [Turn end](#turn-end), [Recovery](wake-recovery.md#finding-the-turns-message), [Transcript](wake-screens.md#transcript) |
| `REQ-COORDINATOR-WAKE-006` | [Autonomy read](wake-screens.md#autonomy-read), [Screens](wake-screens.md#screens) |

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

`Admit(ctx, coordinatorID, mode) (ok bool, reason string, detail string)` reads
the coordinator row and runs, in order, stopping at the first failure. `mode`
is `AdmitCounting` (delivery's call) or `AdmitReadOnly` (the
[autonomy read](wake-screens.md#autonomy-read)); the two differ only in check 2, and `mode`
has no zero value that means either (an unset mode is a programming error and
returns `autonomy_off` with detail `read_error`).

| Order | Check | Reason |
| --- | --- | --- |
| 1 | `autonomy_enabled = 1` | `autonomy_off` |
| 2 | containment passes ([containment](containment.md#check)): `AdmitCounting` calls `CheckForAdmission` (counts and logs), `AdmitReadOnly` calls `Check` (never counts) | `containment`, detail = the failing condition name |
| 3 | spend is measurable ([spend](spend.md#measurement)) | `spend_unmeasured` |
| 4 | spend below the ceiling | `ceiling_reached` |
| 5 | `conversation_task_id` set and that task exists, is not archived | `no_conversation` |
| 6 | the task's primary session exists and is usable: not `FAILED`, `CANCELLED`, `COMPLETED` or `CREATED` | `conversation_unavailable`, detail `session_not_started` for `CREATED` |
| 7 | that session is `WAITING_FOR_INPUT` or `IDLE` (the direct prompt's promptable set), has no pending action, no queued message (`messagequeue.Service.HasPendingForSession`), no active turn, and the coordinator has no turn row with `outcome IS NULL` | `conversation_busy`, detail `turn_open` for the open turn row, `read_error` for a failed read |
| 8 | the newest turn row with `finished_at` set (`ORDER BY finished_at DESC, id DESC LIMIT 1`), whatever its outcome (including `send_failed` and `interrupted`), has `finished_at` at least 5 minutes before now on the service clock (inclusive), or there is none | `cooldown` |

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
  queued (`messagequeue.Service.HasPendingForSession`, which returns its read
  error and counts every entry, including a reserved in-flight one, so a send
  the queue is mid-delivering holds; `GetStatus` is not used because it
  swallows a failed list read as an empty queue); the active turn
  (`ActiveTurnReader.GetActiveTurn`). A failed queue read
  fails check 7 with `conversation_busy` and detail `read_error`.
- A message finder ([Finding the turn's message](wake-recovery.md#finding-the-turns-message)).

A queue whose auto-run is paused still holds check 7 with
`conversation_busy`: the design adds no separate reason or strip copy for it,
and a manager clears or resumes the queue.

The eight reads are sequential, not a snapshot. A change between them is
covered by step 3 of Delivery and the unique index, not by `Admit`.

`Admit` returns `(ok, reason, detail)`. `detail` is empty except for
containment's failing condition name, `session_not_started`, `turn_open` and
`read_error`.
A missing coordinator row returns `autonomy_off` with detail
`coordinator_not_found`; a failed coordinator-row read returns `autonomy_off`
with detail `read_error`. Every other read error inside a check fails that
check with its reason and detail `read_error`. `Admit` writes nothing.

## Delivery

`Deliver(coordinatorID)` is serialised per coordinator by an in-process keyed
mutex, and across processes by the partial unique index. `Kick` schedules a
`Deliver` on a coalescing per-coordinator worker that lives only while there
is work: the first `Kick` for a coordinator with no live worker starts one
goroutine on a `WaitGroup`; it runs `Deliver`, re-runs once if any kicks
arrived during the run (any number coalesce to one pending flag), and exits
when a run ends with the flag clear. At most one goroutine per coordinator is
live and none for an idle or deleted one (`Deliver` for a missing coordinator
ends at `Admit` with `coordinator_not_found`). `Stop` latches closed (a `Kick`
after `Stop` is a no-op), cancels the context of every `Deliver` in progress
and waits on the `WaitGroup`; goleak covers it. Task 05 adds
`svc.StopDelivery()` to the phase 3 cleanup closure that
`registerCoordinatorRoutes` (`internal/backendapp/coordinator.go`) already
registers through `addCleanup` for `StopWakeRecorder` and `StopWakeBackstop`,
after those two calls; cleanups run in reverse registration order and the
database pool's was registered earlier, so the worker is joined before the
store closes. `registerCoordinatorDelivery`'s synchronous body creates the
worker, subscribes to `turn.completed` and the session state change, and calls
`SetBackstopHooks`; the hook it returns runs the [startup pass](wake-recovery.md#startup-pass). A `Deliver` error is
logged and left to the next trigger or backstop tick. A `cooldown` or
`conversation_busy` hold schedules no timer: the next backstop tick (60
seconds) or turn-end kick delivers. Triggers: a committed wake insert, a
`task_session.state_changed` to `WAITING_FOR_INPUT` for a current conversation
session, a committed PATCH of `autonomy_enabled` or `cost_ceiling_usd`, a
ceiling-releasing turn end, and each backstop tick. The backstop's tick
reaches delivery through `Hooks.Deliver(ctx, coordinatorID)`, which task 05
implements as a `Kick` of that coordinator and returns at once, so a slow
resume never stalls the serial pass or delays `CheckCeiling` for the
coordinators after it. Every `Admit` refusal, `Deliver` outcome and settle
below also counts and publishes as the [Observability](#observability) and
[Autonomy read](wake-screens.md#autonomy-read) sections state.

1. `Admit(..., AdmitCounting)`; on failure return, counting
   `coordinator_admission_held_total{reason}`. Delivery never creates,
   archives or repoints a conversation task, and never starts any session
   other than the admitted one (`AC-COORDINATOR-WAKE-003.1`).
2. Read every `pending` wake ordered by `created_at`, `id`; the cap bounds
   this to 200 rows plus at most 20 returned by a failed turn. For each,
   re-check its [episode](#episode-recheck) from stored state with the same
   readers the backstop uses. Each supersede is its own conditional statement,
   `UPDATE coordinator_wakes SET status='superseded', updated_at=? WHERE id=?
   AND status='pending'`; a statement that changes no row lost a race and drops
   that wake from this delivery, and one that changes a row counts
   `coordinator_wake_superseded_total`. A failed watch-set read aborts the whole
   delivery before any supersede, leaving every wake `pending`. A per-wake
   episode read error leaves that wake `pending` and excluded, and the next
   held wake takes its slot, up to 20. Keep the first 20 that still hold. After
   the last supersede statement, when at least one changed a row, publish
   `coordinator.updated` with `autonomy_changed: true` once for the delivery
   (post-commit style: a failure is logged and changes nothing), whether or not a
   turn follows. With none holding, return.
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
   rows rolls back); commit, then count `coordinator_wake_delivered_total` by
   `wake_count` and publish `coordinator.updated` with `autonomy_changed: true`
   (the same post-commit publish as a wake insert; a failure is logged and
   changes nothing). Those rows, and only those, are the turn's wakes
   for the transcript. The unattended turn starts at this commit, so
   an autonomy-off PATCH that commits first makes this step roll back, and one
   that commits after it finds a running turn (`AC-COORDINATOR-WAKE-004.3`).
4. Build the turn message ([Transcript](wake-screens.md#transcript)) and send it through the
   orchestrator's direct prompt path: a new exported entry point beside
   `orchestrator.Service.PromptTask` (which takes no options and stores no
   message; the message handler stores a manager's message first) that runs
   `promptTask` with `dispatchOnly=true` and the unexported
   `promptTaskOptions` (`orchestrator/task_operations.go`). Its exported
   options value carries `metadata.coordinator_wake_turn_id` set to the turn
   row id and `onAccepted`, and no tool or binding option: the session keeps
   its bound tool list, [integration](integration.md#tool-list)). It also
   sets `reserveTurnUntilDispatch` (the prompt's turn is reserved and
   persisted before dispatch, and its `turn.started` waits for agentctl
   acceptance) and `disableDispatchRetry` (no fresh-launch fallback). The entry
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
   bound; a reservation that yields no id fails the seam like a store failure.
   The entry point records whether the seam ran. Every error it returns
   before the seam ran (an admission refusal, `ErrAgentPromptInProgress`,
   `ErrSessionNotPromptable`, `ErrSessionRuntimeUnavailable`,
   `ErrIdleSuspensionProvenanceRequired`, a runtime-ceiling seam-3 refusal, an
   invalid request, or the seam's own store failure) is wrapped in one exported
   sentinel, `ErrWakePromptNotDispatched`, which `Deliver` classifies as a
   refusal before dispatch: no message was stored and no provider I/O
   happened. Because the options include `afterDispatchAdmission`, a seam-3
   refusal is returned unchanged rather than deferred, so no replay record is
   written for a wake send (a manager's message is deferred and replayed; a
   wake send is not, and the next trigger simply asks again). An error the
   entry point returns after the seam ran (timeout, cancelled context,
   transport) is not wrapped. A stored message is never proof of a send: the
   orchestrator rolls the reserved turn back on a pre-acceptance failure, but
   `DeleteTurnIfUnreferenced` keeps a turn a message references, so the
   message and its turn both survive. The only proof is the
   [`onAccepted` binding](wake-recovery.md#sent-test). The stored message has `author_type` `user` (there is no
   system author type) and is marked only by that metadata key. Delivery
   never uses the message queue (`orchestrator/messagequeue`), so a wake
   message is never queued behind another turn: admission check 7 ran in
   step 1, and a manager message that made the session busy since then makes
   this send fail with `ErrAgentPromptInProgress` or
   `ErrSessionNotPromptable`. The options also carry the `onAccepted(turnID)`
   hook of `promptTaskOptions` (`orchestrator/task_operations.go`), run at the
   agentctl acceptance boundary before the prompt runs. It is the only writer
   of `session_turn_id`: `UPDATE ... SET session_turn_id = ? WHERE id = ? AND
   outcome IS NULL AND session_turn_id IS NULL`. An update that changes no row
   or fails logs at warn and leaves the row unbound; nothing else binds it (the
   message's `TurnID` and the session's active turn are never a source). A
   request that beats the binding, or an unbound row, is matched by session and
   denied ([containment](containment.md#unattended-permissions)).
5. On a send that returns without error, set `message_id` to the generated id
   with `UPDATE ... WHERE id=? AND outcome IS NULL AND message_id IS NULL`; a
   statement that changes no row (a settle beat it, or a value is already
   there) is a no-op that never overwrites and is not an error. `message_id`
   is informational: nothing decides on it once `outcome` is set. The step
   never writes `session_turn_id`; if `onAccepted` did not bind it the row
   stays open and unbound, and the [sent test](wake-recovery.md#sent-test) and
   the two-minute rule resolve it. The generated id is not stored on the turn
   row before the send. On a send error:
   - `ErrWakePromptNotDispatched` (a refusal before dispatch, no message and
     no provider I/O): settle `send_failed` now by the
     [settle rule](wake-recovery.md#settle-rule).
   - Any other error (timeout, cancelled context, transport): leave the turn
     open for the backstop (`AC-COORDINATOR-WAKE-005.3`); if `onAccepted` bound
     the row it is sent and continues, otherwise the two-minute rule settles
     it. Nothing is queued, so an unbound turn is never sent later.
   A refusal therefore repeats at most once per cooldown: the next trigger
   holds at admission with `conversation_busy` until the session is idle, or at
   `cooldown`.

### Episode recheck

A pending wake still holds only when stored state shows the same episode, not
merely a condition of the same kind:

| Kind | Still holds when |
| --- | --- |
| `question` | the primary session's pending clarification bundle has `pending_id` equal to `episode_key` |
| `permission` | a pending permission message on the primary session has `pending_id` equal to `episode_key` |
| `stall` | the task's stall row is [current](wake-backstop.md#stall-currency) and its `last_event_at` equals `episode_key` |
| `error` | the primary session's active error has `stamp` equal to `episode_key` |
| `completed` | the task's state is `COMPLETED` |

The task no longer being a watched own task (archived, deleted, ephemeral,
or its workflow outside the watch set) ends every kind. Comparison is exact
string equality with `episode_key`. A source that answers "none" (the primary
session, the pending bundle, the permission message, the error or the stall
row is absent, or a stored field the comparison needs is null or empty) means
the condition does not hold, and delivery supersedes the wake; an
`episode_key` that cannot equal the stored value likewise does not hold. Only
a returned error is a read error: it leaves that wake `pending`, excludes it
from this delivery, and does not supersede it.

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
settles `failed`, `CANCELLED` settles `cancelled`, a session that no longer
exists settles `cancelled` (its wakes stay `delivered`), and any other state
(including `RUNNING` on a drained queued message) settles `completed`. Every
settle site sets `outcome` and `finished_at` (the service clock at the settle)
in one `UPDATE ... SET outcome=?, finished_at=? WHERE id=? AND outcome IS NULL`.
Only when it changed a row does it compute `cost_subcents`
([spend](spend.md#per-turn-cost); a `TurnCost` error never blocks the settle
and the backstop recompute fills a null value), count
`coordinator_unattended_turn_total{outcome}`, publish `coordinator.updated`
with `autonomy_changed: true`, and call `Kick` so a wake recorded during the
turn is delivered after the cooldown; a duplicate `turn.completed` or a
settle that lost to the backstop does none of these.
The backstop re-derives a missed settle, only for rows whose `session_turn_id`
is non-null (a row with it null is handled only by message recovery and the
two-minute `send_failed` rule, as in [spend](spend.md)): an open turn whose `session_turn_id`
turn has `completed_at` set or no longer exists, or whose session's active
turn (`GetActiveTurnBySessionID`) is a different turn or none (a session that
no longer exists has none), is settled by the same rule. A failed read of
the turn, the session's state or its active turn leaves the row untouched
that tick.

Everything that acts on "the open unattended turn" while it runs (the
permission denial of [containment](containment.md#unattended-permissions) and
the ceiling stop of [spend](spend.md#stopping)) first checks that the
request's or the session's active turn id equals the row's
`session_turn_id`. A turn of the same session with another id is attended and
is never denied or stopped by this design.

## Failure and recovery

| Failure | Result |
| --- | --- |
| Event lost or subscriber error | Backstop stores the wake within 60 s |
| Process stops after step 3 of delivery | Startup pass: a bound row is left to turn end; an unbound row is settled `interrupted`, wakes back to `pending` |
| Send refused before dispatch | `send_failed`, wakes back to `pending`, retried on the next trigger after the cooldown |
| Send outcome unknown | Bound by `onAccepted`: sent, the turn continues. Unbound after two minutes with the session not `RUNNING` or `STARTING`: `send_failed`, wakes back to `pending`, the orphan message marked, never re-sent |
| Autonomy turned off during delivery | Step 3 re-reads the flag under the wake lock and rolls back |
| Autonomy turned off while a turn is open | The turn runs on; the backstop keeps its ceiling check, recovery and settle until it ends; no wake is recorded or delivered |
| Two deliveries race | Partial unique index admits one turn row |
| Conversation busy, unavailable or not started (`CREATED`) | Wakes wait; nothing is created or started |
| Message store fails at the dispatch boundary | Claim rolled back, nothing dispatched, `send_failed`, wakes back to `pending` |
| Message stored, never bound by `onAccepted` (pre-acceptance dispatch failure, or a lost binding) | Not sent: `send_failed` (or `interrupted` at startup), wakes back to `pending`, the orphan message marked and never re-sent; the turn the message references stays in the session |
| A read the recovery or settle needs fails | The row is left untouched and retried on the next tick |

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
`coordinator_backstop_skipped_total`, as expvar plus structured zap logs.
Owners: the recorder counts recorded and dropped, delivery step 3 counts
delivered, steps 1 and 2 count superseded and admission holds (only
`AdmitCounting` counts a hold), and every settle of a turn row counts
`coordinator_unattended_turn_total` once, only when it changed a row
(Delivery, [Finding the turn's message](wake-recovery.md#finding-the-turns-message) and
[Turn end](#turn-end)); the backstop owns its skipped counter. No
task, coordinator or session id is a label.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
- [Global run scheduler ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md)
