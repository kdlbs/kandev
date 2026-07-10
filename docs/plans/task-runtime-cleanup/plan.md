---
spec: docs/specs/tasks/runtime-cleanup.md
created: 2026-06-22
status: implemented
---

# Implementation Plan: Task Runtime Cleanup

## Overview

Runtime cleanup moves from active-session discovery to durable
`executors_running` ownership. The implementation starts with repository queries
and service ordering, then hardens agentctl subprocess shutdown, then adds startup
reconciliation and focused regression coverage.

---

## Backend

### Runtime Inventory

- Add `ListExecutorsRunningByTaskID(ctx, taskID string) ([]*models.ExecutorRunning, error)` to the task repository interface in `apps/backend/internal/task/repository/interface.go`.
- Implement the query in `apps/backend/internal/task/repository/sqlite/executor.go`, ordered by `updated_at DESC` for deterministic tests.
- Reuse the existing global `ListExecutorsRunning(ctx)` startup inventory for
  reconciliation, and stop terminal or missing-session rows before removing
  runtime tracking.

### Task Cleanup Ordering

- Update `apps/backend/internal/task/service/service_tasks.go` so archive/delete
  builds stop targets from `executors_running` rows for the task, not only from
  `ListActiveTaskSessionsByTaskID`.
- Keep active-session cancellation for user-facing session state, but make runtime
  stop target discovery independent from that session-state query.
- Change cleanup ordering so `performTaskCleanup` removes executor rows only after
  the stop attempts have completed.
- When stop fails or cannot be confirmed, preserve the executor row with a
  retryable diagnostic instead of deleting it in the same cleanup pass.

### Orchestrator Stop Path

- Reuse `TaskExecutionStopper.StopExecution` when `agent_execution_id` is present.
- Add bounded fallback behavior for rows whose in-memory execution is missing but
  whose runtime handle is still persisted.
- Ensure task archive/delete cleanup logs the task ID, session ID, execution ID,
  runtime, and failure reason for every failed stop attempt.

### Agentctl Subprocess Shutdown

- Audit `apps/backend/internal/agentctl/server/process/manager.go` so every
  agentctl instance shutdown calls `Manager.Stop(ctx)`.
- Ensure `waitForProcessExit` escalates to process-group kill for Codex/Claude ACP
  children when graceful stdin EOF does not finish by timeout.
- Add a Unix child-lifecycle guard where appropriate so ACP subprocess groups do
  not survive an unexpected agentctl parent exit.

### Startup Reconciliation

- Update orchestrator startup reconciliation to scan persisted
  `executors_running` rows and safely clean terminal or missing-session rows.
- Apply fail-closed semantics from ADR-0009: if inventory queries fail, log and
  skip destructive cleanup.
- Remove rows only after the runtime is positively stopped or confirmed absent.

---

## Frontend

No frontend changes are planned. Existing archive/delete/session controls keep the
same UI and API behavior; the cleanup guarantee changes backend behavior.

---

## Tests

- **What:** repository returns all runtime rows for a task regardless of session
  state.
  **File:** `apps/backend/internal/task/repository/sqlite/executor_test.go`
  **How:** SQLite integration test seeding active and terminal sessions with
  `executors_running` rows.

- **What:** archive cleanup stops terminal-session runtime rows before deleting
  runtime tracking.
  **File:** `apps/backend/internal/task/service/service_tasks_test.go`
  **How:** service test with a fake `TaskExecutionStopper` and a completed
  session that still has an executor row.

- **What:** delete cleanup preserves runtime tracking when stop fails.
  **File:** `apps/backend/internal/task/service/service_tasks_test.go`
  **How:** fake stopper returns an error; assert executor cleanup is not called
  for that row and a warning path is exercised.

- **What:** process manager kills the process group when graceful ACP shutdown
  times out.
  **File:** `apps/backend/internal/agentctl/server/process/manager_test.go`
  **How:** subprocess fixture that ignores stdin EOF and spawns a child; assert no
  child remains after stop timeout.

- **What:** startup reconciliation stops terminal or missing-session runtime rows
  before deleting tracking and preserves rows when stop fails.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`
  **How:** SQLite-backed orchestrator test with a fake agent manager covering
  terminal success/failure and missing-session success/failure.

---

## E2E Tests

Skipped. The spec has no new user-visible UI flow; backend integration tests cover
the observable cleanup guarantees.

---

## Implementation Waves

Wave 1:
- [x] [task-01-runtime-inventory](task-01-runtime-inventory.md)
- [x] [task-02-task-cleanup-ordering](task-02-task-cleanup-ordering.md)

Wave 2:
- [x] [task-03-agentctl-process-group-shutdown](task-03-agentctl-process-group-shutdown.md)
- [x] [task-04-startup-reconciliation](task-04-startup-reconciliation.md)

Wave 3:
- [x] [task-05-verification-and-doc-sync](task-05-verification-and-doc-sync.md)

## Open Questions

- Should retryable cleanup failures reuse `executors_running.status`/`error_message`,
  or should they introduce a more explicit cleanup status column in a follow-up?
