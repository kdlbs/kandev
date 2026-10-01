---
id: coordinator-turn-ledger-design
title: Coordinator turn ledger design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-TURN-LEDGER-001
  - REQ-COORDINATOR-TURN-LEDGER-002
  - REQ-COORDINATOR-TURN-LEDGER-003
  - REQ-COORDINATOR-TURN-LEDGER-004
  - REQ-COORDINATOR-TURN-LEDGER-005
  - REQ-COORDINATOR-TURN-LEDGER-006
---

# Coordinator turn ledger System Design

## Purpose and boundaries

The ledger records one row per coordinator turn plus a call digest and a frozen
board snapshot. It is written by observers of events phase 1 to 3 already
publish; it adds no writer to any turn path and no read to any admission,
approval or wake decision. Recording ships ahead of the phase 3.1 flag. The
tool, routes and screens that read the ledger sit behind it. Outcomes,
overrides, replay and the dream build on these tables and are designed in
[outcomes](outcomes.md), [replay](replay.md) and [shadow dream](shadow-dream.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-TURN-LEDGER-001` | [Recording](#recording), [Completion and verdict](#completion-and-verdict), [Failure isolation](#failure-isolation) |
| `REQ-COORDINATOR-TURN-LEDGER-002` | [The stamp](#the-stamp) |
| `REQ-COORDINATOR-TURN-LEDGER-003` | [Links](#links) |
| `REQ-COORDINATOR-TURN-LEDGER-004` | [Query tool](#query-tool) |
| `REQ-COORDINATOR-TURN-LEDGER-005` | [Gating](#gating), [Retention](#retention), [Migration](#migration) |
| `REQ-COORDINATOR-TURN-LEDGER-006` | [Board snapshot](#board-snapshot) |

## Tables

All ids are text primary keys generated as the store already does; timestamps
are UTC.

`coordinator_turns`: `id`, `coordinator_id`, `session_id`, `session_turn_id`,
`trigger` (`message`, `wake`, `dream`), `wake_kinds` (JSON array, at most 20),
`agent_profile_id`, `model`, `harness`, `config_revision`, `policy_revision`,
`prompt_hash`, `snapshot_hash`, `watch_scope`, `watch_ids` (JSON, the effective
workflow ids or `all`), `project_scope` (JSON, see
[watch projects](watch-projects.md)), `started_at`, `finished_at` (null while
running), `outcome`, `verdict`, `calls_truncated` (bool). Unique index on
`(session_id, session_turn_id)`. Index on `(coordinator_id, started_at, id)`.

`coordinator_turn_calls`: `id` (autoincrement integer), `turn_id`, `action`,
`target_task_id` (nullable), `allowed` (bool), `recorded_at`. Index on
`(turn_id, id)`.

`coordinator_turn_snapshots`: `hash` (primary key), `body` (JSON), `created_at`.
`coordinator_proposals` and `coordinator_activity` gain a nullable `turn_id`.
`coordinator_unattended_turns` gains a nullable unique `ledger_turn_id`.

## Recording

`ledger.Recorder` subscribes to the `turn.started` and `turn.completed` events
of the existing event bus, filters to sessions the coordinator service
identifies as a coordinator conversation session or a dream episode session
(the service's own session-to-coordinator lookup, the one the unattended stamp
uses), and never publishes.

`started_at` is the session turn's own start time from the `turn.started`
payload (`started_at`), and `finished_at` is the session turn's completion time
from `turn.completed` (`completed_at`), falling back to the handler's clock
only when the payload carries none. Link repair windows and call attribution
compare against these, so an asynchronous handler never shifts them. A dream
episode's turn is an ordinary `task_session_turns` row.

On start it builds the row and inserts with `INSERT ... ON CONFLICT
(session_id, session_turn_id) DO NOTHING`. A redelivered start finds the row and
does nothing (`001.1`). The trigger is `wake` when a `coordinator_unattended_turns`
row of the session has `session_turn_id` equal to this session turn or
`reserved_turn_id` equal to it. Unlike the unattended-turn binding
(`store_unattended.go`), an unbound row with an empty reservation does not match:
the reservation is the id of the turn the message was stored on
(`OnReserved`), so the wake's own turn always carries it, and a manager's
message in a session with an open delivery matches nothing and stays `message`
and never `needs_you`. When the start event is handled before `OnReserved` has
committed, the turn is recorded `message` and the completion handler corrects it
(below). The wake kinds are copied from the
`coordinator_wakes` rows whose `turn_id` equals the unattended-turn row's id
(first 20 by kind then wake id); the unattended-turn row itself has no kinds
column. The trigger is `dream` when the session is a dream episode
session, else `message`. After the insert the recorder links the
unattended row: `UPDATE coordinator_unattended_turns SET ledger_turn_id = ?
WHERE id = ? AND ledger_turn_id IS NULL` (`003.2`); the unique index makes a
second link fail and be logged.

At completion, a row recorded `message` is re-evaluated by the same match; if an
unattended-turn row now matches, one conditional statement (`WHERE trigger =
'message'`) sets the trigger `wake` and copies the wake kinds, and the link step
runs, so a start that beat the reservation ends as a `wake`. Nothing else ever
changes a stored trigger (`001.2`).

Completion always runs insert-if-missing first (the same `ON CONFLICT (session_id, session_turn_id) DO NOTHING` insert, whether or not the start event was lost), then the conditional update below, so a completion handled before its start, or a start handled after completion, cannot leave a row unfinished: the start insert never touches an existing row, and the update matches only `finished_at IS NULL`. If the start event was lost, the insert derives the stamp at that time. The trigger match
is the same but reads the unattended-turn row in any status except `send_failed` (the acceptance criterion states this exclusion; a `send_failed` or pause-settled unbound row never started a turn, so letting it match would label a manager's message a `wake`)
and except a row settled while unbound (`session_turn_id IS NULL` and `outcome`
set: a failed or pause-stopped delivery never started a turn, so such a row
cannot turn a manager's message into a `wake` or lend it `blocked`), since turn end may already have settled
it; when several rows match, the newest by `(started_at DESC, id DESC)` wins. A late row takes its `started_at` from the session turn row (`task_session_turns`), not from completion time; it is correct except for a model, which may stay empty until the model pass, and its snapshot hash, which is always empty (the board at the turn's start cannot be read; `001.2`, `006.1`). A delivery that failed
before any turn started (`send_failed`, no `session_turn_id`) has no ledger row;
the unattended-turn row already records it.

The turn outcome is read from the unattended-turn row's `outcome` when one is
bound (`completed`, `failed`, `cancelled`, `interrupted`, `stopped_at_ceiling`,
`stopped_by_pause`); when a row is bound but its `outcome` is still NULL because
phase 3's own turn-end settle has not run yet, completion re-enqueues itself
after 1 second, up to 3 times, and then falls through to the session-state read;
otherwise from the session's state read at completion (`failed`, `cancelled`
or ended normally), else `unknown` when that read fails or is ambiguous, so an
attended turn whose state cannot be read is `completed`-unknown, never
`blocked` by guess (the `turn.completed` payload carries no status); `task_session_turns` carries no outcome column, so it is never a
source. A ledger outcome written from the session state is corrected by the
10-minute pass below (the correction and the model fill are two independent
statements of that pass, so a row with its model already set is still
corrected): for a row finished in the last 24 hours whose bound
unattended row now reads `stopped_at_ceiling` or `stopped_by_pause` and whose
own outcome differs, one statement sets `outcome` to that value and `verdict` to
`blocked` (`WHERE outcome NOT IN ('stopped_at_ceiling', 'stopped_by_pause')`), so
first completion never records a ceiling or pause stop permanently as
`cancelled` or `unknown`.

### Ordering and ties

Ledger reads order by `(started_at, id)`. `id` is the store's generated text
id (a random UUID), so the tiebreak between two turns that start in the same
instant is arbitrary but total and fixed: the order never flips between two
reads of the same page, though it is not insertion order. Call rows order by `id`
(autoincrement), which is the recording order (`001.4`). Tool pages use a
`next_before` cursor holding `(started_at, id)`, encoded as the unpadded
base64url of `<started_at RFC 3339 nano UTC>|<id>`; the next call passes it as
the `before` argument and continues with `(started_at, id) < cursor`. A
`before` that does not decode to that shape is a validation error naming
`before`. The page fetches `limit + 1` rows; `next_before` is built from the
last returned row only when the extra row exists and is null otherwise.

## Completion and verdict

Before computing the verdict, completion drains the call queue for this turn
(every queue entry carries a per-process sequence number from a counter the
enqueue increments; completion reads the counter once when it starts and waits
until the writer's written-sequence reaches that value, at most 2 seconds, never
on the turn path since completion runs on the event subscription); calls still unwritten after that are absent from the
verdict input, and the verdict falls back to proposals and activity rows, which
are written synchronously by the guarded call. `proposed` and `acted` depend
only on those rows, so the queue can change only `blocked` by refused calls.

Completion runs as one conditional update: `UPDATE coordinator_turns SET
finished_at = ?, outcome = ?, verdict = ? WHERE id = ? AND finished_at IS NULL`.
Two writers race safely: the second matches zero rows and changes nothing
(`001.5`). The model update at completion is a second statement, `SET model = ?
WHERE id = ? AND model = ''` (`002.2`), so a late first-reported model never
overwrites an earlier one.

The verdict is computed from stored rows by a pure function
`Verdict(inputs) string`, no I/O, table-tested in precedence order (`001.3`):

1. `blocked`: outcome is `failed`, `cancelled`, `interrupted`, `stopped_at_ceiling`
   or `stopped_by_pause`; or the turn has at least one call
   row, every one refused, `calls_truncated` is false, no entry for its session on the
   retry list, and no proposal with this
   turn id. A truncated digest never yields `blocked` by refusals, because a
   dropped call may have been allowed.
2. `acted`: an automatic approval executed inside the turn. It is read from
   `coordinator_activity` rows with outcome `approved`, authorization `automatic`
   and `unattended_turn_id` equal to the id of the unattended-turn row bound to
   this ledger row (`ledger_turn_id`). A failed automatic approval
   (`failProposalStore` writes `automatic` with outcome `failed`) is excluded.
3. `proposed`: at least one proposal has this turn id.
4. `needs_you`: trigger `wake`, no proposal, `wake_kinds` names a question or
   permission wake, and the coordinator's conversation session still has a
   pending interaction at grade time. The signal is the pending-interaction read
   admission already uses (`ConversationReader.ActionPending`, backed by
   `ListPendingInteractions` for the session id). Wake status is never the
   signal: `supersedeWake` only moves `pending` wakes, so a delivered wake never
   becomes `superseded` and cannot show an answer. A failed read grades the turn
   as not `needs_you` and is logged.
5. `nothing_needed`.

A turn with zero call rows and no proposal is `nothing_needed`, not `blocked`:
the "every call refused" clause needs at least one call.

## The stamp

The recorder reads, at start, the coordinator row: agent profile id,
`config_revision`, `policy_revision` (`002.1`). The prompt hash is SHA-256 of
`StandingInstructions(...)` (`prompt.go`), the function that opens a
conversation, called through the composition root's standing-instructions
reader (`backendapp/coordinator.go`), so the inputs are the same as opening a
conversation now: workspace name and id, the coordinator's name and context,
and the standing-orders, goal and improvement sections. A standing-orders or
goal read that fails drops that section exactly as the reader does; a failed
read of the coordinator's name and context yields an empty hash. It does not read the session's stored first message, so a manager
edit that has not yet archived the conversation is reflected as the current
render and the ledger row can disagree with the conversation's real opening
text by design; the row says what the coordinator would be opened with now.
Deviation is visible because a manager save archives the conversation
(`PERMISSIONS-004.1`) and the next turn starts from the new render.

The model is empty at start and set at completion, and again by the ledger's
own 10-minute pass over rows finished in the last 24 hours whose model is still
empty (usage is often written after completion, as the turn cost recompute
already handles), from the provider-reported model of the turn's first usage row with a non-empty model
(`ORDER BY created_at, id`), lower-cased and trimmed. It is never derived from the agent profile (`002.2`). The harness is
`<agent type>@<build version>` built from the same usage row's agent type
and the build version (a constructor argument the composition root sets from
`backendapp.Version`; the ledger package imports no `backendapp`), empty when the agent type is empty (`002.3`).

Tokens and cost are joined from `task_usage_events` by session turn id on read
(`002.4`). A join with no rows returns `cost: null`; a set with an unpriced
row returns `cost: null` too. Comparisons of stamps (used by replay's baseline
reuse) go through `stamp.Equal`, which returns false whenever either side of a
field is empty (`002.5`).

## Links

Which rows a guarded call wrote is decided by columns, never by actor: a
coordinator_activity row is a guarded-call row when `outcome = 'proposed'` or
`"authorization" = 'denied'`, and never when `undo_of_id IS NOT
NULL`. Manager approve, reject, fail and return rows carry `authorization =
'requires_approval'` with outcome `approved`, `failed`, `rejected` or
`returned`, and their actor column is NULL when auth is disabled, so the actor
column discriminates nothing. Automatic approvals (successful and failed) carry `authorization = 'automatic'` and the
unattended-turn id, are written on the decision path outside the session
(`decision_phase2.go`), and never get a `turn_id`; the `acted` verdict links them
through `unattended_turn_id` and counts outcome `approved` only. Refusal rows coalesce within 60 seconds on class, reason and `unattended_turn_id`
(`recordRefusalTx`, `activity.go`); that match gains `turn_id` equality (`turn_id IS NULL` when
the call has no active ledger turn), so two attended turns never share one row and a
coalesced row keeps the turn id of the call that created it. Proposals are all created by guarded calls, so
every proposal with a NULL `turn_id` in the window is repairable.

The guarded-call layer already resolves the calling session. `ledger.ActiveTurnID(sessionID)`
reads an in-memory map from session id to the open ledger row id that the
recorder maintains (set at insert, cleared at completion, rebuilt at startup
from unfinished rows), so the proposal and activity inserts of a guarded call
perform no extra database read. The map entry holds `(rowID, sessionTurnID)`. The start handler first reads the
entry: one already carrying this `sessionTurnID` means a redelivered start, and
the handler leaves the entry alone. Otherwise it generates the row id and sets
the entry before it issues the insert, so the entry never waits on the database.
When the insert changes no row (the row exists), the handler reads the existing
row by `(session_id, session_turn_id)`: an unfinished row replaces the entry
(`rowID` of the existing row), and a finished row removes the entry if it still
holds the generated id, so a redelivered start never leaves an entry whose id
has no row. Every map change is a compare-and-swap on `(rowID, sessionTurnID)`: completion
removes the entry only when it still holds the completing turn's `sessionTurnID`, a start never
replaces an entry whose `sessionTurnID` belongs to a later turn, and an insert that fails with
an error other than a conflict removes the entry it set if it still holds the generated id. A
guarded call that read that id in the meantime may persist it; such a dangling turn id reads as
unknown (`003.3`). The rebuild at startup puts the newest unfinished row of each session
by `(started_at DESC, id DESC)` in the map; older unfinished rows of the same
session stay and are settled by completion or the retention settle. The handler
itself is asynchronous, so a guarded call that beats it sees no entry and its
write proceeds with an empty `turn_id` (`003.1`). Completion
repairs those rows: for proposals of the coordinator, and activity rows of the coordinator that are guarded-call rows by the columns
above (never the approval, rejection, return and undo rows a manager writes,
`003.3`), with a NULL
`turn_id` and `created_at >= started_at` and `created_at <= finished_at` (both
bounds inclusive), one statement per table sets the turn id, but only when no
other turn T2 of the same coordinator overlaps that window, meaning
`T2.started_at <= this.finished_at AND (T2.finished_at IS NULL OR T2.finished_at
>= this.started_at)` (an overlap, including a shared boundary instant, leaves
NULL, never a guess). An older unfinished row of the coordinator, or a
concurrent dream session of the same coordinator, counts as an overlap until it
completes or the 24-hour settle closes it, so one lost completion leaves repair
disabled for that coordinator for up to a day; this is accepted because
leaving NULL is always safe. The repair runs before the verdict is computed. Approvals and undos run outside a session
and set nothing (`003.3`).

## Call digest

The guarded-call layer's decision point (allowed or refused) calls
`ledger.Call(sessionID, action, targetTaskID, allowed)`. It only appends to a
bounded in-process queue (1000 entries) drained by one writer goroutine; the
entry carries the enqueue time, and the writer resolves the turn at write time as
the coordinator's turn for that session with `started_at <=` enqueue time and
`finished_at` NULL or `>=` enqueue time, newest by `(started_at DESC, id DESC)`,
so a late write is never charged to the next turn. An entry that resolves to no
turn is parked, not retried inline: the writer moves it to a retry list with
its next attempt time (1 second later) and goes on to the next queue entry, so
a start race never stalls the queue. After three retries (four attempts in all, one second apart, as the completion re-enqueue of the outcome read does) it is dropped,
counting stage `call_unattributed`. A parked entry counts as settled for the
completion drain (the written-sequence advances when an entry is written,
dropped or parked); if it resolves after the turn's verdict was stored, its row
is still written but is absent from that verdict, as for any late call. The
writer is a single goroutine, so the count-then-insert of the 100-call cap
cannot race and the call row ids are the recording order; a full
queue drops the entry, counts stage `call_queue_full` and, when the session has an active
ledger turn in the map, sets that turn's `calls_truncated`. A call dropped after its retries
resolved no turn and cannot be marked; the verdict reads a turn as having an incomplete digest
when `calls_truncated` is set or an entry for its session is still on the retry list. The writer inserts one
row unless the turn already has 100, in which case it sets `calls_truncated` once
and drops the call. The action name is the tool's registered name; arguments and
results are never passed in (`001.4`). Guard code holds no dependency on the
recorder beyond this one function, which never returns an error to the guard.

## Failure isolation

Every recorder entry point runs in `ledger.safe(stage, fn)`: it recovers a
panic, logs, increments `coordinator_ledger_write_failed_total{stage}` with
`stage` in `start`, `call`, `complete`, `snapshot`, `link`, and returns nothing
to its caller. The event-bus subscription is asynchronous and the call hook only enqueues, so
no database write sits on the turn path, a guarded call or a decision (`001.6`).
The stage set is `start`, `call`, `complete`, `snapshot`, `link`, `call_queue_full`, `call_unattributed`, `settle`, `retention` and `model`.

## Board snapshot

Needs you and Queue are a client projection (`attention.ts`), so the snapshot does
not port them: it is a separate server query, `board.Snapshot(coordinatorID)`.
It selects the workspace's tasks that are not archived and not in a step that
completes tasks, filtered through the coordinator's watch set
([watch projects](watch-projects.md#the-filter)), ordered `(updated_at DESC,
task_id)`, capped at 200, and the coordinator's open proposals capped at 50
(oldest first, ties by id). Proposals created at or after the turn's `started_at` are left out of the open-proposal list and of the per-task kinds, so a turn's own proposals never appear in its snapshot; task rows reflect their state when the handler reads them. Per task it keeps task id, step id, state, the
task's last update time (null sorts last) and the kinds of the coordinator's
`pending` proposals that target it (each once, sorted ascending), only. Per
open proposal (status `pending`, `approving` or `failed`, the proposal store's
`openProposalStatuses`) it keeps id, kind, status and target task id (null when
none), only (`006.1`). A cap carries its own total: `tasks_total`,
`proposals_total`. The body is canonical JSON (keys sorted, arrays in projection
order), its SHA-256 is the hash, and `INSERT ... ON CONFLICT (hash) DO NOTHING`
stores it once, in the same transaction as the ledger row insert that first names it. Retention's
delete re-check runs inside its own batch transaction; the one window left, a delete committing between
the snapshot's conflict check and the ledger commit, leaves a row whose snapshot is gone and reads
as `no_snapshot`, never as an error. A snapshot capped by the item limit carries `"truncated":
true` and the total. It holds no title or text (`006.2`); a failed build leaves
`snapshot_hash` empty (`006.3`).

## Query tool

`list_coordinator_turns_kandev` is registered by the phase 3.1 tool profile
extension: the profile builder adds it to every profile when the flag is
effective, at conversation open (`004.1`). It is read-only and queries through
`ledger.Reader.List(coordinatorID, filter)`, which always binds
`coordinator_id` from the session, never from an argument (`004.2`). Arguments
are `since`, `verdict`, `trigger`, `task`, `limit` and `before`; the `task`
filter matches turns with a call row whose `target_task_id` equals it, and
`since` is inclusive on `started_at`. Tokens and cost are joined per `002.4`;
proposal ids are the turn's proposals ordered by `(created_at, id)`, the first 20. Validation
errors use the phase-1 validation error type naming the argument. `task`
outside the effective watch set returns the phase-1 not-found error before the
query (`004.4`); a digest entry outside it is returned with `target` omitted
after the query. A failed watch-set read returns the same unavailable error and no page. A read error returns the phase-1 unavailable error; the page is
built entirely in memory first so no partial page is sent (`004.5`).

## Gating

`features.coordinatorPhase31` is registered in the runtime flag registry with
`RestartRequired`, off in every profile of `profiles.yaml`, and effective only
when `features.coordinator`, `coordinatorPhase2` and `coordinatorPhase3` are
effective too (`005.1`). Recording (recorder, calls, snapshots, the outcome
grader, the retention job) starts under `coordinator` and `coordinatorPhase2`
alone (`005.2`). Everything read side registers only when the flag is
effective: the tool, HTTP routes (which answer 404 otherwise through the
router's absent-route path), screens, and the Learning section, Projects and
Pause controls. The Pause gate and project filter enforce stored state whatever
the flag (see the ADR's flag boundary). A flag-off boot therefore differs from phase 3 only by extra write-side
tables, columns and observers.

## Retention

A daily job, also run at start, first settles rows unfinished for more than 24
hours (`UPDATE ... SET finished_at = started_at, outcome = 'interrupted', verdict
= 'blocked' WHERE finished_at IS NULL AND started_at < ?`, batched like the
deletes; `001.7`), then deletes in batches of 500, each batch its own
transaction: ledger rows older than 400 days with their call rows
(`DELETE ... WHERE turn_id IN (batch)` first), and snapshots that no ledger row
started within the last 90 days references (the delete statement re-checks that
no such row references the hash, so it cannot remove a snapshot a turn started
a moment ago is using; a row whose snapshot is gone reads as `no_snapshot`,
never as an error; snapshots are shared by content
hash and an existing hash is never refreshed by the insert, so age is judged by
its newest referencing turn, never by the snapshot's own `created_at`; a quiet,
unchanged board keeps its one snapshot while recent turns use it). A turn older
than 90 days can therefore lose its snapshot and replay skips it `no_snapshot`. Coordinator deletion deletes its rows in the same transaction as the
coordinator (`005.3`): the ledger tables join `deleteCoordinatorPhase3Rows`, which
scopes by `coordinator_id` alone (`coordinator_turns` has no `workspace_id`;
call rows are deleted with `turn_id IN (SELECT id FROM coordinator_turns WHERE
coordinator_id <scope>)`), so both `DeleteCoordinator` and `DeleteWorkspaceState`
cover them. This delete is not batched; the 500-row batching applies to the age
retention only. Deleting ledger rows leaves any proposal or activity `turn_id`
and unattended `ledger_turn_id` pointing at a missing row, which readers treat
as unknown (`003.3`). A batch that fails is logged and the job ends; the next
run resumes since deletion is keyed by age.

## Migration

Additive migrations follow the existing `store_phase2_schema.go` pattern: idempotent
`CREATE TABLE IF NOT EXISTS`, and `ALTER TABLE ... ADD COLUMN` guarded by a
column-existence probe that works on SQLite and PostgreSQL (`005.4`). A
migration test upgrades a phase 3 database with rows and reads them back.

## Error handling

| Failure | Behaviour |
| --- | --- |
| Insert fails at start | Logged, counted, turn continues; completion inserts a late row |
| Duplicate start | No-op |
| Completion for a completed row | No-op |
| Snapshot build fails | Empty hash, row written |
| Tool ledger read fails | Phase-1 unavailable error, no partial page |
| `ActiveTurnID` fails | Empty turn id, write proceeds |

## Testing

Verdict table (each precedence pair), redelivery and concurrent completion on a
real SQLite store and a PostgreSQL-backed store, the 100-call cap, snapshot
hash stability, the flag-off boot (no tool, 404 routes, rows still written), the
migration upgrade, retention batch boundaries, and the unattended-row link
uniqueness.
