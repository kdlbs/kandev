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
- Added exact execution teardown ownership across coordinator stops, terminal
  cleanup, launch/recovery races, and durable task-resource cleanup.
- Preserved legacy stop sentinels while exposing exact runtime absence as
  idempotent; real lookup and persistence failures remain retryable.
- Added an atomic session-owned task-state CAS so clarification/terminal state
  cannot be overwritten by delayed runtime reconciliation.
- Changed areas: runtime/lifecycle stop contracts, orchestrator executor and
  recovery paths, task cleanup jobs, MCP clarification state, and focused E2E
  coverage.
- Tests run: `make fmt`, `make typecheck`, `make test`, `make lint`, scoped Go
  race suites, and the sidebar-clarification and Office-onboarding Playwright
  specs against production builds.
- Blockers: none.
- Remaining lifecycle risks: none known; session-scoped lookup absence remains
  retryable until an exact execution identity is available.
