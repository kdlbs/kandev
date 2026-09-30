---
id: coordinator-pause-design
title: Pausing a coordinator design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
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
final state is what the commit order gives, never an error. A successful change publishes `coordinator.updated`; a no-op
publishes nothing.

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

## The precondition

`pause.Gate.Active(ctx, coordinatorID)` reads the state and returns
`(paused bool, err error)`. It is called at exactly three points, each before
anything else that has an effect:

1. **Wake delivery:** in the delivery loop that calls `Admit` for a coordinator's
   pending wakes. When paused, delivery stops for that coordinator in that
   pass, before `Admit`, so no admission check, cooldown timestamp or
   `coordinator_unattended_turns` row is created or consumed. The state is read
   again, and the turn row reserved, inside one `withCoordinatorLock` section, and
   Pause commits under the same lock: a turn is either reserved before the pause
   commit, in which case `Stopper.Stop` (run after the commit) finds and stops
   it, or refused. No turn starts after the pause commits.
2. **Dream trigger:** first condition of `Scheduler.Tick`
   ([shadow dream](shadow-dream.md#trigger-and-lease)).
3. **Automatic approval:** first check of `TryAutomaticApproval`.

The wake recorder and the backstop keep recording wakes and supersede none
because of Pause (`002.1`): Pause adds no code to the recorder, and the only
addition to the backstop pass is the `Stopper.Stop` call for paused coordinators
below. A read error is a
paused answer at every one of the three points, logged with the coordinator id
(`002.5`), with one carve-out that keeps a flag-off boot identical to phase 3
([turn ledger gating](turn-ledger.md#gating)): while the phase 3.1 flag is not
effective, a read error is a paused answer only for a coordinator the process
knows to be paused (an in-memory set filled at boot by
`SELECT id FROM coordinators WHERE paused_at IS NOT NULL` and updated on every
committed pause or resume); a failed boot query with the flag off leaves the set
empty and is logged. With the flag effective the read error is always a paused
answer. A pending wake that waits during pause keeps its `pending` status
and its queue position; Pause adds no expiry to it.

### Ordering and ties

Nothing new is ordered. Wake delivery order after Resume is the ordinary
admission order (oldest pending first, ties by id), so Resume replays the
queue exactly as if the pass had run at that time (`001.3`).

## Resume

Resume clears the state and publishes `coordinator.updated`. Nothing else runs
on Resume: the next ordinary delivery pass admits pending wakes through the
usual eight checks, the five-minute cooldown included, and none is discarded
(`001.3`). No wake is delivered by the request itself, so a burst of pending
wakes is delivered at the pace the phase 3 admission already limits.

## Pause and the running turn

On a successful pause, `pause.Stopper.Stop(coordinatorID)` runs after the
state commit, and again every backstop pass for every paused coordinator. It
reads the coordinator's open unattended-turn rows (`outcome IS NULL`) and acts by
the row's binding columns, the only state that tells the stages apart:

| Row | Meaning | Stop does |
| --- | --- | --- |
| `session_turn_id` set | running | Sets `stop_requested_at` (conditional, once), cancels the session turn through the `CancelTurn` path the spend ceiling uses ([spend](spend.md#req-coordinator-spend-003-stopping-at-the-ceiling)), settles the row `stopped_by_pause` with `settleOpenTurn`, and returns the turn's wakes (`coordinator_wakes.turn_id` equal to the row id) to `pending` with kinds and created times unchanged (`002.2`) |
| both bindings NULL | started, maybe not yet sent | Sets `stop_requested_at` and does nothing else until the row is 2 minutes old; then `settleUnsentTurn(id, 'stopped_by_pause')`, which returns its wakes to `pending` in the same transaction and matches only while `session_turn_id IS NULL`, so it loses to a binding that arrived first |
| `reserved_turn_id` set, `session_turn_id` NULL | send in flight | Sets `stop_requested_at` only; the next pass sees the row bound (running, cancelled as above) or settled |

`stop_requested_at` is the column the ceiling stop already uses; setting it never
settles anything. The accepted-turn binding writer (`OnAccepted`) reads it after
its own conditional write and, when set on this row, cancels the session turn
immediately instead of waiting for the next pass. A send that lands after the
2-minute settle of a not-sent row cannot bind (`bindAcceptedTurn` matches only
open rows); that turn is bounded by the spend ceiling and the ledger records it
as a `message` turn. Two minutes exceeds the orchestrator's dispatch time, so it
is a residual, counted `coordinator_pause_late_send_total`, not a designed path.
The ledger's completion maps the outcome to verdict `blocked`
([turn ledger](turn-ledger.md#completion-and-verdict)). Stop is idempotent: every
statement is conditional on the row still being open.

A manager's own attended turn is never stopped (`002.4`).

## Pause and the episode

If a dream is `running` for the coordinator, the same `Stopper` cancels its
episode session and sets the dream row `failed` with the reason `paused`, in a
conditional update `WHERE status = 'running'` so it cannot overwrite a row the
episode already finished (`002.2`). The window is unchanged, so the next dream
after Resume reads the same evidence.

## Automatic approval

While paused, `TryAutomaticApproval` returns "not eligible" before it counts
anything: the proposal stays `pending` with the note "Paused; a manager will
decide", and the ten-per-24-hours counter is not read or advanced (`002.3`). A
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

The control acts without a save bar. It is disabled while the request is in
flight; on failure it shows the error inline and keeps the previous state; every
open view updates from `coordinator.updated` (`003.3`). On a phone it is a
full-width button with a touch target of at least 44 px at the end of the strip's
second line (`003.4`). Copy is in six locales; no em dash.

## Error handling

| Failure | Behaviour |
| --- | --- |
| State read fails | Paused for all act-on-its-own points, logged |
| Stop fails | State stays paused; failure logged; the turn is stopped by the next ceiling or completion; the next delivery pass still refuses to start another |
| Concurrent pause and resume | Commit order decides |

A Stop that fails leaves a turn running after the state says paused. It does not
start any new one, and the failure is visible in the log; the backstop pass calls
`Stopper.Stop` for every paused coordinator each minute, a no-op when nothing
runs, so a running turn or dream stops within one minute at most (`002.2`).

## Testing

Pause and resume matrix (idempotence, reader 403, coordinator principal 403,
autonomy off), the three call points with a paused and a failing read, a
running turn stopped with its wakes back to `pending`, the dream cancelled with
`paused`, an attended message under Pause, Resume through the cooldown, and
the strip in each of its four states on desktop and phone.
