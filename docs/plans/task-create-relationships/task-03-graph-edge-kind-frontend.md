---
id: "03-graph-edge-kind-frontend"
title: "Render related-to edges in the graph and board view"
status: done
wave: 3
depends_on: ["02-graph-edge-kind-backend"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.3
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 03: Render related-to edges in the graph and board view

## Summary

Thread the new `kind` discriminator through `lib/task-graph/model.ts`,
`lib/task-graph/layout.ts`, the xyflow renderer, and the legend, and update
`TaskGraphButton`'s visibility and the live-refresh signature to account for
related-to links. Include related-to edges in the board Graph view under its
existing same-lane inclusion rule.

## In scope

- `TaskGraphEdgeResponse`/`TaskGraphEdge` gain `kind`; `status` becomes
  optional, present only for `kind: "dependency"`.
- `buildTaskGraphModel` excludes `related` edges from step computation and
  adjacency used for ranking, but includes them in the returned edge list.
- A `related` entry in `task-graph-edge-styles.ts` (dotted neutral line, no
  marker) and `task-graph-legend.tsx`.
- `TaskGraphButton` (and its passthrough-toolbar counterpart) render when the
  store task has a `related` ID, in addition to existing conditions.
- `useTaskGraph`'s live-refresh signature includes each displayed task's
  `related` IDs.
- `buildBoardGraph` draws a `related` edge when both ends are on the same
  lane, matching the existing `dependsOn` same-lane rule.

## Out of scope

- Backend changes (task 02, already done).
- The dependency chip's third section (task 05).

## Acceptance

- A related-to link between two displayed graph nodes draws a dotted,
  arrowless connector, and the legend lists it as "Related".
- A related-to link does not change any node's step number or the Parallel
  steps grouping.
- A task with only a related-to link (no dependency edge, no parent, no
  subtask) shows the Task graph entry control.

## TDD sequence

1. Failing `buildTaskGraphModel` test: a `related` edge is returned but
   excluded from step computation.
2. Failing `task-graph-edge-styles`/legend test for the new style.
3. Failing `useTaskGraphAvailable`/`TaskGraphButton` visibility test for a
   related-only task.
4. Failing `useTaskGraph` signature test including `related` IDs.
5. Failing `buildBoardGraph` test for a same-lane related edge.
6. Implement, then refactor with tests green.

## Verification

```bash
cd apps/web && pnpm vitest run lib/task-graph components/task-graph hooks/domains/task/use-task-graph
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
```

## Files likely touched

- `apps/web/lib/api/domains/task-graph-api.ts`
- `apps/web/lib/task-graph/model.ts`
- `apps/web/lib/task-graph/board.ts`
- `apps/web/components/task-graph/task-graph-edge-styles.ts`
- `apps/web/components/task-graph/task-graph-legend.tsx`
- `apps/web/components/task-graph/xyflow-task-graph-renderer.tsx`
- `apps/web/hooks/domains/task/use-task-graph.ts`
- `apps/web/hooks/domains/task/use-task-graph-available.ts`

## Dependencies

Task 02 (backend `kind` field). Task 01 (store task's `related` IDs reaching
the frontend task model).

## Risks

- `useTaskGraph`'s debounce signature must stay cheap to compute; adding
  `related` IDs to the per-task signature should reuse the same derivation
  pattern as `dependsOn`/`blocks`, not add a new fetch.
- The ESLint `no-restricted-imports` rule isolating `@xyflow/react` and
  `@dagrejs/dagre` must still pass after the edge-style change.

## Parallelism

`parallel-safe` with task 06 (different files).

## Results

Added the graph edge kind, kept related edges out of dependency ranking, rendered related links as dotted arrowless connectors with a legend item, exposed graph availability for related-only tasks, updated live refresh signatures, and included same-lane related links in Board Graph.

Validation results:

- `vitest run lib/task-graph components/task-graph hooks/domains/task/use-task-graph`: 12 files and 64 tests passed.
- `node_modules/.bin/tsc --noEmit`: passed.
- `pnpm run lint`: passed.
- `git diff --check`: passed.
