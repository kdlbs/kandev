---
created: 2026-09-05
status: complete
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
legacy_specs: []
---

# Implementation Plan: Kanban Drag Column Width Stability

## Overview

Correct GitHub issue #3345 without removing drag scroll anchoring. Move the drag end reserve outside the grid sizing box and add regression evidence.

## Confirmed root cause

`AdaptiveDesktopKanban` applies `padding-inline-end: calc(100% - 280px)` while a drag is active. This padding reduces the grid content box.

The grid uses `repeat(N, minmax(280px, 1fr))`. The reduced content box removes free space from the `1fr` tracks and compresses each column to 280px.

A focused Chromium reproduction measured a source column at 362.66px before drag and 280px during drag.

## Scope

### In scope

- Keep the width of existing desktop columns stable when drag state does not change the rendered steps.
- Preserve the end scroll range that supports drag scroll anchoring.
- Preserve auto-hidden drag destinations, cancellation, and successful drops.
- Add component and Chromium E2E regression coverage.

### Out of scope

- Changes to the 280px readable minimum.
- Changes to the drag-and-drop model or task move API.
- Changes to auto-hide or manual-hide rules.
- Changes to tablet or phone composition.
- Public documentation changes.

## Technical approach

### Grid end reserve

Update `apps/web/components/kanban/adaptive-desktop-kanban.tsx`. Replace the drag end padding with logical end margin.

The margin must remain conditional on `isDragging`. It must extend the scrollable area without changing the lane grid content box.

Keep `useKanbanDragScrollAnchor` unchanged. Its source-position capture and `scrollLeft` restoration still own drag anchoring.

### Regression tests

Update the focused component tests to assert the margin style and the absence of end padding.

Extend `apps/web/e2e/tests/kanban/auto-hide-empty-columns.spec.ts`. Measure a visible column before drag and after drag activation.

The E2E test must fail on the current padding behavior. It must continue through auto-hide enablement, cancellation, and a successful drop.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-ADAPTIVE-KANBAN-001.2` | `kanban-grid-template.test.ts` keeps the distributed `minmax(280px, 1fr)` template. |
| `AC-UI-ADAPTIVE-KANBAN-001.3` | The existing Chromium scenario proves contained horizontal lane scrolling. |
| `AC-UI-ADAPTIVE-KANBAN-001.4` | The existing Chromium scenario completes drag cancellation and a successful task move. |
| `AC-UI-ADAPTIVE-KANBAN-001.9` | The Chromium geometry assertion compares the same column before and during drag. |

## E2E tests

Use the Chromium project for `apps/web/e2e/tests/kanban/auto-hide-empty-columns.spec.ts`.

The scenario uses a 1440px desktop viewport and three visible columns. It compares the source column width before and during drag.

No new mobile test is required because this correction changes only `AdaptiveDesktopKanban`. The existing mobile auto-hide scenario remains the parity evidence.

## Work orders

- [x] [Task 01: Stabilize desktop drag columns](task-01-stabilize-desktop-drag-columns.md)

## Verification results

- The focused component suite passed with 10 tests.
- The Chromium production-build scenario passed with one test.
- Scoped ESLint, web type checking, and the i18n ratchet passed.

## Risks

- Removing the reserve instead of relocating it can break source-column anchoring when auto-hidden destinations appear.
- A style-only component test cannot prove browser geometry. The Chromium assertion supplies that evidence.
