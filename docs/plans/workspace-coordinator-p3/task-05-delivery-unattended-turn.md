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
acceptance_criteria:
  - AC-COORDINATOR-WAKE-003.1
  - AC-COORDINATOR-WAKE-003.2
  - AC-COORDINATOR-WAKE-003.3
  - AC-COORDINATOR-WAKE-004.2
  - AC-COORDINATOR-WAKE-005.1
  - AC-COORDINATOR-WAKE-005.2
  - AC-COORDINATOR-WAKE-005.3
  - AC-COORDINATOR-WAKE-005.4
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/containment.md
  - ../../specs/coordinator/system-design/spend.md
---

# Task 05: Admission, Delivery And The Unattended Turn (WP-11)

## Summary

Delivers pending wakes as at most one unattended turn per coordinator into its
existing conversation, after the eight ordered admission checks, and settles
the turn when the session leaves `RUNNING`. No path creates, archives or
repoints a conversation.

## In scope

- `internal/coordinator/admission.go`: `Admit` with the eight checks in order,
  calling task 02's `Check` and task 03's `Spend`
  ([Admission](../../specs/coordinator/system-design/wake.md#admission)).
- `internal/coordinator/delivery.go`: `Deliver` under a keyed mutex, `Kick`
  on a coalescing bounded worker, the re-check and 20-wake batch, the
  transactional turn insert and wake marking, the send through the
  conversation message path with `metadata.coordinator_wake_turn_id`, and the
  `send_failed` rollback ([Delivery](../../specs/coordinator/system-design/wake.md#delivery)).
- `internal/coordinator/turns.go`: turn end on
  `task_session.state_changed`, the backstop's missed-settle rule, the startup
  `interrupted` pass, and cost via task 03's `TurnCost`
  ([Turn end](../../specs/coordinator/system-design/wake.md#turn-end)).
- The backstop hooks of task 04: step 3 calls `CheckCeiling`, step 4 calls
  `Deliver`. Kick triggers from wake insert, conversation idle, the autonomy
  and ceiling PATCH, and turn end.
- The turn message text ([Transcript](../../specs/coordinator/system-design/wake.md#transcript)),
  agent-facing, not localized.
- `no_turn_start_test.go`: add the wake delivery as the one allowed non-manager
  turn start (amended `AC-COORDINATOR-COPILOT-002.1`) and add every other
  phase 3 path (recorder, backstop, relay read, automatic approval,
  improvement approve and apply) to `noTurnStartPaths`. Reply delivery is a
  manager's message and is covered by task 08.

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
- A send error and a crash after marking return the wakes to `pending` and
  record `send_failed` or `interrupted`; a turn uses the phase 1 tool surface
  and policy (guard table test runs against a delivery-started session).

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Admit|Deliver|Turn|NoTurnStart' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'Deliver' -race -count=1
make -C apps/backend lint
```

## Risks

- Reading "queued message" must use the orchestrator's queue read, not a
  message-table heuristic; a wrong read sends into a busy session.
