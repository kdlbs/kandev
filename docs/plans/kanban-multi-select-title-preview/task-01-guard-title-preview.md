---
id: "01-guard-title-preview"
title: "Guard Kanban title previews in selection mode"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RICH-TASK-TITLE-PREVIEWS-001
acceptance_criteria:
  - AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.2
  - AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.3
  - AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.4
system_design:
  - ../../specs/tasks/system-design/rich-task-title-previews.md
---

# Task 01: Guard Kanban title previews in selection mode

## Summary

Suppress the Kanban title preview while multi-select is active so its popover cannot block nearby cards. Preserve title clicks as selection actions and restore the preview after leaving multi-select.

## In scope

- Gate `KanbanCardBody` title preview with the already supplied `isMultiSelectMode` prop in `KanbanCardShell`, and gate the Pipeline row's `CardTitle` through `RowInfoColumn`.
- Add a failing component regression before the production change, then a browser regression for closing the preview and selecting through a title.
- Check the existing phone title-navigation scenario after the change.
- Add a Pipeline-row regression proving its title preview is disabled only while multi-select is active.

## Out of scope

- Shared `TaskTitleHoverCard` behavior on other surfaces, PR/MR badge hovers, and new selection state.

## Acceptance

1. Enabling multi-select removes the Kanban title preview trigger and closes an open title popover for any card.
2. Hovering or focusing a title cannot reopen the preview in multi-select; clicking the title changes selection without task navigation.
3. Disabling multi-select restores the preview, and coarse-pointer direct title navigation still works.

## ASCII UI preview

`UI-01: Kanban card title`, desktop selection mode, excerpt from the [full plan](plan.md#ascii-ui-preview).

```text
[x] Long task title...       no title popover
[ ] Next task               remains clickable
```

`UI-02: Phone Kanban card`, coarse pointer, unchanged.

```text
[ ] Long task title...       tap uses existing card action
```

These structural outcomes map to `AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.2` through `.4`. Spacing is illustrative; no new control or copy is required.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/kanban-card-regression.test.tsx components/kanban-card-click.test.ts
cd apps/web && pnpm exec vitest run components/kanban/pipeline-kanban-shared-source.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run --host --project chromium tests/kanban/task-title-hover-subtasks.spec.ts
cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/kanban/mobile-task-title-hover-subtasks.spec.ts
```

The managed E2E runner must rebuild the web bundle before Playwright runs against the Go backend. Record each result here.

## Files likely touched

- `apps/web/components/kanban-card-content.tsx`
- `apps/web/components/kanban-card-regression.test.tsx`
- `apps/web/e2e/tests/kanban/task-title-hover-subtasks.spec.ts`
- `apps/web/components/kanban/graph2-task-pipeline-row.tsx`
- `apps/web/components/kanban/pipeline-kanban-shared-source.test.tsx`

## Dependencies

None.

## Risks

- A preview open when mode changes must unmount cleanly and release focus or pointer interception.
- The title click must bubble to the card's selection handler without a nested preview trigger consuming it.

## Parallelism

`sequential`

## Inputs

- [Rich task title preview requirements](../../specs/tasks/requirements/rich-task-title-previews.md).
- [Rich task title preview system design](../../specs/tasks/system-design/rich-task-title-previews.md).
- Existing `KanbanCardShell`, `CardTitle`, and Kanban title-hover tests.

## Results

Implemented the selection-mode guard by passing `!isMultiSelectMode` to `KanbanCardBody.enableTitleHover`. The component test failed for selected and unselected cards before the production change. The desktop browser regression proved that entering selection mode closes the active preview, title clicks select without navigation, and the preview returns after selection mode ends.

The review also identified the separate Pipeline row renderer. It now passes `isMultiSelectMode` through `RowInfoColumn` to gate its title preview. The system design and this work order now describe both title-rendering paths.

- RED: `pnpm exec vitest run components/kanban-card-regression.test.tsx` failed on both new selection-mode cases before the production change.
- `pnpm exec vitest run components/kanban-card-regression.test.tsx components/kanban-card-click.test.ts` - passed (2 files, 18 tests).
- `pnpm run typecheck` - passed.
- `pnpm e2e:run --host --project chromium tests/kanban/task-title-hover-subtasks.spec.ts` - passed (4 tests; managed host build and production Vite bundle).
- `pnpm e2e:run --host --project mobile-chrome tests/kanban/mobile-task-title-hover-subtasks.spec.ts` - passed (1 test; managed host build and production Vite bundle).
- RED: `pnpm exec vitest run components/kanban/pipeline-kanban-shared-source.test.tsx -t 'Pipeline title preview in multi-select mode'` failed because the Pipeline title trigger remained mounted in multi-select.
- `pnpm exec vitest run components/kanban/pipeline-kanban-shared-source.test.tsx -t 'Pipeline title preview in multi-select mode'` - passed (2 tests).
- `pnpm exec vitest run components/kanban-card-regression.test.tsx components/kanban-card-click.test.ts components/kanban/pipeline-kanban-shared-source.test.tsx` - passed (3 files, 22 tests).
- `pnpm run typecheck` after the Pipeline guard - passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` after the design update - passed.
