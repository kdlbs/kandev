---
id: "01-refine-compact-trailing-content"
title: "Refine compact trailing content"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
acceptance_criteria:
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.8
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.13
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.14
system_design:
  - ../../specs/ui/system-design/sidebar-task-row-presentation.md
---

# Task 01: Refine Compact Trailing Content

## Summary

Implement the compact trailing-slot refinement as one responsive vertical slice. The result removes
idle change-request action space and standardizes right-side elapsed time.

## In scope

- Add the localized compact elapsed-time formatter and locale keys.
- Refine relative-time and change-request branches in `TaskItemTrailing`.
- Add unit, component, desktop E2E, and mobile E2E coverage.
- Perform focused rendered checks at desktop and phone widths.

## Out of scope

- Sidebar-view persistence and editor changes.
- Provider status or task-menu behavior changes.
- Formatting changes outside the sidebar trailing slot.

## Acceptance

- Idle fine-pointer rows reserve no hidden menu width beside change-request status. Hover and
  keyboard focus reveal an operable menu without covering the status.
- Right-side time uses the specified compact elapsed units, fixed width, full accessible phrase,
  and no direction or calendar words.
- Phone rows keep a visible 44 CSS pixel action, primary row navigation, and zero document overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run lib/i18n/formats.test.ts components/task/task-item-trailing.test.tsx components/task/task-item.test.tsx)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts tests/pr/pr-status-badge.spec.ts -- --grep "compact trailing|task row presentation")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings")
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/web docs/specs docs/plans
```

## Files likely touched

- `apps/web/lib/i18n/formats.ts`
- `apps/web/lib/i18n/formats.test.ts`
- `apps/web/components/task/task-item-trailing.tsx`
- `apps/web/components/task/task-item-trailing.test.tsx`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/src/locales/*/common.json`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`

## Dependencies

None.

## Risks

- Focus can reach a zero-width action before the wrapper expands.
- A translated compact unit can exceed the fixed visual width.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001` acceptance criteria 1.8, 1.13, and 1.14.
- The trailing layout, compact elapsed-time, responsive, and accessibility design sections.
- Existing `TaskItemTrailing`, task-row presentation, PR summary, and mobile sidebar tests.

## Results

Implemented the compact sidebar trailing contract:

- Added a sidebar-only localized elapsed-time formatter with seconds, minutes, hours, days, weeks,
  years, invalid-input omission, future-time clamping, and a `99+` year bound.
- Added plural-safe compact unit catalog entries for English, Portuguese, Simplified Chinese,
  Traditional Chinese, and the pseudo locale.
- Fixed the change-request action cluster so idle fine-pointer rows reserve no menu width and hover
  or focus reveals the menu beside the status.
- Kept the full localized relative phrase as the time accessible name and preserved 44 CSS pixel
  mobile actions and primary row navigation.
- Added focused unit, component, desktop E2E, and mobile E2E coverage.

Verification passed:

```text
pnpm --filter @kandev/web exec vitest run lib/i18n/formats.test.ts components/task/task-item-trailing.test.tsx components/task/task-item.test.tsx
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm e2e:run tests/task/sidebar-filter.spec.ts tests/pr/pr-status-badge.spec.ts -- --grep "compact trailing|task row presentation"
pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings"
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/web docs/specs docs/plans
```
