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
  `CoordinatorsOwningTask`, `RecordWake` (under task 01's `WithWakeLock`:
  count below 200, then insert-or-nothing on the unique key, in one
  transaction), pending reads
  ([Own tasks](../../specs/coordinator/system-design/wake.md#own-tasks)).
- `internal/coordinator/wake_episodes.go`: one reader per kind returning the
  current episode key from stored state (pending clarification bundle,
  pending permission message, `coordinator_stalls` row only while
  [current](../../specs/coordinator/system-design/wake.md#stall-currency),
  active session error, task state), shared by the recorder, the backstop and task 05's re-check.
- `internal/coordinator/wake_recorder.go`: subscribers for
  `session.pending_action_changed`, the stall upsert hook in `stalls.go`,
  `task_session.error_changed` and `task.state_changed`
  ([Recorder](../../specs/coordinator/system-design/wake.md#recorder)).
- `internal/coordinator/wake_backstop.go`: the 60-second ticker, started
  after the startup pass and joined before the store closes. It builds the
  step 1 visit set (autonomy on, an open or recently settled unattended turn,
  a `claimed_automatically = 1` proposal) and runs the step 3.1 wake
  recording only for coordinators whose re-read row has autonomy on
  ([Backstop](../../specs/coordinator/system-design/wake.md#backstop)). A
  `Hooks` struct carries the step 2 turn duties (task 05), the step 2.4
  lowering retry (task 09) and the step 3.2 `Deliver` (task 05); each hook is
  a no-op until its owner fills it.
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
  insert and a metric, and the backstop stores it once below 200; 20
  concurrent `RecordWake` calls for new episodes at 190 pending leave exactly
  200 (SQLite, and PostgreSQL under `KANDEV_TEST_POSTGRES_DSN` with `-race`).
- A stall row whose `detected_at` is earlier than the task's
  `last_activity_at` stores no wake from the recorder or the backstop.
- With every event suppressed, the backstop stores each episode within one
  period (`synctest`); a failing read for one coordinator logs, skips it and
  continues; turning autonomy on stores existing episodes on the next pass.
- The visit set holds a coordinator with autonomy off and an open turn, one
  with a turn settled 9 minutes ago, and one with a `claimed_automatically =
  1` proposal. It runs their turn and setting hooks and records no wake and
  calls no `Deliver` for them. It excludes a coordinator with autonomy off,
  no open turn, no turn settled in the last 10 minutes and no automatic
  claim. A failing visit-set query skips only its own source.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Wake|Backstop|OwnTasks|Episode' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'Wake' -race -count=1
make -C apps/backend lint
```

## Risks

- Goroutine leaks in the ticker: the test uses the repository's goleak
  instrumentation and the join on shutdown.
