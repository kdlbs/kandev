---
id: "01-task-graph-read"
title: "Add the task graph read"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-002
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-003
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-004
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-007
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.5
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.8
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-003.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-003.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-003.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-004.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-004.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-007.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-007.2
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 01: Add the task graph read

## Summary

Add `GET /api/v1/tasks/:id/graph` that returns a bounded subtree or
dependency-chain graph with node state, blockage, and edge verdicts.

## In scope

- `dto.TaskGraphDTO`, node, edge, and root types.
- `Service.BuildTaskGraph(ctx, taskID, mode)` with mode availability, default
  selection, subtree traversal, chain traversal, external nodes, dedupe, and
  the 200-node budget.
- The handler, route registration, `graph_mode_unavailable` error, and
  not-found handling.
- Service and handler tests.

## Out of scope

- Frontend changes.
- Changes to dependency writes, events, or the existing dependency routes.

## Acceptance

- Subtasks mode returns all non-archived, non-ephemeral descendants with
  depth and parent title, member-to-member edges, and one-hop external nodes.
- Chain mode returns only transitive predecessors and dependents, including
  archived ones, with all edges between displayed nodes.
- The read caps at 200 nodes with `truncated: true`, uses the task read's
  authorization, and rejects an unavailable mode with HTTP 400.

## TDD sequence

1. Failing service tests for mode availability and default selection.
2. Failing service tests for subtree root selection, depth, filtering, and
   external nodes.
3. Failing service tests for chain direction, archived nodes, and edge set.
4. Failing service tests for truncation and dedupe.
5. Failing handler tests for the response shape, default mode, 400, and 404.
6. Implement, then refactor with tests green.

## Verification

```bash
cd apps/backend && go test ./internal/task/service -run 'TaskGraph' -count=1
cd apps/backend && go test ./internal/task/handlers -run 'TaskGraph' -count=1
make -C apps/backend test
bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/dto/task_graph.go`
- `apps/backend/internal/task/service/service_task_graph.go`
- `apps/backend/internal/task/service/service_task_graph_test.go`
- `apps/backend/internal/task/handlers/task_graph_handlers.go`
- `apps/backend/internal/task/handlers/task_graph_handlers_test.go`
- `apps/backend/internal/task/handlers/task_handlers.go`

## Dependencies

None. Uses `ListChildrenLimited`, `GetTasksByIDs`, `ListBlockersForTasks`,
`ListDependentsForTasks`, and `BuildDependencyViews`.

## Risks

- The edge verdict helper must be shared, not copied, so graph edges match the
  dependency chip.
- `BuildDependencyViews` truncates the `blocks` list for display; the graph must
  read edges from the batched repository calls, not from the view lists.

## Parallelism

`sequential`

## Results

Implemented `Service.BuildTaskGraph` (`service_task_graph.go`), `dto.TaskGraphDTO`,
and `GET /api/v1/tasks/:id/graph`. Subtree walk uses `ListChildrenLimited` with
the remaining node budget; chain walk runs two direction-only batched BFS
passes; edges reuse `DependencyStatusForTask`; node blockage reuses
`BuildDependencyViews`. Kanban subtasks are limited to one nesting level by the
existing depth rule, so `depth` is 1 for every member in practice.

Validation passed:

- `go test ./internal/task/service -run TaskGraph` (7 tests)
- `go test ./internal/task/handlers -run TaskGraph` (4 tests)
- `bash scripts/lint-go-changed <changed files>`: 0 issues
