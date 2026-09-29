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
  - REQ-COORDINATOR-INTEGRATION-002
acceptance_criteria:
  - AC-COORDINATOR-WAKE-001.1
  - AC-COORDINATOR-WAKE-001.2
  - AC-COORDINATOR-WAKE-001.3
  - AC-COORDINATOR-WAKE-001.4
  - AC-COORDINATOR-WAKE-002.1
  - AC-COORDINATOR-WAKE-002.2
  - AC-COORDINATOR-WAKE-002.3
  - AC-COORDINATOR-WAKE-002.4
  - AC-COORDINATOR-INTEGRATION-002.1
  - AC-COORDINATOR-INTEGRATION-002.3
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/wake-recording.md
  - ../../specs/coordinator/system-design/wake-backstop.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 04: Wake Recorder And Level-Triggered Backstop (WP-11)

## Summary

Turns episodes on a coordinator's own tasks into durable wake rows, once per
episode, from both events and a 60-second backstop that re-derives every
current episode from stored state.

## In scope

- `internal/coordinator/wake_store.go`: `ListOwnTasks`,
  `CoordinatorsOwningTask` (both `kind='create_task'`, `status='approved'`,
  distinct, excluding archived, ephemeral and the conversation task),
  `RecordWake` (under task 01's `WithWakeLock`: re-read autonomy from the
  locked row, re-run the own-task predicates, look up the key in any status
  (`exists`), count below 200, then insert-or-nothing on the unique key, in
  one transaction; outcomes `inserted`, `exists`, `capped`, `autonomy_off`,
  `not_own`; the result carries the workspace id), `ExistingWakeKeys` (a
  read-only pre-filter for the backstop), pending reads
  ([Own tasks](../../specs/coordinator/system-design/wake-recording.md#own-tasks)).
  `ListOwnTasks` returns each task's `workflow_id`; the recorder and the
  backstop read `EffectiveWatchSet` once per coordinator and store a wake only
  for a watched own task, and drop the event or skip the coordinator's wake
  duties for the pass when the set cannot be read
  ([Watch set](../../specs/coordinator/system-design/integration.md#watch-set)).
- `internal/coordinator/wake_episodes.go`: the `WakeSources` interface (implemented
  in backendapp over the task repository) and [Episode keys](../../specs/coordinator/system-design/wake-recording.md#episode-keys): one reader per kind returning the
  current episode key from stored state (pending clarification bundle,
  pending permission message, `coordinator_stalls` row only while
  [current](../../specs/coordinator/system-design/wake-backstop.md#stall-currency),
  active session error, task state), shared by the recorder, the backstop and task 05's re-check.
- `internal/coordinator/wake_recorder.go`: subscribers for
  `session.pending_action_changed`, the stall upsert hook in `stalls.go`,
  `task_session.error_changed` and `task.state_changed`
  ([Recorder](../../specs/coordinator/system-design/wake-recording.md#recorder)).
- `internal/coordinator/wake_backstop.go`: the 60-second ticker, owned by
  the Service with idempotent `Start`/`Stop`, first pass 60s after `Start`,
  no overlapping passes, `Stop` registered via `routeParams.addCleanup` so
  it is joined before the pool closes. It builds the
  step 1 visit set (autonomy on, an open or recently settled unattended turn,
  a `claimed_automatically = 1` proposal) and runs the step 3.1 wake
  recording (reads first, records after: any read error skips that
  coordinator's wake duties for the pass) only for coordinators whose re-read
  row has autonomy on
  ([Backstop](../../specs/coordinator/system-design/wake.md#backstop)). A
  `Hooks` struct carries the step 2 turn duties (task 05), the step 2.4
  lowering retry (task 09) and the step 3.2 `Deliver` (task 05); each hook is
  a no-op until its owner fills it.
- `Kick` uses the existing `Service.SetKick` seam through one nil-safe Service
  method, called only after a committed insert and the wake-insert
  `coordinator.updated{autonomy_changed}` publish (task 04 owns that publish;
  task 05 sets `SetKick`); `Service.SetStallWakeHook` (nil by default, set only
  by `registerCoordinatorWake` when phase 3 is effective) is the stall hook;
  `Stop` latches the backstop closed so a later `Start` is a no-op;
  `SetBackstopHooks` merges per field and is honoured only before `Start`; `backendapp/coordinator.go`
  extends `registerCoordinatorWakeState` (`PruneWakeState` first, then
  `StartWakeBackstop`); the recorder's subscriptions belong to the `Service`
  (`StopWakeRecorder`, registered through `p.addCleanup` beside
  `StopWakeBackstop`); `SetBackstopHooks` is called only in registration
  bodies, never in a returned hook, and a hook must return on `ctx` cancel;
  `SetKick` and `SetStallWakeHook` are mutex or atomic guarded; `RecordWake`
  rejects an empty id, an unknown kind or an empty key (`write_error`).
- `Store.PruneWakeState` (task 01) changes: a `delivered` or `superseded` wake
  older than 30 days is deleted only when its task is no longer an own task
  ([wake-recording](../../specs/coordinator/system-design/wake-recording.md#retention-and-own-tasks));
  its test gains a live-own-task case.
- Metrics `coordinator_wake_recorded_total{kind}`,
  `coordinator_wake_dropped_total{reason}` (cap, autonomy_off, not_own,
  not_found, read_error, write_error),
  `coordinator_backstop_skipped_total`.

## Out of scope

- Admission, delivery, turns (task 05); any screen (task 06).

## Acceptance

- Each of the five kinds on an own task of a coordinator with autonomy on
  stores one `pending` wake; the same episode reported by a redelivered
  event, a restart re-emitting `task.stalled`, and a backstop pass stores no
  second row and leaves the status unchanged; a new episode (new
  `pending_id`, new `last_event_at`, new error `stamp`) stores a new row; the stall and error keys are stable
  across PostgreSQL microsecond timestamps; an empty key stores nothing.
- No wake for a task the coordinator does not own, for its own conversation
  task, for a non-primary session, or while autonomy is off; at 200 pending no
  insert and a metric, and the backstop stores it once below 200; 20
  concurrent `RecordWake` calls for new episodes at 190 pending leave exactly
  200 (SQLite, and PostgreSQL under `KANDEV_TEST_POSTGRES_DSN` with `-race`).
- An episode on an own task in an unwatched workflow, or with no workflow,
  stores no wake from the recorder or the backstop; with the watch read
  failing, nothing is stored for that coordinator in that pass; adding a
  workflow to the watch set, or moving an own task into a watched one, stores
  its existing episodes at the next backstop pass
  (`AC-COORDINATOR-INTEGRATION-002.1`, `002.3`).
- A task a `message`, `move` or `resume` proposal only targets is not own; autonomy
  turned off between the read and the insert gives `autonomy_off` and no row;
  a failing episode read means nothing is stored for that coordinator that pass.
- A stall row whose `detected_at` is earlier than the task's
  `last_activity_at` stores no wake from the recorder or the backstop.
- With every event suppressed, the backstop stores each episode within one
  period (`synctest`); a failing read for one coordinator logs, skips it and
  continues; turning autonomy on stores existing episodes on the next pass.
- Turning autonomy off then on with the same question still pending stores no
  second wake and leaves the `superseded` row as is (`exists`); a new
  `pending_id` stores a new row. A steady state of stored episodes causes no
  write transaction per pass. A `GetStall` not-found is no stall, not a read
  error. With phase 3 off and a stored `autonomy_enabled = 1` a stall upsert
  stores no wake.
- The visit set holds a coordinator with autonomy off and an open turn, one
  with a turn settled 9 minutes ago, and one with a `claimed_automatically =
  1` proposal. It runs their turn and setting hooks and records no wake and
  calls no `Deliver` for them. It excludes a coordinator with autonomy off,
  no open turn, no turn settled in the last 10 minutes and no automatic
  claim. A failing visit-set query skips only its own source.
- A `delivered` completed wake older than 30 days on a live own task survives
  the prune and the backstop stores no second `completed` wake; an archived
  task's old wake also survives, and after unarchive no second wake is stored;
  a wake of an ephemeral or proposal-less task is pruned. `-race` sets `SetKick` and `SetStallWakeHook` while
  the recorder runs. At 200 pending the backstop stores freed slots in task id
  then kind order. `SetBackstopHooks` from a returned hook is ignored (warn).

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Wake|Backstop|Recorder|Kick|OwnTasks|Episode|CoordinatorsOwningTask' -count=1
cd apps/backend && go test ./internal/coordinator/... -race -count=1
cd apps/backend && go test ./internal/backendapp/... -count=1   # the three /var-path startup tests fail on origin/main too
cd apps/backend && go run ./cmd/sqlguard ./internal
make -C apps/backend lint
```

## Risks

- Goroutine leaks in the ticker: the test uses the repository's goleak
  instrumentation and the join on shutdown.
