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
(`001.2`). A successful change publishes `coordinator.updated`; a no-op
publishes nothing.

## Routes

`PUT /coordinators/:id/pause` with `{paused: bool}`. Manager only; a reader and
a coordinator principal on any transport (HTTP, or the coordinator's tool
surface, which has no such tool) are refused with 403 and change nothing
(`001.1`). The route exists only while the phase 3.1 flag is effective (404
otherwise). The response is the coordinator DTO with `paused` (bool),
`paused_at` and `paused_by` (id and display name). It is allowed while autonomy
is off (`001.2`).

## The precondition

`pause.Gate.Active(ctx, coordinatorID)` reads the state and returns
`(paused bool, err error)`. It is called at exactly three points, each before
anything else that has an effect:

1. **Wake delivery:** in the delivery loop that calls `Admit` for a coordinator's
   pending wakes. When paused, delivery stops for that coordinator in that
   pass, before `Admit`, so no admission check, cooldown timestamp or
   `coordinator_unattended_turns` row is created or consumed.
2. **Dream trigger:** first condition of `Scheduler.Tick`
   ([shadow dream](shadow-dream.md#trigger-and-lease)).
3. **Automatic approval:** first check of `TryAutomaticApproval`.

The wake recorder and the backstop keep recording wakes and supersede none
because of Pause (`002.1`): Pause adds no code to them. A read error is a
paused answer at every one of the three points, logged with the coordinator id
(`002.5`). A pending wake that waits during pause keeps its `pending` status
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
state commit: if the coordinator has a running unattended turn, it cancels the
session turn through the same `CancelTurn` path the spend ceiling uses
([spend](spend.md#req-coordinator-spend-003-stopping-at-the-ceiling)), settles the
unattended-turn row with outcome `stopped_by_pause`, and returns the turn's
wakes to `pending` with their kinds and created times unchanged (`002.2`). The
ledger's completion maps the outcome to verdict `blocked`
([turn ledger](turn-ledger.md#completion-and-verdict)). Stop is idempotent: a
second call finds no running turn.

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
start any new one, and the failure is visible in the log; a follow-up pass
retries the stop while the state is paused and a running turn exists (the
backstop calls `Stopper.Stop` for paused coordinators, which is a no-op when
nothing runs).

## Testing

Pause and resume matrix (idempotence, reader 403, coordinator principal 403,
autonomy off), the three call points with a paused and a failing read, a
running turn stopped with its wakes back to `pending`, the dream cancelled with
`paused`, an attended message under Pause, Resume through the cooldown, and
the strip in each of its four states on desktop and phone.
