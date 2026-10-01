---
id: coordinator-pause-design
title: Pausing a coordinator design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-10-01
requirements:
  - REQ-COORDINATOR-PAUSE-001
  - REQ-COORDINATOR-PAUSE-002
  - REQ-COORDINATOR-PAUSE-003
---

# Pausing a coordinator System Design

## Purpose and boundaries

Pause is one stored state and one precondition in front of the phase 3
decision points that let the coordinator act on its own. It does not renumber
or edit the eight admission checks of the wake design: it runs before `Admit`
and before the other two act-on-its-own points (the dream trigger and the
automatic approval), and the wake design is unchanged. It never touches
settings, policy, Watches, the conversation, wakes, proposals or stored spend
(`002.6`). It is available to every coordinator, whether or not autonomy is on.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PAUSE-001` | [State](#state), [Routes](#routes), [Resume](#resume) |
| `REQ-COORDINATOR-PAUSE-002` | [The precondition](#the-precondition), [Pause and the running turn](#pause-and-the-running-turn), [Pause and the episode](#pause-and-the-episode), [Automatic approval](#automatic-approval) |
| `REQ-COORDINATOR-PAUSE-003` | [Screens](#screens) |

## State

The gate is compiled in and reads the stored state whatever the phase 3.1 flag:
turning the flag off while a coordinator is paused leaves it paused for every
act-on-its-own decision, and only the route and controls go (`002.7`). With
the flag never on the state is never set, so behaviour is phase 3's.

`coordinators` gains `paused_at` (nullable timestamp) and `paused_by`
(nullable user id). Paused means `paused_at IS NOT NULL`. The pair is deleted
with the coordinator. Pause is not part of `policy_revision`, and does not
archive the conversation (`002.6`).

Set and clear are single conditional statements:

- Pause: `UPDATE coordinators SET paused_at = ?, paused_by = ? WHERE id = ? AND
  paused_at IS NULL`. Zero rows changed means already paused: the request
  succeeds and changes nothing (`001.1`).
- Resume: `UPDATE ... SET paused_at = NULL, paused_by = NULL WHERE id = ? AND
  paused_at IS NOT NULL`, same no-op success.

Two managers racing commit in order; the last committed statement decides
(`001.2`). Each statement is conditional, so a Resume that commits before a Pause
on an unpaused coordinator changes nothing and the Pause then applies: the
final state is what the commit order gives, never an error. A successful change publishes `coordinator.updated` with `autonomy_changed: true`,
once, after the commit (a further owner in the list of
[wake screens](wake-screens.md#autonomy-read)); a no-op publishes nothing.

## Routes

`PUT /coordinators/:id/pause` with `{paused: bool}`. Manager only; a reader and
a coordinator principal on any transport (HTTP, or the coordinator's tool
surface, which has no such tool) are refused with 403 and change nothing
(`001.1`). The route exists only while the phase 3.1 flag is effective (404
otherwise). The response is the coordinator DTO with `paused` (bool),
`paused_at` and `paused_by` (`{id, name}` with the name read from the user
record at response time; `null` when the stored user id is empty, the user was
deleted or is not readable, and the screens then show "Paused" with no name). It is allowed while autonomy
is off (`001.2`).

Checks run in this order, and the first that fails answers and changes nothing:
the route is absent with the flag off (404); the coordinator id is unknown or
outside the caller's workspaces (404, so existence is not leaked); the caller is
not a manager (403); the body has no boolean `paused`, is not JSON, or is `{}`
(400; a missing field is never read as `false`, so it cannot resume by accident);
then the conditional statement of [State](#state). A database failure is a 500
with no event. A failure to publish `coordinator.updated` after the commit is
logged and the response is still the 200 of the committed state.

The response returns as soon as the statement has committed. After a committed
pause, `Stopper.Stop` runs in a goroutine owned by the pause service, with a
detached context bounded at 30 seconds, and the request never waits for it, so
the control is enabled again when the response arrives (`003.3`). A no-op
request (already paused) does not call `Stopper.Stop`; the backstop does. A
coordinator deleted while `Stop` runs ends the run quietly: nothing to read,
logged at info.

## The precondition

`pause.Gate.Active(ctx, coordinatorID)` reads the state and returns
`(paused bool, err error)`. On a read error it logs, with the coordinator id,
and returns `(true, err)` where the fail-closed rule below applies and
`(false, err)` for the flag-off carve-out of a coordinator not in the known-paused
set, so a caller decides on `paused` alone and never treats `err` as fatal. A
coordinator id with no row reads as not paused (nothing exists to act for); an
empty id is a read error. It is called at exactly three points, each before
anything else that has an effect:

1. **Wake delivery:** in `Service.Deliver`, immediately after the per-coordinator
   delivery lock is acquired and before `Admit`, therefore also before
   `holdingWakes` (the episode re-check that supersedes wakes), so nothing is
   superseded during a pause. When paused, delivery stops for that coordinator in that
   pass, before `Admit`, so no admission check, cooldown timestamp or
   `coordinator_unattended_turns` row is created or consumed. The state is read
   again, and the turn row reserved (the existing `Store.WithWakeLock` (defined in `store_wake.go`, used by
   `store_turns.go`), which locks the coordinator row as `withCoordinatorLock`
   does), and Pause commits under the same lock: a turn is either reserved before the pause
   commit, in which case `Stopper.Stop` (run after the commit) finds and stops
   it, or refused. No turn starts after the pause commits.
2. **Dream trigger:** first condition of `Scheduler.Tick`
   ([shadow dream](shadow-dream.md#trigger-and-lease)), and again after the lease
   is claimed and before the episode session is launched, so a Pause that
   commits between the two stops the start; work order 04 owns both calls and
   work order 05 provides the gate. A Pause that commits after the second read
   is caught by the `Stopper`, which cancels the running episode.
3. **Automatic approval:** in `TryAutomaticApproval` after the guards that decide
   the proposal is one the automatic class would handle (phase 3 on, kind
   `create_task`, no agent start, the class setting `automatic`) and before the
   raiser check, the claim and the ten-per-day counter. A proposal of any other
   kind or class never receives the paused note.

The wake recorder and the backstop keep recording wakes and supersede none
because of Pause (`002.1`): Pause adds no code to the recorder, and the only
addition to the backstop pass is the `Stopper.Stop` call for paused coordinators
below. A read error is a
paused answer at every one of the three points, logged with the coordinator id
(`002.5`); at point 3 a read error that `Gate.Active` answers as paused (`(true, err)`) gives the existing
unavailable note, not the paused note, because the coordinator may not be
paused; the flag-off carve-out answer `(false, err)` for a coordinator outside the known-paused set proceeds to the raiser check, claim and counter exactly as phase 3 does, with no note. There is one carve-out that keeps a flag-off boot identical to phase 3
([turn ledger gating](turn-ledger.md#gating)): while the phase 3.1 flag is not
effective, a read error is a paused answer only for a coordinator the process
knows to be paused (an in-memory set filled at boot by
`SELECT id FROM coordinators WHERE paused_at IS NOT NULL` and updated on every
committed pause or resume); a failed boot query with the flag off leaves the set
empty and is logged. With the flag effective the read error is always a paused
answer. The set is per process; with the flag off the backstop pass refreshes it
with the same query before its `Stopper.Stop` calls, so a pause or resume
committed by another process is picked up within a minute. A failed refresh is
logged and leaves the set as it was. A pending wake that waits during pause keeps its `pending` status
and its queue position; Pause adds no expiry to it.

### Ordering and ties

Nothing new is ordered, except that `Stopper.Stop` takes a coordinator's open
turn rows by `(started_at, id)` and the backstop takes paused coordinators by
`id`. Wake delivery order after Resume is the ordinary
admission order (oldest pending first, ties by id), so Resume replays the
queue exactly as if the pass had run at that time (`001.3`).

## Resume

Resume clears the state and publishes `coordinator.updated` with `autonomy_changed: true`. Nothing else runs
on Resume: the next ordinary delivery pass admits pending wakes through the
usual eight checks, the five-minute cooldown included, and Pause and Resume
discard none (`001.3`); turning autonomy off while paused still supersedes wakes, as it always did, and is not a Pause effect. That pass runs the ordinary episode re-check
(`holdingWakes`) first, which supersedes a pending wake whose task is no longer
owned or watched or whose episode no longer holds; after a long pause this can
supersede many wakes, for the phase 3 reason, and a test must not assert that
every pending wake of a paused period is delivered. No wake is delivered by the request itself, so a burst of pending
wakes is delivered at the pace the phase 3 admission already limits.

## Pause and the running turn

`stop_requested_at` stays the spend ceiling's marker and Pause never writes it:
`boundTurnOutcome` (`turn_end.go`) maps any `stop_requested_at` to
`stopped_at_ceiling`, and `checkCeiling` (`ceiling.go`) treats a marked row as a
ceiling stop already decided. Pause has its own nullable column,
`pause_requested_at`, on `coordinator_unattended_turns`, added by the schema
migration of this work order, and the two code paths change as follows.

- `boundTurnOutcome`: `stop_requested_at` set gives `stopped_at_ceiling`
  (unchanged, so a turn the ceiling already marked keeps that outcome); else
  `pause_cancel_at` set gives `stopped_by_pause` whatever the session state
  reads (`CancelTurn` leaves the session `WAITING_FOR_INPUT`, so the state can
  never tell a Pause stop from a normal end); else the session-state read as
  today. `pause_cancel_at` is a second nullable column of this work order, the
  cancel intent: only the code that is about to call `CancelTurn` for Pause
  writes it (the Stopper and the `OnAccepted` late-send path, below), always
  before the call and conditionally (`WHERE outcome IS NULL AND
  stop_requested_at IS NULL`), so the turn-end handler that runs when the cancel
  lands already sees it. `pause_requested_at` alone (a mark that cancelled
  nothing) never changes an outcome, and a turn that ends on its own after that
  mark is an ordinary `completed` turn whose wakes stay handled.
- `settleBoundTurn` (`turn_end.go`) stays the one settle path for a bound turn,
  with its cost recompute, counter, publish and kick. Its store call
  `Store.settleOpenTurn` (`store_turns.go`), when the outcome is
  `stopped_by_pause`, also returns the turn's wakes (`coordinator_wakes.turn_id`
  equal to the row id) to `pending` with `turn_id` cleared and kinds and created
  times unchanged, in the same transaction as the settle. The turn-end handler
  and the Stopper both call `settleBoundTurn`, so either may run first and the
  loser changes nothing; the outcome is the same either way because it comes
  from `pause_cancel_at`, not from timing (`002.2`). The ceiling keeps its own
  `settleTurnStoppedAtCeiling` (`store_ceiling.go`), which returns no wakes.
- `checkCeiling` is unchanged: it reads `stop_requested_at` only.

`pause.Stopper.Stop(coordinatorID)` runs after the state commit (from the route's
goroutine, [Routes](#routes)), and again every backstop pass for every paused
coordinator; the backstop lists those coordinators with `SELECT id FROM
coordinators WHERE paused_at IS NOT NULL ORDER BY id`, and when that query
fails it logs and skips the pass, the next one retrying. Two `Stop` calls for one
coordinator may overlap (the route's goroutine and the backstop). Cancel
ownership is single because `CancelTurn` claims the session's cancellation: the
loser gets `ErrCancelInFlight`, which is not an error either: it leaves
`pause_cancel_at` set, logs at info, and the next pass retries. Each row is
handled on its own: a failure on one row is logged and the loop goes on to the
later rows and then to the dream, and `Stop` returns the joined errors for the
log only. Before it writes `pause_cancel_at` for a row, and in the late-send
path below, `Stop` reads `Gate.Active` once more and does nothing for the row
when the coordinator is no longer paused (a Resume committed), so a delayed
`Stop` never cancels a turn on a resumed coordinator. The window between that
read and the cancel call is accepted: a turn cancelled in it settles
`stopped_by_pause` with its wakes `pending`, which Resume's ordinary delivery
redelivers. It reads the coordinator's open
unattended-turn rows (`outcome IS NULL`) ordered `(started_at, id)` and acts by
the row's binding columns, the only state that tells the stages apart:

| Row | Meaning | Stop does |
| --- | --- | --- |
| `session_turn_id` set | running | Sets `pause_requested_at` (conditional, `WHERE pause_requested_at IS NULL AND outcome IS NULL`) and `pause_cancel_at` (conditional as above; zero rows changed means the row settled or the ceiling marked it, and `Stop` then does not call `CancelTurn` for it, the ceiling's own path owning that cancel), cancels the session turn through the `CancelTurn` path the spend ceiling uses ([spend](spend.md#stopping)), then, after a successful cancel, re-reads the row by id and passes that fresh row to `settleBoundTurn` (so the struct it works on carries the written `pause_cancel_at`), which settles `stopped_by_pause` with the wakes back to `pending`. `ErrTurnNotActive` from `CancelTurn` is not an error: the turn is ending, either on its own or because a sibling `Stop` already cancelled it, so the Stopper settles nothing and never clears `pause_cancel_at`; the turn-end handler settles the row and, seeing `pause_cancel_at`, gives it `stopped_by_pause` with its wakes `pending`. A turn that was ending on its own when the cancel intent was written is therefore also labelled `stopped_by_pause`; that is accepted, because the wakes are redelivered after Resume and a wake is an invitation, not a record. Nothing ever unsets `pause_cancel_at`, so overlapping `Stop` calls cannot lose a cancel another call made. Any other cancel error is logged, leaves `pause_cancel_at` set and the next pass retries (`002.2`) |
| both bindings NULL | started, maybe not yet sent | Sets `pause_requested_at` and does nothing else until the row's `started_at` is at least 2 minutes before the store clock (`started_at <= now - 2 minutes`, inclusive; the age is of the row, not of the pause, so a pause that finds an old unbound row settles it at once); then `Store.settlePausedUnsentTurn(id, coordinatorID)` (new, `store_turns.go`), which sets `stopped_by_pause`, returns the row's wakes to `pending` in the same transaction and matches only `outcome IS NULL AND session_turn_id IS NULL AND reserved_turn_id IS NULL` and a coordinator row that still has `paused_at IS NOT NULL` (one statement, so it loses to a binding, a reservation or a Resume that arrived between the read and the settle; after a Resume the ordinary `recoverUnboundTurn` handles the row). A settle error is logged and the next pass retries. The existing `settleUnsentTurn` is not changed: `settleNotSent` keeps closing reserved rows as `send_failed` and `interrupted`, so a pause never leaves a row that blocks delivery |
| `reserved_turn_id` set, `session_turn_id` NULL | send in flight | Sets `pause_requested_at` only; the next pass sees the row bound (running, cancelled as above) or settled |

Setting `pause_requested_at` never settles anything. The accepted-turn binding
writer (the `OnAccepted` callback that `delivery.go` passes on the orchestrator's
`UnattendedWakePrompt`, which calls `bindAcceptedTurn`) reads the row after its
own conditional write and, when `pause_requested_at` is set on this row and
`Gate.Active` still reports the coordinator paused, runs the same per-row routine
as the Stopper's `running` row: it writes `pause_cancel_at` conditionally
before the cancel, cancels, and settles through `settleBoundTurn`, so the bound
row gets its `stopped_by_pause` outcome and its wakes return to `pending` like
any Stopper stop. `OnAccepted` runs inside the orchestrator's dispatch
admission and `CancelTurn` waits for the cancellation to finish, so the
callback never calls `CancelTurn` itself: it hands the routine to a goroutine
owned by the pause service (detached context bounded at 30 seconds, drained on
shutdown) and returns at once. A failure there leaves `pause_cancel_at` set
for the backstop's retry. A send that
lands after the 2-minute settle of a not-sent row cannot bind
(`bindAcceptedTurn` matches only open rows, and the spend ceiling's
`openCeilingTurn` matches the same open rows, so neither bounds it). When
`bindAcceptedTurn` matches no row, `OnAccepted` reads the coordinator's pause
state and, when it is paused, cancels the accepted session turn through
`CancelTurn` from the same service-owned goroutine (an `ErrTurnNotActive` result is ignored; a read error is paused, with the carve-out of [The precondition](#the-precondition)) and counts
`coordinator_pause_late_send_total`. A `bindAcceptedTurn` database error is logged and handled as no matching row, so the same pause read and cancel follow. Any other `CancelTurn` error, `ErrCancelInFlight` included, is logged and counted in `coordinator_pause_late_send_failed_total` and is not retried: no open row remains for the backstop to find, so that turn runs to its end as an ordinary `message` turn. This is an accepted residual of a path that is itself a residual; when it is not paused the turn runs as an
unbound turn of a coordinator that was resumed, which the ledger records as a
`message` turn. The cancelled late turn has no open unattended row to settle, so
it is an ordinary `message` turn in the ledger: outcome and verdict come from
the session state at completion by the ordinary precedence, and Pause does not
relabel it. Its wakes were already returned to `pending` by the settle of the
not-sent row. Two minutes exceeds the orchestrator's dispatch time, so the path
is a residual, not a designed one; task-05 tests that a late accepted send on a
paused coordinator is cancelled. The ledger's
completion maps a `stopped_by_pause` outcome to verdict `blocked`
([turn ledger](turn-ledger.md#completion-and-verdict)). Stop is idempotent: every
statement is conditional on the row still being open. In one backstop pass
`Stopper.Stop` runs before the not-sent recovery (`recoverUnboundTurn`), and both
settle conditionally on `outcome IS NULL`, so for one unsent row exactly one
wins and the other matches nothing; the winner only decides the label
(`stopped_by_pause` or `send_failed`), the wakes return to `pending` either way. If the ceiling and Pause
both mark one row, `stopped_at_ceiling` wins and the row's wakes follow the
phase 3 ceiling settle.

A manager's own attended turn is never stopped (`002.4`).

## Pause and the episode

If a dream is `running` for the coordinator, the same `Stopper` first re-reads
`Gate.Active` and does nothing when the coordinator is no longer paused (as for a
turn row, so a delayed `Stop` never cancels a dream started after Resume), then
sets the dream row `failed` with the reason `paused` in a conditional update
`WHERE status = 'running'`, and only then cancels the episode session. Marking
first makes the row's reason `paused` however the episode ends: the dream's
orchestrating goroutine writes its terminal status with the same
`WHERE status = 'running'` guard, finds zero rows changed and stops writing
([shadow dream](shadow-dream.md#trigger-and-lease)), so it cannot record
`bad_output` or a run error over `paused` (`002.2`). A row that already finished
is left as it is. A failed update leaves the row `running` and nothing is
cancelled; the next pass retries. A cancel that fails after the update is
not retried by the `Stopper`, which no longer finds a `running` row: the dream's
goroutine finds zero rows changed at its next refresh, at most one minute later,
and cancels the episode itself ([shadow dream](shadow-dream.md#trigger-and-lease)).
Work order 05 ships the `Stopper`'s registration point
for the dream canceller and tests it with a fake; the dream does not exist until
work order 04, whose tests verify the cancel and the `failed`/`paused` row end to
end. The window is unchanged, so the next dream
after Resume reads the same evidence.

## Automatic approval

While paused, `TryAutomaticApproval` returns, at the placement of [The
precondition](#the-precondition), `pendingResult(proposal, pausedNote)` (a
non-nil `AutomaticResult` with status `pending`, the existing helper
`TryAutomaticApproval` uses for its other holds), where today the earlier guards
return `nil, nil` and show no note: the proposal stays `pending` with the note
"Paused; a manager will decide", and the ten-per-24-hours counter is not read or
advanced (`002.3`). The note is derived on each call, nothing is stored, so a
repeated attempt gives the same result. Pause committing after the gate read
does not stop an approval already past it: that is accepted, one proposal at a
time and inside the ten-per-day limit. A
manager's manual approval is unaffected.

## Attended messages

The manager message path does not call `pause.Gate`, so a turn starts as it
does today, and the coordinator's proposals from it are created as usual
(`002.4`).

## Screens

The Pause state is part of the coordinator DTO whatever the flag, so with the
flag off (the state was set while it was on) the strip and the Autonomy section
show a read-only "Paused" badge with the note "Resume needs the phase 3.1
features to be on", and no control; the route and controls stay flag-gated.

The autonomy strip of Needs you ([wake screens](wake-screens.md)) gains a
fourth state. While the flag is effective and the coordinator is paused it shows
"Paused" with the manager's name and the time (relative and absolute in the
tooltip), keeps the pending count, and offers a manager **Resume**; when not
paused and autonomy is on it offers a manager **Pause**. A reader sees the state
without a control (`003.1`). The Autonomy section of the coordinator's settings
shows the state and the same control, also while autonomy is off, with the note
"A paused coordinator keeps its queue. Turning autonomy off does not." (`003.2`).

The strip keeps its rule of appearing only while autonomy is on; a paused
coordinator with autonomy off has no strip, and the Autonomy section carries the
state and the control. The strip has four states, Active, Active with a transient
text, Held with a reason, and Paused, which wins over the other three: line 1
reads "Autonomy: Paused by <name> . <time>" (without " by <name>" when the name
is null), "Last woke <age>" and the pending count stay.

The control acts without a save bar. The paused state reaches every view through
the autonomy read ([wake screens](wake-screens.md#autonomy-read)), which gains
`paused` (bool), `paused_at` (RFC 3339 UTC or `null`) and `paused_by`
(`{id, name}` or `null`, the name rule of [Routes](#routes)) whatever the phase
3.1 flag, so the flag-off badge has a source. `coordinator.updated` carries no
state: a client treats it as "re-read", and the strip's existing rule re-reads
on `autonomy_changed`, which Pause and Resume set. The control is disabled while
the request is in flight. When the request answers 200 the view applies the
response's coordinator as the state and also re-reads the autonomy read. The
ordering rule is on reads: every read carries the client's sequence number and
the response of the PUT counts as a read issued at the click; a read issued
after the click outranks the PUT response, and of two reads the later-issued
wins, so a stale PUT response or another manager's change never overwrites a
later read. On failure the control shows the error inline and the state of the
latest read that landed since the click, or the state held when the click
happened when none has (`003.3`). On a phone it is a
full-width button with a touch target of at least 44 px at the end of the strip's
second line (`003.4`). Copy is in six locales; no em dash.

## Error handling

| Failure | Behaviour |
| --- | --- |
| State read fails | Paused for all act-on-its-own points, logged; with the flag off, only for a coordinator in the known-paused set; automatic approval answers with the unavailable note (flag-on, or flag-off for a known-paused coordinator); flag-off for any other coordinator proceeds as phase 3 |
| Stop fails | State stays paused; failure logged; the next backstop pass retries the stop, and the turn can also end by the ceiling or on its own; the next delivery pass still refuses to start another. If a Resume commits before the retry, the backstop no longer calls `Stop` for that coordinator and the turn runs to its end; because `pause_cancel_at` stays set, the turn-end handler labels it `stopped_by_pause` (ledger verdict `blocked`) and returns its wakes to `pending`. This is accepted: a wake is an invitation, not a record, and the redelivered wakes meet the ordinary episode re-check, the five-minute cooldown and the spend ceiling, so the cost is bounded to one further turn per pass of admission |
| Concurrent pause and resume | Commit order decides |
| `CancelTurn` returns `ErrCancelInFlight` | `pause_cancel_at` stays set, logged at info, next pass retries |
| Backstop's paused-coordinator query fails | Logged, pass skipped, next pass retries; wake recording untouched |
| Resume commits during a stop | `Stop` re-reads the state before each write and stops; a cancel already issued settles `stopped_by_pause` with wakes `pending` |
| A failed cancel, then Resume | See Stop fails: the turn finishes on its own, settles `stopped_by_pause` and its wakes return to `pending` (accepted) |

A Stop that fails leaves a turn running after the state says paused. It does not
start any new one, and the failure is visible in the log; the backstop pass calls
`Stopper.Stop` for every paused coordinator each minute, a no-op when nothing
runs, so a running turn or dream stops within one minute at most (`002.2`).

## Testing

Pause and resume matrix (idempotence, reader 403, coordinator principal 403,
autonomy off), the three call points with a paused and a failing read, a
running turn stopped with its wakes back to `pending`, the dream cancelled with
`paused`, an attended message under Pause, Resume through the cooldown, and
the strip in each of its four states on desktop and phone, and the strip absent with autonomy off. Also: the route's 404/403/400 order and `{}` body, a delayed `Stop` after Resume cancelling nothing, `ErrCancelInFlight` retried, one failing row not skipping the next, a paused read in `OnAccepted` for a bound row (`pause_cancel_at` written, settle `stopped_by_pause`), `settlePausedUnsentTurn` losing to a Resume, the 2-minute boundary at exactly 2 minutes and at 1 minute 59 seconds, the automatic-approval note only on automatic-class proposals and the unavailable note on a read error, wakes superseded by the episode re-check only after Resume, and the flag-off badge with no control. Also: two overlapping `Stop` calls where the second gets `ErrTurnNotActive` (the turn still settles `stopped_by_pause` with wakes `pending`, `pause_cancel_at` never cleared), the flag-off read error at automatic approval for a known-paused and for another coordinator, a PUT response overtaken by a read issued after the click being discarded, a failed request showing the latest read, Pause and Resume publishing `autonomy_changed` so a second manager's strip updates, the autonomy read carrying `paused` with the flag off, a dream row ending `failed`/`paused` when the episode ends between mark and cancel, and a failed cancel followed by Resume ending `stopped_by_pause` with wakes `pending`, a late-send cancel failing once and not retried, and a delayed `Stop` after Resume cancelling no dream.
