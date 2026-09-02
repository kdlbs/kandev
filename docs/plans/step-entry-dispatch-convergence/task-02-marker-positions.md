---
id: "02-marker-positions"
title: "Persist the allocated marker position set"
status: done
wave: 2
depends_on: ["01-ownership-declaration"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-004
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-008
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.9
  - AC-OFFICE-STEP-ENTRY-DISPATCH-004.10
  - AC-OFFICE-STEP-ENTRY-DISPATCH-008.1
  - AC-OFFICE-STEP-ENTRY-DISPATCH-008.3
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 02: Persist the allocated marker position set

## Summary

Add `workflow_step_entries.marker_positions` and write the ordered list of
marker-bearing positions, computed once from the ownership declaration, inside
the same transaction as the entry row. Entry terminality becomes decidable from
the store alone, without loading a workflow declaration.

## In scope

- The `r.migrate.Apply("workflow_step_entries.marker_positions", ...)` ALTER in
  `base_migrations.go`, with `""` as the legal empty representation.
- `BuildPendingAllocation` returning the position set it already computes, and
  `allocateStepEntryIfPending` persisting it.
- Making allocation unconditional for a step declaring at least one
  marker-bearing kind.
- Test coverage for the atomic decision clear (008.1) and for an allocation
  failure that fails the transition as a unit (008.3).

## Out of scope

- Reading the column back. Task 06 consumes it; the probe that guards that read
  belongs there.
- The terminal marker state and its closed reason list. This work order claims
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.9 only for the stored allocated position
  set that terminality is derived from; Task 04 claims the state itself.
- Adding a state column to `workflow_step_entries`. Terminality stays derived.

## Acceptance

- An entry allocated at a step whose sequence declares a non-marker-bearing
  kind stores a `marker_positions` value that omits that position.
- A failed allocation leaves neither the entry row nor the step change, and
  emits an ERROR naming task id and step id.
- The atomic-clear test fails against a non-atomic implementation, with the
  failure injected inside the repository method rather than a store double.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite/... ./internal/workflow/stepentry/... ./internal/orchestrator/...
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/step_entries.go`
- `apps/backend/internal/task/repository/sqlite/step_entries_test.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/workflow/stepentry/stepentry.go`
- `apps/backend/internal/orchestrator/step_entry_dispatch_atomic_test.go`

## Dependencies

Task 01, which owns the declaration the position set is computed from.

## Risks

- A test over a step whose every kind is marker-bearing cannot distinguish a
  persisted set from one recomputed from the declaration. The shipped Review
  step is not that shape, and the test must use one that is not either.
- Unconditional allocation adds one row per arrival on routes that previously
  allocated none.

## Parallelism

`sequential`

## Inputs

- System design, "Where terminality lives" and "Task-scoped turn completion"
  for the `entry_seq` allocation note.
- `internal/task/repository/sqlite/step_entries.go`.

## Results

Shipped in Build round 1 (`92755673f` + `02f5a1b1e`), with two subsequent
hardening rounds. `workflow_step_entries.marker_positions` was added via an
idempotent `ADD COLUMN` migration; `BuildPendingAllocation` computes the ordered
marker-bearing position set from the ownership declaration and
`allocateStepEntryIfPending` persists it inside the same transaction as the entry
row. Testing round 1 found the atomicity guarantee (AC-008.1) and the
allocation-failure ERROR log (AC-008.3) were untested; Build round 2
(`1ee2eb149`) closed both with a SQLite trigger-based fault injection between the
delete and marker-complete steps, and an ERROR log at both error-return sites in
`allocateStepEntryIfPending`, each verified discriminating by temporarily
reintroducing the defect. Testing round 2 found the Postgres counterpart of the
`marker_positions` ADD COLUMN had no replay test; closed directly (additive gap)
via `cd9c7f6aa` with a SQLite legacy-replay test plus an env-gated Postgres
counterpart (Postgres itself unexercised in this environment — `KANDEV_TEST_POSTGRES_DSN`
unset, declared gap). Review round 1 found `BuildPendingAllocation`'s raw-list
indexing could desync from `DispatchStepEntry`'s compiled-list indexing whenever
an earlier action fails to compile; Build round 3 (`9c27351f2`) closed this via a
new `Action.DeclaredPosition` field set at compile time and used by the claim
path instead of a loop index, plus a regression test.
Verification: `go test ./internal/task/repository/sqlite/... ./internal/workflow/stepentry/...
./internal/orchestrator/...` passes, including `-race`.
