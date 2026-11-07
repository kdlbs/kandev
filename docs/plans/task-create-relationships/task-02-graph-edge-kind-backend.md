---
id: "02-graph-edge-kind-backend"
title: "Add the related-to edge kind to the task graph read"
status: done
wave: 2
depends_on: ["01-relation-storage-and-creation"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.4
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 02: Add the related-to edge kind to the task graph read

## Summary

Add a `kind` discriminator (`dependency` | `related`) to
`dto.TaskGraphDTO`'s edges, and make `Service.BuildTaskGraph`'s Subtasks and
Chain traversals include related-to links between two already-displayed
nodes, without pulling in new external nodes or affecting step computation.

## In scope

- `kind` field on the graph edge DTO; `dependency` edges keep `status`,
  `related` edges omit it.
- Subtasks traversal: read related-to links for all members with
  `ListRelationsForTasks`; include a link only when both ends are members;
  drop a link whose other end is not a member.
- Chain traversal: after the existing upstream/downstream BFS settles the node
  set, read related-to links over that final set and include any link between
  two displayed nodes.
- Edge dedupe keyed by `(from, to, kind)`.

## Out of scope

- Frontend model, legend, renderer, or board-graph changes (task 03).

## Acceptance

- A related-to link between two subtree members appears as one `related`
  edge; a related-to link to a task outside the subtree does not add an
  external node and does not appear as an edge.
- A related-to link between two chain nodes appears as one `related` edge,
  including between two upstream (or two downstream) nodes.
- `related` edges carry no `status` field and are excluded from any
  step/rank computation this service performs (none today; the graph model
  computes steps client-side, but the DTO ordering must not imply one).

## TDD sequence

1. Failing service test: subtasks mode includes a related-to edge between two
   members and drops one to a non-member.
2. Failing service test: chain mode includes a related-to edge between two
   upstream nodes and between a focus and a dependent.
3. Failing service test: edge dedupe with a `dependency` and a `related` edge
   between the same pair produces two edges, not one.
4. Implement, then refactor with tests green.

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

## Dependencies

Task 01 (`RelationRepository.ListRelationsForTasks`).

## Risks

- The subtasks walk discovers members breadth-first inside the node budget;
  reading relations for members found right at the 200-node cutoff must not
  silently include a related-to edge to a node that did not make the cut.

## Parallelism

`parallel-safe` with task 04 and task 05 (different files).

## Results

Added the `kind` discriminator to graph edges and included related-to links only between already displayed nodes. Subtasks mode requires both endpoints to be members; Chain mode retains related links between any displayed nodes. Edge de-duplication now includes kind, so dependency and related edges remain distinct.

Validation results:

- `go test ./internal/task/service -run 'TaskGraph' -count=1`: passed.
- `go test ./internal/task/handlers -run 'TaskGraph' -count=1`: passed.
- `make -C apps/backend test`: failed in the full run. Existing failures included fixed-port collisions and launcher version assertions; `internal/task/repository/sqlite` also reached its 600-second timeout. Focused affected-package tests passed.
- `bash scripts/lint-go-changed` on the graph service and test files: extracted blocker and related edge collection into separate helpers, hoisted the shared edge key type, and removed a redundant map type; rerun passed with 0 issues.
- `git diff --check`: passed.
