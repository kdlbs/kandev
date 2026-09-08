---
created: 2026-08-28
status: done
requirements:
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
system_design:
  - ../../specs/ui/system-design/sidebar-task-row-presentation.md
legacy_specs: []
---

# Implementation Plan: Compact Sidebar Trailing Content

## Overview

Refine the compact task-row trailing slot in one vertical implementation pass. The pass removes the
idle change-request menu gap and gives right-side time a short, stable column.

## Scope

### In scope

- Collapse the hidden fine-pointer menu width beside trailing change-request status.
- Reveal the menu on outer-row hover and keyboard focus without covering the status.
- Add a sidebar-only localized elapsed-time ladder for seconds through years.
- Give right-side time a fixed visual width and a full localized accessible name.
- Preserve phone task actions, row navigation, and document-width containment.

### Out of scope

- Changes to saved sidebar-view data or editor choices.
- Changes to provider status derivation or summary content.
- Changes to the shared full relative-time formatter or other time surfaces.
- A new mobile drawer, menu primitive, or task-row preference.

## Technical approach

### Compact elapsed formatter

Add `formatSidebarElapsedTime` to `apps/web/lib/i18n/formats.ts`. Use catalog-backed compact unit
tokens and the bucket rules from the system design. Clamp future values to zero, omit invalid input,
and cap displayed years at `99+`.

Add the six compact unit keys and the bounded-year form to every locale catalog. Keep the visual
tokens short in each language. Do not change `formatRelativeTime` or `formatRelativeCompact`.

### Trailing row geometry

Update `TaskItemTrailing` so a valid formatted time uses one fixed, right-aligned, tabular-number
column. Use the full `formatRelativeTime` phrase as its accessible name. Omit the column when the
compact formatter returns an empty value.

Make the menu wrapper beside `TaskItemChangeRequestStatus` collapse to zero width on idle
fine-pointer rows. Expand it on outer-row hover, menu-open state, or keyboard focus. Keep the status
mounted and interactive. Reuse the current mobile action classes so phone rows keep the visible
44 CSS pixel menu target.

### Test integration

Extend the formatter and trailing-component tests before production changes. Add rendered geometry
assertions to the existing desktop sidebar and PR tests. Extend the mobile sidebar-view scenario for
the touch action, fixed time width, primary row action, and horizontal containment.

## Tests

| Acceptance criterion                         | Evidence                                                                              |
| -------------------------------------------- | ------------------------------------------------------------------------------------- |
| `AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.8`  | `components/task/task-item-trailing.test.tsx`, `e2e/tests/pr/pr-status-badge.spec.ts` |
| `AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.13` | `lib/i18n/formats.test.ts`, `e2e/tests/task/sidebar-filter.spec.ts`                   |
| `AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.14` | `components/task/task-item-trailing.test.tsx`, desktop and mobile sidebar-view E2E    |

## E2E tests

Desktop Chromium will select a no-details row view. It will prove that compact time has no direction
copy and that all rendered time columns have equal width. A linked change-request row will prove
that the idle status reaches the trailing edge and hover reveals an operable menu beside it.

The `mobile-chrome` project will use the existing task-row settings drawer. It will prove that the
menu target remains visible and touch-sized, the row remains the primary action, and the page has no
horizontal overflow.

## Work orders

- [x] [Task 01: Refine compact trailing content](task-01-refine-compact-trailing-content.md)

## Verification results

- 84 focused Vitest tests passed across the formatter, trailing component, and task item suites.
- `pnpm run lint`, `pnpm run typecheck`, and `pnpm run i18n:check` passed.
- Managed Chromium E2E passed both task-row presentation and PR trailing-action scenarios.
- Managed mobile Chromium E2E passed the task-row settings, touch-target, navigation, and overflow scenario.
- Specification lint and the scoped whitespace check passed.

## Risks

- A zero-width wrapper can leave a focusable button visually hidden if focus expansion is incomplete.
- Localized unit tokens can exceed the visual column if a catalog uses full words.
- Hover geometry can cover the status or title if the action cluster uses absolute positioning.
