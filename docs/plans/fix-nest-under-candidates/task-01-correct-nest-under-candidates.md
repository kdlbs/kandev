---
id: "01-correct-nest-under-candidates"
title: "Correct Nest under candidates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001
acceptance_criteria:
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.2
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.3
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.5
system_design:
  - ../../specs/tasks/system-design/subtask-reparenting-drag-drop.md
---

# Task 01: Correct Nest under candidates

## Summary

The sidebar context menu and drag interaction offer the same eligible tasks from the rendered group,
including arbitrary-depth Office targets, while Kanban keeps its one-level hierarchy.

## In scope

- Preserve the backend Office discriminator through HTTP mapping, lightweight WebSocket merges,
  active/snapshot reconciliation, and the sidebar projection.
- Make `computeNestCandidates` distinguish Office from Kanban and reject known descendants.
- Supply the rendered group to the context menu and remove its independent raw-snapshot lookup.
- Keep drag target discovery on the same group and helper.
- Add focused unit, component, and Office E2E regression coverage.

## Out of scope

- Backend validation, task PATCH semantics, persistence, and WebSocket contracts.
- Kanban card drag, sibling reorder, Office list parent picker, menu copy, and menu geometry.
- Lifting the one-level Kanban hierarchy limit.

## Acceptance

- A visible eligible task remains selectable from `Nest under` when the corresponding raw
  multi-workflow snapshot is partial, and drag exposes exactly the same candidate identifiers.
- Office subjects with children and Office targets at any depth are eligible except for self,
  current parent, and descendants; the existing Kanban root-only and childless-subject behavior
  remains unchanged.
- The focused frontend suites, Office browser scenario, typecheck, lint, i18n ratchet, and spec
  validation pass.

## ASCII UI preview

### UI-01: `Nest under` submenu with a visible eligible Office target

See the combined preview in
[plan.md](plan.md#ui-01-nest-under-submenu-with-a-visible-eligible-office-target).

```text
Before                         After
+--------------------+        +--------------------+
| No other tasks     |        | Office target      |
| disabled           |        | selectable         |
+--------------------+        +--------------------+
```

Desktop keeps its anchored submenu. Phone uses the same candidate row in the existing inset menu
sheet. No spacing, control, or navigation change is part of this work order.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- lib/kanban/map-task.test.ts lib/ws/handlers/tasks-office-identity.test.ts lib/sidebar/nest-candidates.test.ts components/task/task-session-sidebar-aggregate.test.ts components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts components/task/task-switcher-subtask-dnd.test.ts components/task/task-switcher-context-menu.test.tsx components/task/task-switcher-nest-context-menu.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run tests/task/sidebar-nest-task.spec.ts tests/task/office-sidebar-nest-task.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-subtask-reparent-drag-drop.spec.ts tests/task/mobile-office-sidebar-nest-task.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/fix-nest-under-candidates apps/web
```

Run the complete block from the repository root.

## Files likely touched

- `apps/web/lib/kanban/map-task.ts`
- `apps/web/lib/kanban/map-task.test.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/ws/handlers/task-merge.ts`
- `apps/web/lib/ws/handlers/tasks-office-identity.test.ts`
- `apps/web/lib/sidebar/nest-candidates.ts`
- `apps/web/lib/sidebar/nest-candidates.test.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-session-sidebar-aggregate.ts`
- `apps/web/components/task/task-session-sidebar-aggregate.test.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-session-sidebar-item.test.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.test.ts`
- `apps/web/components/task/task-switcher-tree.tsx`
- `apps/web/components/task/task-switcher-row.tsx`
- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher-context-menu-items.tsx`
- `apps/web/components/task/task-nest-context-menu.tsx`
- `apps/web/components/task/task-switcher-context-menu.test.tsx`
- `apps/web/components/task/task-switcher-nest-context-menu.test.tsx`
- `apps/web/components/task/task-switcher-subtask-dnd.tsx`
- `apps/web/components/task/task-switcher-subtask-dnd.test.ts`
- `apps/web/e2e/tests/task/office-sidebar-nest-task.spec.ts`
- `apps/web/e2e/tests/task/mobile-office-sidebar-nest-task.spec.ts`

Use fewer files when the rendered-group value can be threaded through an existing context or prop.
Do not introduce a general task registry for this local composition.

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- Requirement acceptance criteria and scenarios in
  `docs/specs/tasks/requirements/subtask-reparenting-drag-drop.md`.
- Candidate-source and eligibility contracts in
  `docs/specs/tasks/system-design/subtask-reparenting-drag-drop.md`.
- Existing canonical mutation path in `apps/web/hooks/use-nest-task.ts` and backend validation in
  `apps/backend/internal/task/service/service_tasks.go`.

## Risks

- A cycle filter based only on direct children would expose deeper descendants in Office trees;
  follow ancestor chains with a visited set.
- Defaulting a missing Office discriminator to true would relax Kanban rules; absent values remain
  non-Office.
- A menu-local store selector would let menu and drag diverge again; both consume the rendered
  group supplied by their common tree composition.

## Results

Implemented the shared rendered-group candidate path, Office-aware hierarchy filtering, Office
identity projection and partial-update preservation, and descendant-cycle prevention. The new
browser scenario proves an Office parent with a child can be nested under another Office task,
persists the relationship, renders depth two, and survives reload.

Verification passed: frozen install; 115 focused Vitest tests across nine files; TypeScript
typecheck; full frontend lint; i18n ratchet; two Chromium desktop E2E cases; three `mobile-chrome`
touch E2E cases; both specification validators; and the scoped diff check.
