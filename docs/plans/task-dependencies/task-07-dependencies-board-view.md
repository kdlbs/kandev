---
id: "07-dependencies-board-view"
title: "Dependencies board view"
status: dropped
wave: 7
depends_on: ["06-kanban-dependency-ui"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
---

# Task 07: Dependencies Board View

> **Dropped.** This view was implemented and then removed before shipping. Two
> reasons: it rendered edges as per-node text lists rather than drawn connectors,
> so a chain had to be read rather than followed; and it was scoped to one
> workflow, so it could not show a chain that crossed workflows. The per-task
> chip plus `list_related_tasks_kandev` cover reading the graph for now. If this
> is revived, draw real connectors with the existing
> `components/kanban/graph2-connector.tsx` primitives and add a focus-one-chain
> filter — and remember that a `VIEW_REGISTRY` entry is not reachable until the
> `TaskListingView` vocabulary and the header toggle learn about it.

## Acceptance

- A third `VIEW_REGISTRY` entry, "Dependencies", is registered in
  `apps/web/lib/kanban/view-registry.ts` alongside `kanban` and `graph2`, with
  its own stored value, label, and icon.
- The view renders the dependency DAG for the active workflow from
  `GET /api/v1/workflows/:id/dependencies` plus the tasks already in the store:
  one node per task, one directed connector per edge, drawn in dependency order.
- Blocked nodes are visually distinguished from unblocked ones, and
  `blocked_reason: "failed"` is distinguished from `"pending"`.
- Edges that leave the active workflow but stay in the workspace are marked as
  such rather than dropped, so a cross-workflow dependency is not invisible.
- Dragging from a node's outgoing handle to another node creates "target depends
  on source". Deleting a connector removes the edge. Both go through the same
  task-scoped endpoints and the same optimistic-mutation and cycle-error
  handling as the picker.
- A rejected edge (cycle, cross-workspace) reverts the optimistic connector and
  surfaces the formatted cycle path.
- A node click opens the task; the view does not become a second task editor.
- The view is desktop-only. `getEffectiveView` already forces the Kanban view on
  mobile; a test pins that the Dependencies view is not in the mobile view set.
- All new copy goes through `t()` / `<Trans>`, the new paths are appended to
  `i18nGuardFiles` in the same change, and the pseudo-locale shows no English
  in the view.
- No new charting or graph-layout dependency is added without first checking
  whether the existing connector primitives in `components/kanban/graph2-*.tsx`
  cover the need.

## TDD sequence

1. Failing test: the view builder turns an edge payload plus store tasks into
   nodes and directed edges in dependency order.
2. Failing test: blocked, failed, and unblocked nodes get distinct markers.
3. Failing test: a cross-workflow edge is marked, not dropped.
4. Failing test: drag-to-connect calls the create endpoint with the correct
   direction and applies the edge optimistically.
5. Failing test: a `409` with a `cycle` body reverts the connector and shows the
   formatted path.
6. Failing test: connector delete calls the delete endpoint and removes the
   edge.
7. Failing test: the Dependencies view is absent from the mobile view set.
8. Implement the registry entry, the view component, and the graph interactions.

## Verification

```bash
cd apps/web
pnpm test -- --run lib/kanban/view-registry components/kanban/dependencies
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/kanban/view-registry.ts`
- `apps/web/components/kanban/dependencies-*.tsx` (new)
- `apps/web/components/kanban/graph2-connector.tsx` (reuse, if extended)
- `apps/web/hooks/use-kanban-display-settings.ts`,
  `apps/web/lib/task-listing/view-preference.ts`
- `apps/web/lib/api/domains/` (workflow dependency-graph client)
- `apps/web/eslint.i18n.options.mjs`
- focused `*.test.tsx` / `*.test.ts` files

## Dependencies

Task 06 — reuses its API client, mutation handling, and cycle-error formatting.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed commands pass. Record whether existing connector primitives were reused
or a new layout approach was needed and why, the stored view value, the i18n
paths appended, and test results in this file and `plan.md`.
