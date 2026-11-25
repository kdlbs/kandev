---
id: "02-graph-model-layout"
title: "Add the graph model and layout"
status: done
wave: 2
depends_on:
  - "01-task-graph-read"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-002
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-003
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-005
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-006
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-007
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.6
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.7
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-003.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.6
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-007.3
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 02: Add the graph model and layout

## Summary

Add the renderer-agnostic frontend layer: API client, graph model with step
numbers and chain tracing, dagre layout, and the `useTaskGraph` hook.

## In scope

- `getTaskGraph(taskId, mode?)` and response types.
- `buildTaskGraphModel`, `traceChain`, and `noOrderSet`.
- `layoutTaskGraph` with fixed node sizes and dagre ranks equal to step numbers.
- `useTaskGraph` with loading, error, retry, mode change, and debounced
  refresh from the store signature.
- Add `@dagrejs/dagre` to `apps/web` after user approval.
- The `no-restricted-imports` rule for `@dagrejs/dagre` outside `layout.ts`.

## Out of scope

- React components, the renderer, and `@xyflow/react`.

## Acceptance

- Step numbers follow the longest-path rule, and a corrupt cycle terminates
  with a bounded step.
- `traceChain` returns exactly the transitive predecessors, dependents, and the
  edges between them.
- The hook refetches once per debounce window when the store signature
  changes and keeps the previous graph during a refetch.

## TDD sequence

1. Failing API client tests.
2. Failing model tests for steps, parallel nodes, cycles, and `noOrderSet`.
3. Failing `traceChain` tests.
4. Failing layout tests: every node has a position, and x order follows step.
5. Failing hook tests with fake timers.
6. Implement, then refactor with tests green.

## Verification

```bash
cd apps/web && pnpm exec vitest run lib/task-graph lib/api/domains/task-graph-api.test.ts hooks/domains/task/use-task-graph.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/api/domains/task-graph-api.ts` and test
- `apps/web/lib/task-graph/model.ts` and test
- `apps/web/lib/task-graph/layout.ts` and test
- `apps/web/hooks/domains/task/use-task-graph.ts` and test
- `apps/web/package.json`, `apps/pnpm-lock.yaml`
- `apps/web/eslint.config.mjs`

## Dependencies

Task 01 for the response contract.

## Risks

- dagre rank constraints: use the model step as the node `rank` input, or post-
  align x coordinates by step, and assert the result in tests.
- The store signature selector must return a stable primitive string to avoid
  render loops.

## Parallelism

`sequential`

## Results

Implemented `getTaskGraph`, `buildTaskGraphModel`, `traceChain`,
`layoutTaskGraph`, and `useTaskGraph`. dagre takes no rank input, so the layout
uses dagre only for in-column order and sets x from the model step, re-packing
each column to avoid overlap. Added `@dagrejs/dagre` 3.1 and the
`no-restricted-imports` boundary rule (verified with a probe file).

Validation passed:

- `pnpm exec vitest run lib/task-graph lib/api/domains/task-graph-api.test.ts hooks/domains/task/use-task-graph.test.ts` (21 tests)
- `pnpm run typecheck`, `pnpm exec eslint --max-warnings 0` on changed files
