---
created: 2026-09-22
status: complete
requirements:
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
system_design:
  - ../../specs/ui/system-design/sidebar-task-row-presentation.md
legacy_specs: []
---

# Implementation Plan: Sidebar Title Width

## Overview

Reclaim unused desktop time-column width in one implementation work order.
The user selected content-sized time columns, stable hover geometry, adjacent PR icons,
and preserved mobile touch targets.

UI owns this existing reusable presentation contract. No task or provider state changes are needed.

## Evidence and correction

Both the relative-time wrapper and its child in TaskItemTrailing use w-11 (44px).
TaskMenuButton uses size-6 (24px) on desktop. TaskItemContent already uses flex-1;
its long title consumes the space left by badges and trailing content.
The earlier diagnosis blaming removal of w-full is withdrawn: restoring it would move
badges away from short titles without reclaiming the fixed time column.

This is a source-grounded explanation, not a measured browser reproduction.
The first implementation check must reproduce the gap with a long title and PR badge.

## Scope

### In scope

- Replace fixed desktop time widths with max(localized time width, 24px).
- Preserve stable title width during hover, focus, and menu use.
- Preserve accessible full time, invalid-time fallback, badges, and mobile navigation.
- Update tests that currently assert equal desktop time widths.

### Out of scope

- Title-wrapper growth changes, formatting, locale keys, saved settings, and backend changes.
- Git-change and change-request trailing layout redesign.
- Changing coarse-pointer action visibility or introducing new touch targets.

## Technical approach

In apps/web/components/task/task-item-trailing.tsx, scope intrinsic sizing and
a 24px minimum to fine pointers at widths of at least 640px. The child time must
also lose its fixed width in that condition. Keep the time in layout while hidden
by opacity; overlay the existing right-aligned menu. Avoid measurements, new state,
and observers. Keep mobile in-flow actions and existing classes compatible.

Use task-item-menu-button.tsx as the button-size reference and app/globals.css
as the phone override authority. Inspect actual rendered ancestors before changing
selectors. No shared ScrollOnOverflow changes are needed.

The existing requirement/design pair now records this intended behavior.
The completed sidebar-task-row-compact-trailing package records the old fixed-width
delivery; its results remain historical. This package supersedes only that sizing choice.

## ASCII UI preview

```text
UI-01 Desktop sidebar, long title
Before: [state] Long title... [PR] [unused   2h]
After:  [state] Longer title text... [PR] [2h]
Hover:  [state] Longer title text... [PR] [...]
Short:  [state] Short title [PR]          [2h]

UI-02 Phone task picker, inset bottom drawer
+------------------------------------------+
| Tasks                                    |
| [state] Title... [PR] [time] [44px action] |
|             vertically scrolling rows    |
+------------------------------------------+
```

Spacing is illustrative. UI-01 requires adjacent title badges and identical title geometry
between idle, hover, focus, and menu-open states. UI-02 keeps primary row navigation,
the existing safe-area handling, one internal scroll owner, and visible touch actions.
Criteria: AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.12, .14, .15, and .16.

## Tests

Component tests in task-item-trailing.test.tsx retain valid/invalid timestamp,
accessible full phrase, and menu behavior coverage for criterion .14.
Replace obsolete w-11 assertions; do not substitute class-only tests for geometry.

## E2E tests

- New e2e/tests/task/sidebar-title-width.spec.ts, desktop project: test
  "title width follows compact time without hover movement". Long and short titles,
  with and without PR badges, details on/off, 2h and wider localized tokens,
  narrow and expanded sidebars. Measure max(time glyph width, button width),
  title available width, badge adjacency, and unchanged rectangles across hover,
  keyboard focus, menu open, and close (.14/.15).
- Same file: test "title width respects action breakpoint". Check 639/640/768px
  with explicit fine/coarse pointer contexts and computed sizing; include
  representative CJK and pseudo-locale tokens (.12/.15/.16).
- Update sidebar-filter.spec.ts "task row presentation" scenario to remove the
  obsolete equal desktop-column width assertion (.14/.15).
- Extend mobile-sidebar-views.spec.ts "task row settings" scenario in mobile-chrome:
  visible 44px action, time/action non-overlap, control containment in row,
  no document overflow, and primary tap navigation (.12/.16).

Use existing seeded fixtures and causal waits. No user instance is needed.
Compare rendered desktop and phone screenshots with UI-01/UI-02.

## Work orders

- [x] [Task 01: Reclaim desktop title width](task-01-reclaim-title-width.md)

One wave, sequential execution, no dependencies.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/task-item-trailing.test.tsx components/task/task-item-compact-layout.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/task-item-trailing.tsx components/task/task-item-trailing.test.tsx e2e/tests/task/sidebar-title-width.spec.ts e2e/tests/task/mobile-sidebar-views.spec.ts e2e/tests/task/sidebar-filter.spec.ts)
make build-web
(cd apps/web && pnpm e2e:run tests/task/sidebar-title-width.spec.ts tests/task/sidebar-filter.spec.ts -- --grep "title width|task row presentation")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings|title width")
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Task 01 is complete. Component, rendered desktop, and mobile geometry checks passed;
the web build, typecheck, lint, specification validation, and whitespace check passed.
The desktop and phone captures were inspected against UI-01 and UI-02.

## Risks

- Localized tokens can exceed 24px; intrinsic sizing must allow expansion without clipping.
- Timestamp updates can change width; hover with an unchanged value must not.
- Phone CSS makes the menu in-flow; an unconditional desktop width can clip it.
- Existing tests encode equal widths and must change with the new contract.
