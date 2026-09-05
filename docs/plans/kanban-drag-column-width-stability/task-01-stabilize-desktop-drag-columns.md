---
id: "01-stabilize-desktop-drag-columns"
title: "Stabilize desktop drag columns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
acceptance_criteria:
  - AC-UI-ADAPTIVE-KANBAN-001.2
  - AC-UI-ADAPTIVE-KANBAN-001.3
  - AC-UI-ADAPTIVE-KANBAN-001.4
  - AC-UI-ADAPTIVE-KANBAN-001.9
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
---

# Task 01: Stabilize Desktop Drag Columns

## Summary

Move the desktop drag end reserve after the overflowing grid tracks. Add browser geometry evidence and preserve the current drag-anchor behavior.

## In scope

- Replace drag end padding with a trailing spacer after the lane grid in `AdaptiveDesktopKanban`.
- Update the focused component tests for the drag-only style.
- Add a Chromium E2E assertion for stable column width during drag.
- Keep the existing auto-hidden destination, cancellation, and drop assertions green.

## Out of scope

- Changes to column minimum width or responsive breakpoints.
- Changes to `useKanbanDragScrollAnchor` behavior.
- Changes to phone or tablet layouts.
- Changes to backend or persistence contracts.

## Acceptance

- The regression test fails because the current drag padding changes a column from 362.66px to 280px.
- A drag with an unchanged rendered step set keeps the source column width within one CSS pixel.
- A board wider than the viewport retains the full drag reserve after its overflowing tracks.
- The focused scenario still restores auto-hidden destinations and moves a task into one destination.

## Verification

```bash
(cd apps/web && pnpm test -- --run components/kanban/adaptive-desktop-kanban.test.tsx components/kanban/kanban-grid-template.test.ts)
(cd apps/web && pnpm exec eslint components/kanban/adaptive-desktop-kanban.tsx components/kanban/adaptive-desktop-kanban.test.tsx components/kanban/kanban-grid-template.test.ts e2e/tests/kanban/auto-hide-empty-columns.spec.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/auto-hide-empty-columns.spec.ts)
```

## Files likely touched

- `apps/web/components/kanban/adaptive-desktop-kanban.tsx`
- `apps/web/components/kanban/adaptive-desktop-kanban.test.tsx`
- `apps/web/components/kanban/kanban-grid-template.test.ts`
- `apps/web/e2e/tests/kanban/auto-hide-empty-columns.spec.ts`

## Dependencies

None.

## Risks

- The end reserve must remain in the scroll overflow area after it leaves the grid content box.
- The E2E assertion must allow subpixel layout values without hiding a real width change.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/adaptive-kanban.md`
- `docs/specs/ui/system-design/adaptive-kanban.md`
- `apps/web/components/kanban/adaptive-desktop-kanban.tsx`
- `apps/web/hooks/domains/kanban/use-kanban-drag-scroll-anchor.ts`
- `apps/web/e2e/tests/kanban/auto-hide-empty-columns.spec.ts`

## Results

- Replaced the drag-only grid end padding with a trailing spacer after the lane grid.
- Added component coverage for the spacer and the absence of padding.
- Added Chromium geometry and overflow-track scroll-range assertions.
- Initial focused unit and production-build Chromium checks passed before review feedback.
