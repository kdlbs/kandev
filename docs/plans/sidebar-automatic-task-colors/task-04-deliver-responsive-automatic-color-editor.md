---
id: "04-deliver-responsive-automatic-color-editor"
title: "Deliver responsive automatic-color editor"
status: done
wave: 4
depends_on:
  - "02-resolve-effective-task-colors"
  - "03-build-repository-target-catalog"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.1
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.3
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.4
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.5
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.1
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.11
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.12
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.13
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.14
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.15
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.1
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.3
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.4
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.5
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.6
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.8
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.5
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.6
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.8
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.9
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.10
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.11
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
---

# Task 04: Deliver Responsive Automatic-Color Editor

## Summary

Deliver the compact disclosure layout and the complete automatic-color editor. Prove the same user outcome on desktop and mobile.

## In scope

- Shared Sort, Group by, Task row, and Automatic colors disclosures.
- Global enable control, ordered rule cards, selectors, color outputs, and removal.
- Disabled incomplete-rule state, generated rule summaries, and the 50-rule limit state.
- Explicit Executor profile, Task state, Priority, Origin, and Kanban-origin options.
- Desktop repository popover and focused mobile repository pane.
- Unavailable targets from inactive workspaces.
- Localized copy in all shipped catalogs.
- Component, desktop Playwright, and mobile Playwright coverage.

## Out of scope

- New rule dimensions.
- Nested mobile drawers.
- Public documentation.

## Acceptance

- The compact editor shows accurate summaries without changing a view draft.
- Rule operations remain usable with keyboard, pointer, and touch input.
- Desktop and mobile flows persist rules and recolor tasks after fact changes.
- A second browser context receives the stored rule order, targets, and output colors.
- The manual color menu remains unchanged. Task 06 replaces its device-local persistence.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/sidebar-filter/sidebar-filter-popover.test.tsx components/task/sidebar-filter/task-row-settings.test.tsx components/task/sidebar-filter/sidebar-settings-disclosure.test.tsx components/task/sidebar-filter/automatic-color-settings.test.tsx components/task/sidebar-filter/task-color-rule-options.test.tsx)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/task/sidebar-automatic-colors.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-automatic-colors.spec.ts)
```

## Files likely touched

- `apps/web/components/task/sidebar-filter/sidebar-view-editor.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-filter-popover.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-settings-disclosure.tsx`
- `apps/web/components/task/sidebar-filter/task-row-settings.tsx`
- `apps/web/components/task/sidebar-filter/automatic-color-settings.tsx`
- `apps/web/components/task/sidebar-filter/automatic-color-rule-card.tsx`
- `apps/web/components/task/sidebar-filter/automatic-color-repository-picker.tsx`
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/task/sidebar-automatic-colors.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-automatic-colors.spec.ts`

## Dependencies

Task 02 supplies effective colors, dimension options, and identity matching. Task 03 supplies repository options and source status.

## Risks

- Dense rule cards can overflow the 22rem desktop popover.
- Mounting the parent editor during mobile picker navigation can create two scroll owners.
- A stored target from another workspace needs a clear unavailable label without changing the rule.
- New copy must remain complete in five locales.

## Parallelism

`sequential`

## Inputs

- Editor composition and responsive sections in the system design.
- Current Task row disclosure and the drawer in `sidebar-filter-popover.tsx`.
- Focused navigation behavior from `mobile-picker-sheet.tsx` without a nested drawer.
- Mobile UI language and E2E sidebar guidance.

## Results

Implemented shared settings disclosures, the global automatic-color editor, ordered drag and touch actions, disabled incomplete rules, scalar and repository selectors, unavailable-target retention, localized copy, and the focused mobile repository pane. Added desktop and mobile Playwright coverage for saved rules, recoloring, repository selection, and reload persistence.

Verification: the component suite passed; `pnpm run i18n:zh-hant` and `pnpm run i18n:check` passed; both automatic-color Playwright specs passed.
