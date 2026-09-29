---
id: coordinator-wake-backstop-design
title: Wake backstop design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-WAKE-002
---

# Wake backstop System Design

## Purpose and boundaries

This design owns the level-triggered backstop: its lifecycle, the duties it runs
for each coordinator it visits, and how it keeps stall records current. Split out of
[wake](wake.md) for size; the wake tables, the [wake lock](wake.md#wake-lock),
admission, delivery and turn end stay there, and recording stays in
[recording wakes](wake-recording.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-WAKE-002` | [Backstop](#backstop), [Lifecycle](#lifecycle), [Stall currency](#stall-currency) |

## Backstop

`internal/coordinator/wake_backstop.go` runs one goroutine with a 60-second
ticker. Its duties come in two groups. **Turn and setting duties** run for a
coordinator whatever its `autonomy_enabled` reads. **Wake duties** run only
while autonomy is on. Turning autonomy off therefore stops new wakes and
deliveries at once, but it never abandons a turn that is already open or an
undo lowering that is already owed (`AC-COORDINATOR-WAKE-004.3`).

### Lifecycle

The backstop is a `WakeBackstop` value owned by the `Service`, with
`Start(ctx)` and `Stop()` (the goroutine-ownership shape of
`internal/integrations/healthpoll`):

- `Start` is idempotent, creates a fresh cancellable context under the same
  mutex `Stop` takes, registers the goroutine on a `sync.WaitGroup`, and runs
  no pass at start: the first pass is 60 seconds after `Start`. `Stop` is
  idempotent, cancels the context and waits for the goroutine, including a pass
  in progress (every store call in a pass takes the pass context, so it
  returns promptly). `Stop` also latches the value closed: a `Start` after
  `Stop` is a no-op, so a shutdown that lands before the detached startup pass
  reaches `StartWakeBackstop` leaves no ticker behind. Tests use a fresh value.
- One goroutine runs passes serially, so two passes never overlap; a pass that
  takes longer than 60 seconds makes the ticker drop the missed ticks rather
  than queue them.
- A panic in a pass is recovered, logged at error and counted in
  `coordinator_backstop_skipped_total`; the loop continues.
- The wake registration (`registerCoordinatorWakeState` in
  `internal/backendapp/coordinator.go`) keeps `PruneWakeState` first, then
  calls `svc.StartWakeBackstop(ctx)`, so the backstop starts after the wake
  hook's own startup pass. The later startup hooks (delivery's settle of an
  interrupted turn) need no ordering against it: the first pass is 60 seconds
  after `Start`, and every settle both perform is conditional on the row's
  `outcome IS NULL` (and `message_id IS NULL` for the no-message settle), so
  whichever runs first wins and the other is a no-op.
- `registerCoordinatorRoutes` registers `svc.StopWakeBackstop` through
  `routeParams.addCleanup` when phase 3 is effective. Cleanups run in reverse
  registration order and the database pool's cleanup was registered earlier,
  so the backstop is joined before the store closes.
- The `Hooks` value carries the three later-task duties: `TurnDuties(ctx,
  coordinatorID)` (steps 2.1 to 2.3; one field, task 05 composes spend's
  `CheckCeiling` into it), `Lowering(ctx, coordinatorID)` (step 2.4, task 09)
  and `Deliver(ctx, coordinatorID)` (step 3.2, task 05).
  `Service.SetBackstopHooks` merges per field under a mutex (a non-nil field
  replaces, a nil field leaves the stored one) and is honoured only before
  `Start`. Every owner calls it in the synchronous body of its registration
  function, never in the hook that function returns (the returned hooks run
  after every body, and the wake hook's `Start` is one of them). A hook must
  return promptly once its `ctx` is cancelled, which bounds `Stop`; a later call logs at warn and changes nothing. A nil field is a
  no-op; a hook is called serially for one coordinator in the step order
  below; a hook error or panic is logged at warn, counted in
  `coordinator_backstop_skipped_total`, and does not skip the later duties or
  coordinators.

Each tick:

1. Builds the visit set, the union of three queries, deduplicated and ordered
   by coordinator id:
   - coordinators with `autonomy_enabled = 1`;
   - coordinators with a `coordinator_unattended_turns` row that has
     `outcome IS NULL`, or `finished_at` in the last 11 minutes;
   - coordinators with at least one `coordinator_proposals` row with
     `claimed_automatically = 1` ([automatic](automatic.md#lowering)).

   A query that fails logs at warn and increments
   `coordinator_backstop_skipped_total`, and the tick continues with the
   other queries. A coordinator deleted between the list and its visit is
   skipped.
2. For each coordinator in the visit set, re-reads the coordinator row and
   runs the turn and setting duties in this order:
   1. message recovery for its open turn with `message_id` null
      ([Finding the turn's message](wake.md#finding-the-turns-message));
   2. missed-settle re-derivation for its open turn ([Turn end](wake.md#turn-end)),
      only when the row's `session_turn_id` is non-null (a row with it null is
      left to step 1 and the two-minute `send_failed` rule),
      then the per-turn cost recompute for its turns settled in the last 11
      minutes ([spend](spend.md#per-turn-cost));
   3. the ceiling check `CheckCeiling` of [spend](spend.md#stopping) for its
      open turn, if any, including a row with `stop_requested_at` set;
   4. the lower-on-undo retry of [automatic](automatic.md#lowering).
   5. the re-resolution of recorded unattended denials,
      `ReresolveRecordedDenials(ctx, coordinatorID)` of
      [containment](containment.md#unattended-permissions), called for this
      coordinator only, whatever `autonomy_enabled` reads. A denial whose turn
      has settled stays as recorded and is never re-resolved; a coordinator
      that leaves the visit set is picked up on its next visit.

   A read or write error in one duty logs at warn, increments
   `coordinator_backstop_skipped_total`, and does not skip the later duties
   or coordinators.
3. Only when the re-read row has `autonomy_enabled = 1`, runs the wake duties:
   1. **Reads first, records after**, as
      [Backstop recording](wake-recording.md#backstop-recording) states:
      every own task's current episodes are read from stored state
      (`ListOwnTasks`, the watch set, `WakeSources`, `GetStall`), and only when
      every read succeeded does it `RecordWake` each one. A read error
      abandons this coordinator's wake duties for the pass before anything is
      recorded, skips step 3.2 for it, logs at warn, increments
      `coordinator_backstop_skipped_total` and moves on
      (`AC-COORDINATOR-WAKE-002.3`).
   2. Calls `Deliver(coordinatorID)` through `Hooks`.

With autonomy off, an open turn therefore keeps the 60-second ceiling bound of
`AC-COORDINATOR-SPEND-003.2`, the `stop_failing` state of
`AC-COORDINATOR-SPEND-003.4`, and the recovery of a missed settle. No wake is
recorded for it and nothing is delivered after it ends. Once its turns are
settled and past the 11-minute recompute, and it has no automatic claim, the
coordinator leaves the visit set.

### Stall currency

A `coordinator_stalls` row is never deleted when its task resumes; it stays
until the 30-day prune. Every reader in this design (recorder, backstop and
delivery step 2) therefore treats a stall row as a condition only while it is
current: the task's persisted `TaskStatusSummary.last_activity_at` is absent
or not later than the row's `detected_at`; a failing `LastActivityAt` read is
a read error, not an absent value. This is the test Needs you applies
(`AC-COORDINATOR-NEEDS-YOU-001.2`, [needs-you design](needs-you.md)). A task
that resumes before delivery makes its stall wake's condition end, and
delivery supersedes it (`AC-COORDINATOR-WAKE-005.6`). A later stall upserts a
new `last_event_at`, which is a new episode key.

The recorder and the backstop call the same `RecordWake` with the same keys,
which is why a restart, a redelivered event and a backstop pass converge on
one row (`AC-COORDINATOR-WAKE-001.2`). `task.stalled` is emitted again for a
still-stalled task after a restart; the stall row's `last_event_at` does not
change, so its key does not either.
