---
spec: docs/specs/ui/adaptive-kanban.md
created: 2026-07-27
status: complete
---

# Implementation Plan: Adaptive Kanban

## Overview

First capture the narrow-desktop, preview-width, stage-navigation, and long-parent-title regressions
in Playwright. Then replace viewport-category column sizing with a container-measured desktop board,
add the compact stage navigator, and turn subtask badge metadata into a contained relationship line.
Phone and tablet composition, backend contracts, and persisted settings remain unchanged.

## Frontend

### Container fit model

- `apps/web/components/kanban/kanban-grid-template.ts`: make the readable desktop column minimum a
  single invariant for full and compact desktop; add pure helpers for deciding when the measured
  board requires windowed composition and for resolving the leading column during horizontal scroll.
- `apps/web/components/kanban/kanban-grid-template.test.ts`: cover the grid template, exact fit
  threshold, unmeasured-width fallback, single-step behavior, and leading-column selection.
- `apps/web/components/kanban-board-grid.tsx`: update the legacy board-grid call site to the shared
  invariant so the typechecked fallback cannot retain the old full-desktop squeeze behavior.

### Windowed desktop composition

- Add `apps/web/components/kanban/adaptive-desktop-kanban.tsx`: measure the rendered workflow width
  with `ResizeObserver`, keep horizontal overflow inside the workflow, and render a restrained
  vertical stage navigator only when every column cannot fit. Navigator rows show the step color,
  title, tabular count/WIP label, current state, and at least a 44px hit target. Selecting a row
  scrolls its snap-aligned column into view; scrolling updates the active index.
- `apps/web/components/kanban/swimlane-kanban-content.tsx`: compose desktop columns through the new
  wrapper, reuse the existing transient active-column state, and retain the current DnD handlers,
  multi-select handlers, orphan display, per-column vertical scroll, phone layout, and tablet layout.
- No changes are needed in `apps/web/hooks/use-kanban-layout.ts`: inline and floating preview behavior
  already changes the Kanban surface width, which the new local measurement observes directly.

### Card hierarchy

- `apps/web/components/kanban-card-content.tsx`: render `Subtask of <parent>` as a left-aligned,
  full-width relationship line with a fixed icon/prefix and a truncating title. Keep session and
  review statuses in the existing wrapping status row. Expose a stable relationship test id and the
  full available parent title through accessible hover metadata.

No backend, API, state-store, or persistence changes are required.

## Mobile design contract

- **Desktop outcome:** readable complete columns and discoverable hidden stages at any board width;
  the stage navigator is inline desktop navigation, not a modal surface.
- **Mobile entry point and hierarchy:** unchanged `MobileColumnTabs` workflow/step control, inset
  navigator `Drawer`, one focused column, direct task-card navigation, and safe-area FAB.
- **Nearest shipped exemplars:** `mobile-column-tabs.tsx` contributes current-step hierarchy and
  count/WIP treatment; `TabletKanbanLayout` contributes snap-aligned lane scrolling;
  `use-kanban-layout.ts` and `kanban-header.tsx` contribute container measurement patterns.
- **Surface rationale:** desktop stages are frequent primary navigation and remain inline; phone
  keeps the shipped temporary bottom drawer because a persistent rail would steal the focal column.
- **Scroll and geometry:** each task column remains its vertical scroll owner; desktop horizontal
  overflow belongs only to the lane window; the page/document never gains horizontal overflow.
  Existing phone dynamic-viewport, safe-area, and 44px contracts remain unchanged.
- **Shared behavior:** task data, filtering, active-step derivation, DnD, moving, selection, and
  actions remain shared. Only responsive presentation differs, and no responsive state is persisted.
- **Mobile proof:** the existing `mobile-kanban.spec.ts` focused workflow/step, direct navigation,
  preference fallback, drawer geometry, and no-document-overflow scenarios run unchanged.

## Tests

- **Readable desktop invariant:** `apps/web/components/kanban/kanban-grid-template.test.ts` asserts
  every desktop grid uses the readable minimum and only enters windowed mode below the measured fit
  threshold.
- **Scroll synchronization:** the same unit file asserts leading-column selection from column
  offsets, including midpoint ties and empty offsets.
- **Rendering-only card change:** no React component unit test is added; project TDD guidance routes
  card containment and truncation geometry to Playwright.

## E2E Tests

- **Constrained desktop:** extend
  `apps/web/e2e/tests/layout/compact-desktop-responsive.spec.ts` to seed a long-parent subtask,
  assert the stage navigator and readable column width, navigate to a distant stage, compare the
  relationship/card bounding boxes, and prove document horizontal overflow remains zero.
- **Wide desktop:** resize the same surface wide enough for all four seeded steps and assert the
  stage navigator disappears while all columns remain attached.
- **Preview-driven width:** extend
  `apps/web/e2e/tests/kanban/kanban-board.spec.ts` so the existing preview-width scenario also proves
  the navigator appears while preview is inline and disappears after the preview closes, without
  changing preview behavior.
- **Tablet parity:** the compact responsive spec asserts a 700px viewport still mounts the shipped
  two-column tablet layout and no desktop navigator.
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

- Showing the navigator changes the width being measured. The fit decision must use the outer
  workflow width before subtracting navigator space to avoid resize hysteresis.
- Scroll synchronization must not fight user scrolling or mobile Embla state; the adaptive wrapper
  mounts only on desktop and updates the existing transient index from the leading snap position.
- `ResizeObserver` can report zero during initial layout. The grid minimum must independently prevent
  compression before the first positive measurement.
- Desktop DnD droppable IDs must remain owned only by real columns; navigator rows are selectors, not
  duplicate drop targets.
