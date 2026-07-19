---
id: "01-execution-stop-semantics"
title: "Execution stop outcomes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/parent-child-task-stop.md"
---

# Task 01: Execution stop outcomes

## Acceptance

- A detailed per-session stop primitive distinguishes accepted cancellation,
  natural terminal/absent execution, and genuine lookup/persistence failure.
- Only `lifecycle.ErrNoExecutionForSession` counts as natural absence. Other
  lookup errors and empty-ID/nil are failures.
- The stop-specific state transition reports changed/final state. Natural
  terminal races do not get overwritten or count as stopped; successful
  acceptance persists `CANCELLED` before async teardown.
- State-write failure prevents teardown for that candidate. Runtime teardown is
  asynchronous and detached from caller cancellation.
- Existing UI, completion, Office/tree/workspace, and handoff stop callers retain
  supplied reason/force and legacy `ErrExecutionNotFound` / partial-success
  behavior.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor ./internal/orchestrator
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction_test.go`
- `apps/backend/internal/orchestrator/executor/executor_mocks_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

## Dependencies

None.

## Inputs

- Spec: `What`, `State and persistence`, `Failure modes`, and `Scenarios`.
- Existing task-stop seam: `Service.StopTask` and `Executor.StopByTaskID`.
- Normal lifecycle absence sentinel: `lifecycle.ErrNoExecutionForSession`.

## Output Contract

Update this task to `done`, update `plan.md`, and report per-session outcomes,
legacy compatibility, files changed, tests run, blockers, and remaining
lifecycle risks.

## Completion

- Added a strict per-session outcome with accepted/final-state reporting.
- Preserved legacy stop reason, force, lookup, persistence, and teardown policy.
- Verified exact lifecycle absence, invariant/lookup failures, terminal races,
  persistence failure, state-before-teardown ordering, and detached teardown.
- Remaining concurrency boundary: the task-level caller must hold the existing
  `cancelInFlightGuard`; that is Task 02.
