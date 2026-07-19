---
id: "02-coordinator-stop-operation"
title: "Coordinator stop operation"
status: done
wave: 2
depends_on: ["01-execution-stop-semantics"]
plan: "plan.md"
spec: "../../specs/tasks/parent-child-task-stop.md"
---

# Task 02: Coordinator stop operation

## Acceptance

- A coordinator-specific orchestrator operation accepts only a task ID, uses the
  exact fixed reason `"stopped by parent task via MCP"` with
  `force=false`, and returns structured `stopped` / `not_running` status.
- Candidate sessions are processed in stable order under the existing
  per-session `cancelInFlightGuard`; ready, pending-move, and queue-drain races
  cannot dispatch replacement work after cancellation acceptance.
- Aggregation follows the spec: all absent/terminal is `not_running`; at least
  one accepted with the rest absent is `stopped`; any genuine synchronous
  failure is an error even after partial success.
- An accepted stop attempts guarded `REVIEW` only for active Kanban state.
  Office, archived, non-active, and `not_running` targets retain task state.
- Queues, workspaces, task environments, descendants, and existing legacy stop
  entrypoints remain intact.
- A deterministic test documents that an execution registering after its
  candidate lookup can escape because v1 has no task-wide launch fence.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/service.go`

## Dependencies

- Task 01 supplies the detailed per-session stop outcome.

## Inputs

- Spec: `Choosing the control`, `State and persistence`, `Failure modes`,
  and `Scenarios`.
- Existing ready/interrupt/drain serialization: `cancelInFlightGuard`.
- Guarded task-state path: `UpdateTaskStateIfCurrentIn` for
  `IN_PROGRESS` / `SCHEDULING` through the production task-service adapter;
  do not publish a duplicate event.
- Scope boundary: lifecycle registration after a candidate lookup is not fenced.

## Output Contract

Update this task to `done`, update `plan.md`, and report aggregation rules,
race coverage, task-state behavior, files changed, tests run, blockers, and
remaining launch-fence risk.

## Completion

- Added deterministic stable-ID traversal and explicit `stopped` /
  `not_running` aggregation with joined synchronous failures.
- Reused `cancelInFlightGuard` through final re-read and cancellation
  acceptance; channel tests cover the ready/queue race under `-race`.
- Added atomic active-session cancellation in SQLite so a concurrent terminal
  transition cannot be overwritten.
- Guarded REVIEW reconciliation preserves Office, archived, non-active, and
  idle targets; queues and task resources are untouched.
- The approved late-registration escape remains documented and tested.
