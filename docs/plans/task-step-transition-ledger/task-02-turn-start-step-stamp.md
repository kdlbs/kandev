---
id: "02-turn-start-step-stamp"
title: "Turn-start step stamp (Slice 1)"
status: done
wave: 2
depends_on: ["01-telemetry-activation-registry"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-task-step-transition-ledger.md"
---

# Task 02: Turn-start step stamp (Slice 1)

Record the workflow step a turn's task was in when the turn started, as a new
key in the existing `task_session_turns.metadata` JSON object. No column is
added. This is the whole of Slice 1, and it ships and is measured before Slice 2
is built.

## Acceptance

1. Every turn created after activation whose task holds a workflow step carries
   `metadata.workflow_step_id_at_start` = that step ID; a turn whose task holds
   no step carries **no such key** — not `""`, not `null`, not `0`.
2. The stamp does not depend on `runtime_config_snapshot`: a turn with nothing
   to snapshot still carries the stamp, and a turn with neither still persists
   no metadata at all.
3. A task read failure omits the stamp and still creates the turn; turn creation
   never fails because telemetry could not be resolved.

## Verification

```
cd apps/backend && go test ./internal/task/service/... ./internal/task/models/... ./internal/task/repository/sqlite/... && make lint
```

## Files likely touched

- `apps/backend/internal/task/models/models.go` — add
  `TurnMetaKeyWorkflowStepIDAtStart` beside `TurnMetaKeyRuntimeConfigSnapshot`
  (line 232)
- `apps/backend/internal/task/service/service_turns.go` — add
  `turnStartMetadata`; repoint `StartTurn` (line 44) and `createCompletedTurn`
  (line 79) at it
- `apps/backend/internal/task/service/service_turns_step_stamp_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/step_stamp_migration_test.go`
  (new) — pre-activation turns are not backfilled
- `apps/backend/internal/telemetrycontract/contract.go` — fill in the
  `turn.workflow_step_id_at_start` contract's `ExistsQuery` / `StatsQuery` now
  that the stamp is real

## Dependencies

Task 01 — the contract must be registered and activated before the writer is
meaningful.

## Parallelism

`sequential`.

## Inputs

- Spec, *Slice 1 — turn-start step stamp*, *`task_session_turns.metadata`*, and
  the seven *Scenarios → Slice 1* cases; each maps to one test row
- Plan, *Area 2*
- `service_turns.go:89-98` — `runtimeConfigSnapshotMetadata` returns `nil` when
  the snapshot is empty. Do **not** early-return on that nil: the spec has an
  explicit scenario for a session with no runtime config whose turn is still
  stamped. Compose, don't short-circuit.
- `createCompletedTurn` (line 68) is the synthetic-turn path for lifecycle
  messages. It shares the same composer, which is what satisfies the spec's
  "synthetic completed turn carries the same stamp" scenario — do not give it a
  separate path.
- `s.tasks.GetTask(ctx, session.TaskID)` is the read; `service_turns.go:723`
  already uses that seam.
- Immutability needs no code — nothing rewrites turn metadata after creation.
  Add the regression test that pins it rather than adding a guard.
- Assert key **absence** with the two-value map read (`_, ok := md[key]`), never
  by comparing to `""`. A test that compares values passes on a bug that writes
  an empty string, which is the exact failure the spec forbids.
- Call `steptelemetry.RecordTurnStamp` only if Task 06's metrics helper already
  exists; otherwise leave a plain counter out of scope here and let Task 06 add
  it. Do not invent a second metrics namespace.

## Output contract

Summary, files changed, tests run with counts, blockers, risks, and a status
update to this file and `plan.md` in the same conversation.

## Results

See `plan.md` § Verification Results for the consolidated commands, outcomes, and decision record covering all six tasks (implemented and committed together).
