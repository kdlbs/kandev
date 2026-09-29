---
id: "05-delivery-unattended-turn"
title: "Admission, delivery and the unattended turn"
status: pending
wave: 3
depends_on:
  - "02-containment"
  - "03-spend"
  - "04-wake-recorder-backstop"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-WAKE-003
  - REQ-COORDINATOR-WAKE-004
  - REQ-COORDINATOR-WAKE-005
  - REQ-COORDINATOR-INTEGRATION-002
  - REQ-COORDINATOR-INTEGRATION-003
  - REQ-COORDINATOR-INTEGRATION-004
  - REQ-COORDINATOR-INTEGRATION-005
acceptance_criteria:
  - AC-COORDINATOR-WAKE-003.1
  - AC-COORDINATOR-WAKE-003.2
  - AC-COORDINATOR-WAKE-003.3
  - AC-COORDINATOR-WAKE-004.2
  - AC-COORDINATOR-WAKE-005.1
  - AC-COORDINATOR-WAKE-005.2
  - AC-COORDINATOR-WAKE-005.3
  - AC-COORDINATOR-WAKE-005.4
  - AC-COORDINATOR-WAKE-005.6
  - AC-COORDINATOR-INTEGRATION-002.2
  - AC-COORDINATOR-INTEGRATION-002.4
  - AC-COORDINATOR-INTEGRATION-003.1
  - AC-COORDINATOR-INTEGRATION-003.2
  - AC-COORDINATOR-INTEGRATION-004.1
  - AC-COORDINATOR-INTEGRATION-005.1
  - AC-COORDINATOR-INTEGRATION-005.2
  - AC-COORDINATOR-INTEGRATION-005.3
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/wake-recovery.md
  - ../../specs/coordinator/system-design/wake-screens.md
  - ../../specs/coordinator/system-design/containment.md
  - ../../specs/coordinator/system-design/spend.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 05: Admission, Delivery And The Unattended Turn (WP-11)

## Summary

Delivers pending wakes as at most one unattended turn per coordinator into its
existing conversation, after the eight ordered admission checks, and settles
the turn when the session turn its message started completes. No path
creates, archives or repoints a conversation.

## In scope

- `internal/coordinator/admission.go`: `Admit(ctx, coordinatorID, mode)` with
  the eight checks in order and two modes (`AdmitCounting` calls task 02's
  `CheckForAdmission` and counts held reasons; `AdmitReadOnly`, used by the
  autonomy read, calls `Check` and writes and counts nothing), calling task
  03's `Spend`, through two coordinator-side interfaces (a session snapshot
  reader and a message finder). Check 7 reads the queue through
  `messagequeue.Service.HasPendingForSession` (error-returning, any entry
  counts; a read error is a hold with detail `read_error`), and also holds
  while the coordinator has a turn row with `outcome IS NULL` (detail
  `turn_open`)
  and with a `CREATED` primary session held as `conversation_unavailable`
  (detail `session_not_started`)
  ([Admission](../../specs/coordinator/system-design/wake.md#admission)).
- `internal/coordinator/delivery.go`: `Deliver` under a keyed mutex, `Kick`
  on a coalescing one-goroutine-per-coordinator worker (Start/Stop, goleak; a
  kick during a run re-runs once; no timer for a hold) the per-kind episode re-check of every
  pending wake at the step 2 read
  ([Episode recheck](../../specs/coordinator/system-design/wake.md#episode-recheck))
  and the 20-wake batch, the step 3 transaction under `WithWakeLock` (re-read
  autonomy, insert the turn row, mark wakes, roll back when none changed),
  the send through a new exported orchestrator entry point beside `PromptTask`
  (it stores the message under a generated UUID id in the
  `afterDispatchAdmission` seam with `CreateUserMessageIdempotent`,
  `metadata.coordinator_wake_turn_id`, the claimed turn id, author type
  `user`; never the message queue), the sent test (a send counts as sent only
  when `onAccepted` bound `session_turn_id`; a stored message with no binding
  was not sent and is never re-sent), and the settle rule: `send_failed` for
  any refusal before dispatch (identified by the wrapper sentinel
  `ErrWakePromptNotDispatched`, not an enumerated list) and for a store
  failure of that seam, each returning the wakes to `pending` with `turn_id`
  null, marking the orphan message best effort, and conditional on
  `outcome IS NULL AND session_turn_id IS NULL`
  ([Delivery](../../specs/coordinator/system-design/wake.md#delivery),
  [Sent test](../../specs/coordinator/system-design/wake-recovery.md#sent-test),
  [Settle rule](../../specs/coordinator/system-design/wake-recovery.md#settle-rule)).
- The delivery worker lifecycle: `svc.StopDelivery()` joins the worker in the
  existing phase 3 cleanup closure of `registerCoordinatorRoutes`, and
  `registerCoordinatorDelivery` wires the worker, subscribers and
  `SetBackstopHooks` in its synchronous body and the startup pass in its
  returned hook.
- `internal/coordinator/turns.go`: `onAccepted` as the only writer of
  `session_turn_id`, turn end on `turn.completed` for that turn id, the
  backstop's missed-settle and unbound-row rules, the startup pass
  ([Startup pass](../../specs/coordinator/system-design/wake-recovery.md#startup-pass))
  and cost via task 03's `TurnCost`
  ([Turn end](../../specs/coordinator/system-design/wake.md#turn-end)).
- `Spend`'s own diagnostic counter and log are exempt from the `AdmitReadOnly`
  no-write rule: they move under `AdmitReadOnly` too, because they are not
  check state.
- The backstop hooks of task 04: the step 2 turn duties (message recovery,
  missed-settle re-derivation, per-turn cost recompute, and `CheckCeiling`,
  which run whatever `autonomy_enabled` reads) and the step 3.2 `Deliver`.
  The turn row's `start_ceiling_subcents` is written at the step 3 insert. Kick triggers from wake insert, conversation idle, the autonomy
  and ceiling PATCH, and turn end.
- Step 2 reads `EffectiveWatchSet` once per delivery and supersedes a wake
  whose task is outside it; a failed read leaves the wakes `pending` and
  delivers none
  ([Watch set](../../specs/coordinator/system-design/integration.md#watch-set)).
  Delivery passes no tool, binding or allowlist option to `PromptTask` and
  the message names no tool
  ([Tool list](../../specs/coordinator/system-design/integration.md#tool-list)).
- `currentUnattendedTurn` and the unattended stamp on the rows the coordinator
  principal writes (the propose handlers and `RecordRefusal`), and the
  refusal coalescing key including `unattended_turn_id`
  ([Log rows](../../specs/coordinator/system-design/integration.md#log-rows)).
  The column, read columns and values come from task 01; task 05 adds the
  `InsertActivity` write of `unattended_turn_id`; the copy is task 08's. The
  automatic-approval stamp is task 09's.
- Instructions, orders and goal: delivery adds none to the wake message;
  `MarkApplied` stays where phase 2 runs it
  ([Instructions](../../specs/coordinator/system-design/integration.md#instructions-orders-and-goal)).
- `coordinator.updated` with `autonomy_changed: true` from each delivery and
  turn settle that changed a row, and the counters
  `coordinator_wake_delivered_total`, `coordinator_wake_superseded_total`,
  `coordinator_unattended_turn_total{outcome}` and
  `coordinator_admission_held_total{reason}`, each owned by the step that
  changes the row ([Observability](../../specs/coordinator/system-design/wake.md#observability)).
- A turn row with a null `session_turn_id` stamps no log row
  ([Log rows](../../specs/coordinator/system-design/integration.md#log-rows)).
- The turn message text ([Transcript](../../specs/coordinator/system-design/wake-screens.md#transcript)),
  agent-facing, not localized.
- `no_turn_start_test.go`: add the wake delivery as the one allowed non-manager
  turn start (amended `AC-COORDINATOR-COPILOT-002.1`) and add every other
  phase 3 path this work order or an earlier one owns (recorder, backstop,
  relay read) to `noTurnStartPaths`. Reply delivery is a manager's message
  and is covered by task 08; the automatic approval row is added by task 09
  and the improvement approve and apply rows by task 10, each with its own
  code. The table also gains a row for the passive resume block
  `autoResumeBlockedCoordinatorMessageOnly`
  (`orchestrator/task_operations.go`, `autoResumeEligibility`), which stays
  unchanged: delivery is a direct `PromptTask` and never a passive resume.

## Out of scope

- Screens and the transcript rendering (task 06).

## Acceptance

- Each admission reason is produced by its condition and checks stop at the
  first failure in order; busy (starting, running, pending action, queued
  message) holds and is retried on idle or the next tick; no conversation or an
  ended session holds until a manager opens a usable one; no delivery path
  calls the conversation open, archive, the automation runner or the Office
  wakeup services (a fake of each fails the test if called).
- Concurrent `Deliver` from events, the backstop and a PATCH yield one open
  turn (unique index), 20 wakes at most, oldest first, ended conditions
  `superseded`, and no turn when none holds.
- An autonomy-off PATCH committed between `Admit` and step 3 makes step 3
  roll back with no turn row and no wake `delivered`; one committed after
  step 3 leaves the turn running (`AC-COORDINATOR-WAKE-004.3`, owned by task
  01).
- Autonomy turned off while a turn is open, with every event and observer
  call suppressed (`synctest`). The backstop still records the missing
  `message_id` and settles a missed `turn.completed`. A usage row that brings
  spend to the ceiling is stopped within one tick. A stop whose cancel keeps
  failing reaches `stop_state: "stop_failing"` after five minutes. With the
  ceiling then cleared, the check uses `start_ceiling_subcents`. No wake is
  recorded and no turn is delivered for that coordinator.
- More than 50 pending wakes with ended conditions are all `superseded` in
  one delivery; a stall wake whose task shows activity after the stall is
  `superseded` and not delivered (`AC-COORDINATOR-WAKE-005.6`).
- Episode re-check per kind: a question wake whose `pending_id` was answered
  and replaced by a new pending question, an error wake whose error was
  replaced by a new one with a different `stamp`, and a stall wake whose row
  has a newer `last_event_at` are each `superseded`, not delivered; a
  completed wake of a task no longer `COMPLETED` is `superseded`; a read
  error leaves the wake `pending` and undelivered. A condition that ends
  after the step 2 read is still delivered.
- A manager message that makes the session `RUNNING` between admission and
  the send: `PromptTask` returns `ErrAgentPromptInProgress`, nothing is
  queued, the turn is `send_failed`, its wakes are `pending`, and the next
  delivery holds with `conversation_busy` until the session is idle
  (`AC-COORDINATOR-WAKE-005.3`).
- A refused send and a crash after marking with no stored message return the
  wakes to `pending` and record `send_failed` or `interrupted`. Three tests:
  a bound `session_turn_id` means sent (the turn continues and is not sent
  again); a stored message with no binding is settled `send_failed` by the
  backstop after two minutes with the session not `RUNNING`, its wakes are
  `pending`, its message is marked orphaned, and nothing is re-sent; a restart
  during the gap between storing and binding settles the row `interrupted`
  at startup, also with no re-send.
- The session goes `WAITING_FOR_INPUT` and immediately runs a drained queued
  manager message while the `turn.completed` settle is suppressed: the
  backstop settles the unattended row `completed` because the active turn
  differs, and the manager's turn is never treated as unattended.
- A turn uses the phase 1 tool surface and policy: the guard table test runs
  against a delivery-started session and asserts the same refusals and the
  same bound tool list as an attended session, and that delivery, admission,
  containment and the permission denial add no tool and approve no request the
  allowlist left open (`AC-COORDINATOR-INTEGRATION-003.1`, `003.2`). During
  the turn, a read or proposal naming an unwatched workflow or task is
  answered as in an attended turn (`AC-COORDINATOR-INTEGRATION-002.4`).
- A wake whose task left the watch set after it was stored is `superseded` at
  the next delivery; a failed watch read leaves it `pending`
  (`AC-COORDINATOR-INTEGRATION-002.2`).
- A proposal and a refusal written while the turn is open carry its `unattended_turn_id`; the same writes by a manager's
  request while it is open, and any write outside a turn, carry none; a
  failed turn read stamps nothing (the automatic-approval row is covered by
  task 09); two refusals coalesce only when their turn
  ids match (`AC-COORDINATOR-INTEGRATION-004.1`).
- The turn carries the session's instructions with no second copy in the wake
  message; a proposal citing a standing order marks it applied in its own
  transaction and a turn that proposes nothing marks none; after a
  conversation reset the next turn is held `no_conversation` until a manager
  opens the copilot (`AC-COORDINATOR-INTEGRATION-005.1` to `005.3`).
- A `CREATED` primary session is held `conversation_unavailable` with detail
  `session_not_started`, and a store failure at the dispatch boundary
  settles `send_failed` with the wakes back to `pending`.
- A stored message whose reserved turn was rolled back on a pre-acceptance
  dispatch failure is not sent: the turn settles `send_failed` (`interrupted`
  at startup), its wakes are `pending`, and the next delivery sends a new
  message under a new turn id; a failed message or session read leaves the
  row untouched that tick.
- `Admit` in `AdmitReadOnly` mode moves no counter and writes no log; check 7
  holds for a queued message (read error: `read_error`) and for an open turn
  row (`turn_open`); check 8 uses the newest `finished_at`, tie broken by id
  descending.
- A duplicate or losing settle changes no row and so publishes no
  `autonomy_changed`, counts no outcome and kicks no delivery.
- A `WAITING_FOR_INPUT` session with no live execution gets the same result
  from delivery as from a manager's message through `PromptTask` (a resumed
  turn, or a refusal that returns the wakes to `pending`).

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Admit|Deliver|Turn|NoTurnStart' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'Deliver' -race -count=1
make -C apps/backend lint
```

## Risks

- Reading "queued message" must use the orchestrator's queue read
  (`HasPendingForSession`), not a message-table heuristic or `GetStatus`
  (which swallows errors); a wrong read sends into a busy session.
- A stored message is not proof of a send: the orchestrator rolls back the
  reserved turn on a pre-acceptance failure and leaves the message. Only the
  `onAccepted` binding is proof; recovery must never re-send an unbound row.
