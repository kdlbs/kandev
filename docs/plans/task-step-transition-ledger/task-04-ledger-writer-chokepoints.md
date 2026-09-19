---
id: "04-ledger-writer-chokepoints"
title: "Ledger writer in the mutating transactions"
status: done
wave: 4
depends_on: ["03-ledger-schema-migration"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-task-step-transition-ledger.md"
---

# Task 04: Ledger writer in the mutating transactions

Add the attribution contract package and write exactly one ledger row from
inside each of the seven repository transactions that can change
`tasks.workflow_step_id`. Callers are not touched, so every row lands with
trigger `unknown` and actor `unknown` — which the spec defines as a recorded
fact, not an error. Task 05 makes the attribution real.

This is the only task in the plan that changes committed behaviour on every step
change in the product. Scoping it this way lets the write path be reviewed for
correctness on its own, before the orchestrator and MCP edits arrive.

## Acceptance

1. Every committed change to `tasks.workflow_step_id` produces exactly one row
   in the same transaction; a rolled-back, WIP-rejected, or conditionally-skipped
   update produces none; a no-op step change (position-only reorder, re-issued
   move to the current step) produces none.
2. For any task, reading its rows ordered by `(occurred_at, id)`, each row's
   `from_workflow_step_id` equals the previous row's `to_workflow_step_id`, and
   the last row's `to_workflow_step_id` equals the task's current
   `tasks.workflow_step_id` — under concurrent moves.
3. `''` never appears in any of the four `from_*`/`to_*` columns; the same
   condition stores `NULL`.

## Verification

```
cd apps/backend && go test -race ./internal/steptelemetry/... ./internal/task/repository/sqlite/... && KANDEV_TEST_POSTGRES_DSN="${KANDEV_TEST_POSTGRES_DSN}" go test -race -run 'Postgres|StepTransition' ./internal/task/repository/sqlite/... && make lint
```

`-race` is not optional here: the chain-invariant tests run concurrent movers.
Record in `## Results` whether the Postgres leg ran or skipped.

## Files likely touched

- `apps/backend/internal/steptelemetry/steptelemetry.go` (new) — `Trigger` and
  `ActorKind` enums, `Attribution`, `WithAttribution`, `FromContext`,
  `ContractVersion`, `ContractKey`
- `apps/backend/internal/steptelemetry/steptelemetry_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/step_transitions.go` (new) —
  `stepTransitionTx`, `readTaskStepInTx`, `recordStepTransition`
- `apps/backend/internal/task/repository/sqlite/task.go` — `insertTaskTx` (319),
  `UpdateTask` (469), `UpdateTaskIfWorkflowStepHasCapacity` (778),
  `PromoteQueuedTaskIfWorkflowStepHasCapacity` (830),
  `RestoreTaskMessageRollbackIfSessionState` (2043)
- `apps/backend/internal/task/repository/sqlite/workflow.go` —
  `AddTaskToWorkflow` (17) and `RemoveTaskFromWorkflow` (25), both of which must
  gain a transaction they do not currently have
- `apps/backend/internal/task/repository/sqlite/step_transitions_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/step_transitions_chain_test.go`
  (new)
- `apps/backend/internal/task/repository/sqlite/step_transitions_lifecycle_test.go`
  (new)

## Dependencies

Task 03 — the table must exist.

## Parallelism

`sequential`.

## Inputs

- Spec, *Slice 2 — task-level transition ledger*, *Determinism, ordering, and
  concurrency*, *Failure modes*, and *Scenarios → Slice 2 / Ordering, chain, and
  concurrency*
- Plan, *Area 4* and *Area 5*, including the per-chokepoint table
- **Read the old step inside the write transaction.** The chain invariant holds
  under concurrent moves only if the read of the old step and the write of the
  new one are serialized against other writers of the same task row. Use
  `readTaskStepInTx` with `FOR UPDATE` appended on Postgres
  (`dialect.IsPostgres`); on SQLite the writer pool already serializes writers.
  Reading the old step from the in-memory `*models.Task` the caller handed you
  is the bug this invariant exists to prevent.
- **The missing-table case needs no detection code, and must not get any.** If
  the `CREATE TABLE` was silently swallowed, the INSERT fails and that error
  returns, rolling back the enclosing transaction — exactly the required
  behaviour. The rule is negative: never log-and-continue on
  `recordStepTransition`'s error, and never guard it with a table-exists probe,
  which would let the step change commit with no row.
- **Three of the seven have conditional UPDATEs.**
  `PromoteQueuedTaskIfWorkflowStepHasCapacity`,
  `RestoreTaskMessageRollbackIfSessionState`, and `RemoveTaskFromWorkflow` can
  affect zero rows. Record only when `RowsAffected() > 0`.
- **Two of the seven have no transaction today.** `AddTaskToWorkflow` and
  `RemoveTaskFromWorkflow` are bare `r.db.ExecContext` calls; wrap each in
  `r.db.BeginTx` with the established `defer func() { _ = tx.Rollback() }()`
  shape used elsewhere in the package.
- **Genesis rows come free from the existing placement logic.** By the time
  `insertTaskTx` runs, `applyAdmissionPlacement` (task.go:235) has already
  rewritten `task.WorkflowStepID` to the *actual* placement, so recording from
  the task struct satisfies the spec's feeder-step scenario without a special
  case. A task created with no workflow writes nothing — a row with both sides
  NULL is forbidden.
- `stepTransitionTx` must be satisfied by both `*sql.Tx` and `*sqlx.Tx`; the
  package uses both (`insertTaskTx` takes `*sql.Tx`, `purgeTaskQueueInTx` takes
  `*sqlx.Tx`).
- No unique constraint and no idempotency key. A card legitimately moving
  A→B→A→B produces four rows. Engine retries are already deduplicated upstream
  by the engine's `OperationID` applied-operations store.
- Archive, unarchive-in-place, cascade archive, and delete write no row because
  none of them touches `workflow_step_id`. That is the specified behaviour;
  add the lifecycle test that pins it rather than adding code.
- Watch the backend complexity limits (`.golangci.yml`: 80 lines / 50 statements
  per function, cyclomatic 15). `UpdateTask` and
  `UpdateTaskIfWorkflowStepHasCapacity` are already long — extract rather than
  grow them.

## Output contract

Summary, files changed, tests run with counts, blockers, risks, and a status
update to this file and `plan.md` in the same conversation.

## Results

See `plan.md` § Verification Results for the consolidated commands, outcomes, and decision record covering all six tasks (implemented and committed together).
