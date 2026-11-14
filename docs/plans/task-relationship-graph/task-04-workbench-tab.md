---
id: "04-workbench-tab"
title: "Move the graph into a workbench tab with Show in graph"
status: done
wave: 4
depends_on:
  - "03-graph-dialog-renderer"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-006
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-009
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.5
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 04: Move the graph into a workbench tab with Show in graph

## Summary

On desktop, show the graph as a `task-graph` dockview tab beside Chat instead of
a modal, and add Show in graph to the sidebar and Kanban task menus. Phones keep
the full-screen sheet.

## In scope

- `task-graph` panel registration, store action, and `TaskGraphPanel` content.
- `linkToTask` `panel` option and `TaskGraphPanelRequest`.
- Status-row button and chip action open the tab on desktop, the sheet on phones.
- Show in graph in the sidebar context menu and the Kanban card menu.
- Anchor wide or tall graphs at the top-left step.

## Out of scope

- A top-level graph page outside the task workbench.

## Acceptance

- The tab opens beside Chat, is reused, follows the active task, and survives
  layout save/restore.
- Show in graph from a sidebar row or a Kanban card opens that task with the
  graph shown and focused.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/task-graph lib/state components/task/task-switcher components/kanban-card
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run --docker --project chromium -- tests/task/task-graph.spec.ts
cd apps/web && pnpm e2e:run --docker --project mobile-chrome -- tests/task/mobile-task-graph.spec.ts
```

## Results

Implemented the `task-graph` dockview panel, `addTaskGraphPanel`,
`TaskGraphPanel`, `TaskGraphPanelRequest`, `useOpenTaskGraph`, the
`linkToTask` `panel` option, Show in graph in the sidebar and Kanban card
menus, and focus-centred fitting. Verified against the real harman tasks on the
LAN instance, which exposed two races the E2E fixtures cannot: an environment
layout restore and a first-visit environment adoption both replaced the newly
added tab. Both are handled in `TaskGraphPanelRequest` and unit-tested.

Validation passed:

- `pnpm exec vitest run components/task-graph hooks/domains/task components/kanban-card-menu components/task/task-dependency-chip components/task/task-switcher lib/state/dockview-task-graph-panel.test.ts lib/links.test.ts components/kanban`
- `pnpm exec tsc --noEmit`, ESLint on changed files, `pnpm run i18n:check`
- Desktop E2E (Docker): `task-graph`, `task-dependencies`, `sidebar-layout` specs, 22 passed
- Mobile E2E (Docker): `mobile-task-graph`, `mobile-task-dependency-navigation`, passed
