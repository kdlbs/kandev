---
spec: docs/specs/ui/requirements/adaptive-kanban.md
created: 2026-08-21
status: complete
issue: https://github.com/kdlbs/kandev/issues/2893
---

# Implementation Plan: Kanban Large-Column Virtualization

## Overview

The board mounts one complete `KanbanCard` tree for every task in every rendered column. An isolated
production-build test mounted all 440 task titles and delayed board readiness from 1.24s to 5.66s.

This repair adds vertical windowing to the shared column scroll area. The repair applies to desktop,
tablet, and phone layouts because all three layouts use `KanbanColumn`.

## Frontend

### Virtualized column task list

- Add `@tanstack/react-virtual` to `apps/web/package.json` and update `apps/pnpm-lock.yaml`.
- Add `apps/web/components/kanban/virtualized-column-task-list.tsx`.
- Use the current column scroll element as the virtualizer scroll element.
- Use stable task IDs as item keys.
- Measure each rendered row because card height changes with badges, plugin slots, and the WIP divider.
- Render a small overscan to keep scrolling smooth without mounting the complete column.
- Keep the full ordered task ID list for shift selection and bulk-action order.
- Keep the queue divider inside its task row so its logical position and measured height stay correct.
- Keep the outer column as the drop target. The existing `DragOverlay` keeps the dragged card visible
  if vertical scrolling removes the source row.

### Kanban column integration

- Update `apps/web/components/kanban-column.tsx` to use the virtualized task list.
- Add a stable test ID to the existing vertical scroll owner.
- Preserve the header, counts, empty state, responsive composition, and card action props.
- Do not add user-facing text or a new mobile surface.

The phone contract does not change. Phone Kanban keeps one focused column, one vertical scroll owner,
the current navigator, and direct card navigation.

## Tests

This visual performance fault has no useful pure-function boundary. Playwright is the regression level
because it measures the mounted production DOM and operates the real card tree.

- **What:** existing drag, open, selection, and WIP behavior remains available for small columns.
- **Files:** existing Kanban E2E specifications.
- **How:** run the focused existing desktop, tablet, mobile, and WIP specifications after the repair.

## E2E Tests

- **Scenario:** A desktop column contains 440 inert tasks.
- **File:** `apps/web/e2e/tests/kanban/large-column-virtualization.spec.ts`.
- **What to validate:** fewer than 50 card bodies mount, the count remains 440, scrolling replaces the
  mounted window, and a reached card opens normally.

- **Scenario:** A phone column contains 440 inert tasks.
- **File:** `apps/web/e2e/tests/kanban/mobile-large-column-virtualization.spec.ts`.
- **What to validate:** the focused-column layout remains present, fewer than 100 card bodies mount,
  the document has no horizontal overflow, and a reached card opens by touch.

- **Scenario:** A coarse-pointer tablet column contains 440 inert tasks.
- **File:** `apps/web/e2e/tests/kanban/tablet-large-column-virtualization.spec.ts`.
- **What to validate:** the two-column snap-scrolling layout remains present, fewer than 50 card
  bodies mount, scrolling replaces the mounted window, and a reached card opens normally.

- **Shared setup:** `apps/web/e2e/tests/kanban/large-column-virtualization-helpers.ts` seeds tasks in
  bounded request batches and contains shared structural assertions.

## Verification Results

RED:

- `pnpm e2e:run tests/kanban/large-column-virtualization.spec.ts`: failed as expected before the
  implementation because 440 card bodies were mounted.
- `pnpm e2e:run --no-build --project mobile-chrome tests/kanban/mobile-large-column-virtualization.spec.ts`:
  failed as expected before the implementation because 440 card bodies were mounted.

GREEN:

- `pnpm e2e:run tests/kanban/large-column-virtualization.spec.ts tests/kanban/tablet-large-column-virtualization.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/wip-overflow-queue.spec.ts`:
  9 passed after review remediation.
- `pnpm e2e:run --no-build --project mobile-chrome tests/kanban/mobile-large-column-virtualization.spec.ts tests/kanban/mobile-kanban.spec.ts`:
  28 passed.
- `pnpm exec eslint components/kanban/swimlane-container.tsx components/kanban/virtualized-column-task-list.tsx e2e/tests/kanban/large-column-virtualization-helpers.ts e2e/tests/kanban/large-column-virtualization.spec.ts e2e/tests/kanban/mobile-large-column-virtualization.spec.ts e2e/tests/kanban/tablet-large-column-virtualization.spec.ts`: passed.
- `pnpm run typecheck`: passed.
- `pnpm run build:e2e`: passed.

The managed capture run produced and validated fresh synthetic desktop and phone PNGs in the ignored
`.pr-assets` directory. The disposable capture specs were removed afterward.

Diagnostic evidence before implementation:

- `pnpm e2e:run --no-build tests/kanban/issue-2893-repro.spec.ts`: passed, 1 test.
- 25 tasks mounted 25 title nodes. The board became visible in 1,238ms.
- 440 tasks mounted 440 title nodes. The board became visible in 5,658ms.
- The temporary diagnostic specification was removed after the run.

## Implementation Waves And Parallel Candidates

Wave 1:

- [completed] [task-01-virtualize-kanban-columns](task-01-virtualize-kanban-columns.md)

The task is sequential because the source change, package lock, and E2E contract form one TDD cycle.

## Risks

- Dynamic card heights can cause incorrect offsets if rows are not measured after plugin or status changes.
- A task move can change a row index. Stable task keys prevent measurement reuse for the wrong task.
- A drag can auto-scroll the source row outside the mounted window. The existing drag overlay must stay active.
- Queue-divider height belongs to the first queued task and must be part of that row measurement.

## Out Of Scope

- Backend response changes, including description removal or truncation.
- Pipeline and List view virtualization.
- New pagination controls or persisted scroll state.
