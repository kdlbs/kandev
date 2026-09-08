---
created: 2026-08-26
status: complete
requirements:
  - ../../specs/ui/requirements/sidebar-task-focus.md
system_design:
  - ../../specs/ui/system-design/sidebar-task-focus.md
legacy_specs: []
---

# Implementation Plan: Sidebar Task Focus

## Overview

Strengthen the active task treatment in the shared sidebar row so the current
task remains easy to identify when a user assigns it a custom color. The work
is one sequential frontend slice: update the shared row presentation, then
prove the desktop sidebar and mobile task-switcher surfaces.

## Scope

### In scope

- Add a stronger theme-aware active-row surface with only top and bottom
  active-task borders.
- Preserve the existing task-color marker when a color is assigned, with no
  default left marker for uncolored tasks, and preserve all activation
  semantics.
- Verify the shared treatment in desktop and mobile task switchers.

### Out of scope

- Task-color picker or persistence changes.
- Active-task state, task/session data, or API changes.
- New mobile navigation or task-row interaction patterns.

## Technical approach

- Update `taskItemRowClassName` in `apps/web/components/task/task-item.tsx`
  so the existing `isSelected` state adds the active surface and horizontal
  borders. Keep `SelectionBar` conditional on an assigned task color and keep
  the multi-selection ring unchanged.
- Keep `TaskRowItem` and `MobileTaskList` data flow unchanged because both
  already reach the shared `TaskItem` with active-task state.
- Extend the existing desktop sidebar-open Playwright flow and mobile sidebar
  task-action flow to assert the active row's visible state and preserve the
  mobile no-overflow check.

## Tests

- `apps/web/components/task/task-item.test.tsx` remains a focused regression
  check for the shared row's existing state and action behavior. No React
  component test is added solely for static class markup.
- The desktop E2E flow maps to `AC-UI-SIDEBAR-TASK-FOCUS-001.1` and
  `AC-UI-SIDEBAR-TASK-FOCUS-001.2`, with its uncolored active-row assertion
  covering `AC-UI-SIDEBAR-TASK-FOCUS-001.5`.
- The mobile E2E flow maps to `AC-UI-SIDEBAR-TASK-FOCUS-001.3` and
  `AC-UI-SIDEBAR-TASK-FOCUS-001.4`, with the uncolored active-row flow also
  covering `AC-UI-SIDEBAR-TASK-FOCUS-001.5`.

## E2E tests

- `apps/web/e2e/tests/task/sidebar-task-open.spec.ts` shall assert the active
  row's `data-active` state, background, top-and-bottom-only borders, and no
  leading marker when opening an uncolored task.
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` shall assert
  the same active treatment inside the mobile task sheet for colored and
  uncolored tasks, and keep its document-horizontal-overflow assertion.

## Work orders

- [x] [Task 01: Strengthen active sidebar task focus](task-01-sidebar-task-focus.md) — complete

## Verification results

- Red E2E: the new desktop assertion failed on the previous `bg-primary/10`
  class, confirming the browser contract before implementation.
- `cd apps && pnpm --filter @kandev/web test -- components/task/task-item.test.tsx` —
  49 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-task-open.spec.ts` —
  2 tests passed.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --grep "active task"` —
  1 test passed.

## Background-only comparison variant

- Red desktop E2E failed on the existing active ring, confirming the new
  background-only contract before the production change.
- Desktop sidebar E2E — 2 tests passed.
- Mobile active-task E2E — 1 test passed.
- Typecheck, lint, focused unit tests (49 passed), and specification checks
  passed.

## Risks

- A class-order change could allow the generic hover background to hide the
  active surface. Keep the active hover class in the same active branch.
- Removing the active ring must not remove the separate multi-selection ring.
  Keep the multi-selection utility independent and verify both states.
- The shared row is mounted in more than one responsive surface. Run both
  desktop and mobile E2E projects against a fresh production build.

## Top-and-bottom border comparison variant

- The active row keeps the primary-tinted background and custom marker, with a
  1px primary-accent border on the top and bottom edges only.
- Desktop E2E first failed on the expected missing top border, then passed after
  adding the horizontal border utilities.

## Conditional marker refinement

- Red desktop E2E first failed because an uncolored active row still rendered
  the default left marker.
- `SelectionBar` now renders only for a supported, explicitly assigned task
  color. Uncolored active rows use the background and horizontal borders only.
- Desktop sidebar E2E — 2 tests passed, including the uncolored active-row
  assertion.
- Mobile active-task E2E — 2 tests passed for colored and uncolored rows,
  including the no-horizontal-overflow assertion.
- Focused unit tests (49 passed), typecheck, lint, and specification checks
  passed.
