---
spec: docs/specs/ui/requirements/adaptive-kanban.md
created: 2026-07-27
status: complete
---

# Implementation Plan: Adaptive Kanban

## Overview

First capture the narrow-desktop, preview-width, lane-scrolling, and long-parent-title regressions in
Playwright. Then replace viewport-category column sizing with readable desktop lanes whose overflow
is contained by the board, and turn subtask badge metadata into a contained relationship line. Phone
and tablet composition, backend contracts, and persisted settings remain unchanged.

## Frontend

### Container fit model

- `apps/web/components/kanban/kanban-grid-template.ts`: make the readable desktop column minimum a
  single invariant for every desktop lane grid.
- `apps/web/components/kanban/kanban-grid-template.test.ts`: cover the grid template for multi-lane
  and single-lane desktop boards.
- `apps/web/components/kanban-board-grid.tsx`: update the legacy board-grid call site to the shared
  invariant so the typechecked fallback cannot retain the old full-desktop squeeze behavior.

### Windowed desktop composition

- Add `apps/web/components/kanban/adaptive-desktop-kanban.tsx`: keep horizontal overflow inside the
  workflow and snap readable complete lanes into view, without a secondary desktop stage selector.
- `apps/web/components/kanban/swimlane-kanban-content.tsx`: compose desktop columns through the new
  wrapper and retain the current DnD handlers,
  multi-select handlers, orphan display, per-column vertical scroll, phone layout, and tablet layout.
- No changes are needed in `apps/web/hooks/use-kanban-layout.ts`: inline and floating preview behavior
  already changes the Kanban surface width; CSS grid sizing and the lane window contain any resulting
  horizontal overflow.

### Card hierarchy

- `apps/web/components/kanban-card-content.tsx`: render `Subtask of <parent>` as a left-aligned,
  full-width relationship line with a fixed icon/prefix and a truncating title. Keep session and
  review statuses in the existing wrapping status row. Expose a stable relationship test id and the
  full available parent title through accessible hover metadata.

No backend, API, state-store, or persistence changes are required.

## Mobile design contract

- **Desktop outcome:** readable complete columns at any board width; horizontal lane scrolling is the
  only compact-desktop navigation.
- **Mobile entry point and hierarchy:** unchanged `MobileColumnTabs` workflow/step control, inset
  navigator `Drawer`, one focused column, direct task-card navigation, and safe-area FAB.
- **Nearest shipped exemplars:** `mobile-column-tabs.tsx` contributes current-step hierarchy and
  count/WIP treatment; `TabletKanbanLayout` contributes snap-aligned lane scrolling;
  `use-kanban-layout.ts` and `kanban-header.tsx` contribute container measurement patterns.
- **Surface rationale:** desktop preserves direct lane navigation with its existing headers and
  horizontal scrolling; phone keeps the shipped temporary bottom drawer for its focused column.
- **Scroll and geometry:** each task column remains its vertical scroll owner; desktop horizontal
  overflow belongs only to the lane window; the page/document never gains horizontal overflow.
  Existing phone dynamic-viewport, safe-area, and 44px contracts remain unchanged.
- **Shared behavior:** task data, filtering, active-step derivation, DnD, moving, selection, and
  actions remain shared. Only responsive presentation differs, and no responsive state is persisted.
- **Mobile proof:** the existing `mobile-kanban.spec.ts` focused workflow/step, direct navigation,
  preference fallback, drawer geometry, and no-document-overflow scenarios run unchanged.

## Tests

- **Readable desktop invariant:** `apps/web/components/kanban/kanban-grid-template.test.ts` asserts
  every desktop grid uses the readable minimum.
- **Rendering-only card change:** no React component unit test is added; project TDD guidance routes
  card containment and truncation geometry to Playwright.

## E2E Tests

- **Constrained desktop:** extend
  `apps/web/e2e/tests/layout/compact-desktop-responsive.spec.ts` to seed a long-parent subtask,
  assert the absence of a desktop stage selector and readable column width, scroll to a distant
  lane, compare the relationship/card bounding boxes, and prove document horizontal overflow remains
  zero.
- **Wide desktop:** resize the same surface wide enough for all four seeded steps and assert no
  desktop stage selector is mounted while all columns remain attached.
- **Preview-driven width:** extend
  `apps/web/e2e/tests/kanban/kanban-board.spec.ts` so the existing preview-width scenario also proves
  the stage selector is absent while preview is inline and after it closes, without changing preview
  behavior.
- **Tablet parity:** the compact responsive spec asserts a 700px viewport still mounts the shipped
  two-column tablet layout and no desktop stage selector.
- **Phone parity:** run `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts` unchanged to prove the
  focused-column navigator, direct navigation, saved-view fallback, and no-document-overflow paths.

## Implementation waves and parallel candidates

Wave 1:

- [x] [Task 01 — Responsive browser contract](task-01-responsive-browser-contract.md) — RED contract recorded
  RED test task.

Wave 2:

- [x] [Task 02 — Adaptive desktop board](task-02-adaptive-desktop-board.md) — complete; depends on Task 01 and
  turns the browser and unit contracts green.

No tasks are parallel-safe: Task 02 consumes and completes the RED contract established by Task 01.

## Risks

- The lane window must remain the sole horizontal-scroll owner so the document never overflows.
- The adaptive wrapper mounts only on desktop; it must not interfere with mobile Embla state.
- Desktop DnD droppable IDs must remain owned only by real columns.

## Final refinement

After visual review, the compact desktop stage rail was removed. The completed implementation keeps
the readable-lane and contained-overflow guarantees while relying on direct horizontal lane scrolling
rather than a duplicate stage selector.
