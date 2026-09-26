---
id: "06-writer-health-controls"
title: "Writer health controls"
status: done
wave: 6
depends_on: ["04-ledger-writer-chokepoints", "05-caller-attribution-wiring"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-task-step-transition-ledger.md"
---

# Task 06: Writer health controls

A telemetry writer that silently stops is worse than one that was never built,
because its output still looks like data. Add the spec's three required
controls: a pinning test over the set of statements that may mutate
`tasks.workflow_step_id`, runtime counters, and a boot health line covering both
contracts.

The pinning test is the control that matters. The realistic failure is not the
writer breaking; it is a new code path bypassing it.

## Acceptance

1. A new production statement that mutates `tasks.workflow_step_id` fails the
   test suite until it is registered in the pinned set.
2. Writing a ledger row increments an expvar counter keyed by trigger and emits
   a `telemetry.metric.*` log line naming the event; creating a turn increments
   a counter keyed by whether the stamp was present and emits the same shape.
3. Boot emits one health line per registered contract key reporting object
   existence, activation time, row count, and most recent `occurred_at` — so an
   install whose ledger stopped growing is visible in one line.

## Verification

```
cd apps/backend && go test ./internal/steptelemetry/... ./internal/telemetrycontract/... ./internal/task/repository/sqlite/... ./internal/task/service/... && make lint
```

Additionally confirm the pinning test actually bites, and record the result:
temporarily add an unregistered `UPDATE tasks SET workflow_step_id = ...`
statement to a new function in the repository package, re-run
`go test -run TestStepTransitionWriters ./internal/task/repository/sqlite/...`,
observe the failure, then remove it and confirm `git diff --check` is clean.

## Files likely touched

- `apps/backend/internal/steptelemetry/metrics_vars.go` (new) — expvar maps
  `telemetry_step_transitions_total`, `telemetry_turn_stamps_total`
- `apps/backend/internal/steptelemetry/metrics.go` (new) — `RecordLedgerRow`,
  `RecordTurnStamp`
- `apps/backend/internal/steptelemetry/metrics_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/step_transitions.go` — call
  `RecordLedgerRow` after a successful insert
- `apps/backend/internal/task/service/service_turns.go` — call
  `RecordTurnStamp` on both branches of `turnStartMetadata`
- `apps/backend/internal/task/repository/sqlite/step_transition_writers_pin_test.go`
  (new)
- `apps/backend/internal/telemetrycontract/health_test.go` — extend to cover
  both registered contracts

## Dependencies

Tasks 04 and 05 — the pinned set is only meaningful against a finished writer
set, and the trigger-keyed counter needs real triggers.

## Parallelism

`sequential`.

## Inputs

- Spec, *Writer health* (the three numbered controls and the exact pinned set)
  and *Scenarios → Writer health*
- Plan, *Area 4* (metrics) and *Area 1* (health line)
- The pinned set as of the spec is exactly seven: `UpdateTask`,
  `UpdateTaskIfWorkflowStepHasCapacity`,
  `PromoteQueuedTaskIfWorkflowStepHasCapacity`,
  `RestoreTaskMessageRollbackIfSessionState`, `AddTaskToWorkflow`,
  `RemoveTaskFromWorkflow`, and task creation's admission placement
  (`insertTaskTx`).
- **Implement the pin with `go/parser` over the package's non-test files**, not
  a line regex. Walk function declarations, collect those whose body contains a
  string literal matching `INSERT INTO tasks` or an `UPDATE tasks` statement
  that assigns `workflow_step_id`, and assert set equality against the
  registered names. A regex over raw lines misreports on multi-line SQL literals,
  which every statement in this package is.
- The failure message should name the offending function and say what to do —
  register it and wire `recordStepTransition` — rather than just printing a set
  diff. This test fires for someone who has never read this spec.
- Mirror `internal/office/scheduler/metrics_vars.go` and `routing_metrics.go`:
  counters only, expvar maps published at package init, label strings built as
  `k1=v1;k2=v2`, and each recorder emits **both** the counter bump and the
  structured log. The event name goes first in the log line so an aggregation
  rule matches on it without scanning free text.
- `/debug/vars` only exposes these in dev mode (`KANDEV_MOCK_AGENT=true` or
  `debug.pprofEnabled`); that is the existing contract and needs no change.
- The health line for `turn.workflow_step_id_at_start` has no table of its own.
  Count turns whose `metadata` carries the stamp key and use `started_at` as the
  recency column; `metadata` is stored as TEXT on both dialects, so a
  dialect-neutral predicate works and avoids per-dialect JSON functions.

## Output contract

Summary, files changed, tests run with counts, the pinning-test bite check and
its cleanup evidence, blockers, risks, and a status update to this file and
`plan.md` in the same conversation.

## Results

See `plan.md` § Verification Results for the consolidated commands, outcomes, and decision record covering all six tasks (implemented and committed together).
