---
id: "04-wake-recorder-backstop"
title: "Wake recorder and level-triggered backstop"
status: pending
wave: 2
depends_on:
  - "01-flag-schema-settings"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-WAKE-001
  - REQ-COORDINATOR-WAKE-002
acceptance_criteria:
  - AC-COORDINATOR-WAKE-001.1
  - AC-COORDINATOR-WAKE-001.2
  - AC-COORDINATOR-WAKE-001.3
  - AC-COORDINATOR-WAKE-001.4
  - AC-COORDINATOR-WAKE-002.1
  - AC-COORDINATOR-WAKE-002.2
  - AC-COORDINATOR-WAKE-002.3
  - AC-COORDINATOR-WAKE-002.4
system_design:
  - ../../specs/coordinator/system-design/wake.md
---

# Task 04: Wake Recorder And Level-Triggered Backstop (WP-11)

## Summary

Turns episodes on a coordinator's own tasks into durable wake rows, once per
episode, from both events and a 60-second backstop that re-derives every
current episode from stored state.

## In scope

- `internal/coordinator/wake_store.go`: `ListOwnTasks`,
  `CoordinatorsOwningTask`, `RecordWake` (count below 200, then
  insert-or-nothing on the unique key), pending reads
  ([Own tasks](../../specs/coordinator/system-design/wake.md#own-tasks)).
- `internal/coordinator/wake_episodes.go`: one reader per kind returning the
  current episode key from stored state (pending clarification bundle,
  pending permission message, `coordinator_stalls` row, active session error,
  task state), shared by the recorder, the backstop and task 05's re-check.
- `internal/coordinator/wake_recorder.go`: subscribers for
  `session.pending_action_changed`, the stall upsert hook in `stalls.go`,
  `task_session.error_changed` and `task.state_changed`
  ([Recorder](../../specs/coordinator/system-design/wake.md#recorder)).
- `internal/coordinator/wake_backstop.go`: the 60-second ticker, started
  after the startup pass and joined before the store closes, steps 1 and 2
  of [Backstop](../../specs/coordinator/system-design/wake.md#backstop), with
  a `Hooks` struct task 05 fills for steps 3 and 4 (no-op until then).
- `Kick` is a no-op interface here; task 05 implements it.
- Metrics `coordinator_wake_recorded_total{kind}`,
  `coordinator_wake_dropped_total{reason}`,
  `coordinator_backstop_skipped_total`.

## Out of scope

- Admission, delivery, turns (task 05); any screen (task 06).

## Acceptance

- Each of the five kinds on an own task of a coordinator with autonomy on
  stores one `pending` wake; the same episode reported by a redelivered
  event, a restart re-emitting `task.stalled`, and a backstop pass stores no
  second row and leaves the status unchanged; a new episode (new
  `pending_id`, new `last_event_at`, new error `stamp`) stores a new row.
- No wake for a task the coordinator does not own, for its own conversation
  task, for a non-primary session, or while autonomy is off; at 200 pending no
  insert and a metric, and the backstop stores it once below 200.
- With every event suppressed, the backstop stores each episode within one
  period (`synctest`); a failing read for one coordinator logs, skips it and
  continues; turning autonomy on stores existing episodes on the next pass.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Wake|Backstop|OwnTasks|Episode' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'Wake' -race -count=1
make -C apps/backend lint
```

## Risks

- Goroutine leaks in the ticker: the test uses the repository's goleak
  instrumentation and the join on shutdown.
