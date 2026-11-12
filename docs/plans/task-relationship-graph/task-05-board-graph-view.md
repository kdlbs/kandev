---
id: "05-board-graph-view"
title: "Add the board Graph view"
status: done
wave: 5
depends_on:
  - "04-workbench-tab"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-009
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-010
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.5
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.6
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.7
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-010.8
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 05: Add the board Graph view

## Summary

Add a Graph view beside Pipeline in the Home view switcher that draws each
workflow lane's filtered tasks as a dependency graph with parent frames, and
point Show in graph at it.

## In scope

- `"graph"` listing view, board view mode, registry entry, desktop toggle and
  tablet menu-sheet item, phone fallback.
- `buildBoardGraph`, `layoutGroupedTaskGraph`, renderer frames, shared
  `TaskGraphCanvas`, `BoardGraph`, `SwimlaneTaskGraphContent`.
- `?graphFocus=` and `useOpenTaskGraph().showInGraph`.

## Out of scope

- Cross-workflow edges between lanes; each lane is its own graph.

## Acceptance

- The Graph view renders the lane's filtered tasks with frames and opens a task
  on activation; Show in graph lands on it with the task focused.

## Verification

```bash
cd apps/web && pnpm exec vitest run lib/task-graph lib/task-listing lib/kanban hooks/use-task-listing-view.test.tsx hooks/domains/task components/task-graph components/kanban
cd apps/web && pnpm exec tsc --noEmit
cd apps/web && pnpm e2e:run --docker --project chromium -- tests/task/task-graph.spec.ts tests/kanban/pipeline-view.spec.ts tests/task/task-listing-view-preferences.spec.ts
cd apps/web && pnpm e2e:run --docker --project mobile-chrome -- tests/task/mobile-task-graph.spec.ts tests/task/mobile-task-listing-display.spec.ts
```

## Results

Implemented as specified. Verified with the copied prod harman workspace on the
LAN instance (25 tasks, 45 edges, one parent frame). Desktop E2E: 21 passed,
including the existing Pipeline and view-preference specs; mobile E2E: 3 passed.
Follow-ups in the same work order: select-then-open on the board (selection bar,
double-click) and the Parallel steps toggle with aligned step bands; graph E2E
6 passed, mobile 1 passed.
