---
id: "01-pending-prompt-count-queries"
title: "Add pending prompt count queries to the message queue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-queued-prompt-count.md"
---

# Task 01: Add Pending Prompt Count Queries

## Acceptance

- `messagequeue.Repository` (interface, SQLite, memory) exposes
  `CountPendingByTaskIDs(ctx, taskIDs []string) (map[string]int, error)`
  returning pending rows grouped by `task_id`. Pending excludes rows whose
  metadata marks `lifecycle_reserved_in_flight` (same semantics as
  `Service.GetStatus`); durable lifecycle rows not yet reserved still count.
  The reserved exclusion happens in Go, never via JSON matching in SQL.
- `messagequeue.Service` exposes `CountPendingByTaskIDs` and
  `CountPendingByTask(ctx, taskID string) (int, error)`.
- Unit tests prove: multi-session accumulation for one task, reserved-in-flight
  exclusion, empty result maps to zero, batch with mixed found/missing task
  IDs, and cross-task isolation. Memory and SQLite repositories both covered.

## TDD Sequence

1. Write repository tests (SQLite + memory) and service tests asserting the
   behavior above. Run RED.
2. Implement `CountPendingByTaskIDs` on the interface, `sqliteRepository`, and
   `memoryRepository`, plus the two service methods.
3. Run the package tests GREEN and refactor.

## Verification

```bash
cd apps/backend
go test ./internal/orchestrator/messagequeue/...
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service_test.go`

## Dependencies

None.

## Output Contract

Report RED/GREEN evidence, the final method signatures, and changed files.
Update this task and `plan.md` status in the same implementation conversation.

## Results

- RED: `TestCountPendingByTaskIDs*` and `TestServiceCountPendingByTask` failed to compile (methods absent).
- GREEN: `go test -tags fts5 ./internal/orchestrator/messagequeue/` passes — SQLite and memory both cover
  multi-session accumulation, reserved-in-flight exclusion, empty/missing task IDs, and cross-task isolation.
- Final signatures: `Repository.CountPendingByTaskIDs(ctx, taskIDs []string) (map[string]int, error)`
  (SQLite + memory), `Service.CountPendingByTaskIDs`, `Service.CountPendingByTask(ctx, taskID) (int, error)`.
