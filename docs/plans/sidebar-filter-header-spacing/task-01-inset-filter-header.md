---
id: "01-inset-filter-header"
title: "Inset sidebar filter header"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.8
  - AC-UI-CONTROL-SIZING-001.4
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 01: Inset Sidebar Filter Header

## Summary

Give the desktop Filters heading and Add button a visible top inset below the View divider, and preserve a 44px Add hit area in the touch drawer.

## Scope

Add focused rendered regression coverage, then change the Filters section's vertical classes in `apps/web/components/task/sidebar-filter/sidebar-view-editor.tsx`. Preserve filtering behavior, section order, bottom separator, and scrolling ownership. Keep the compact Add styling while applying a 44px minimum hit area in the touch drawer. No copy, settings, API, or backend changes.

## Acceptance

- In the desktop popover, the rendered Filters label starts at least 8 CSS pixels below the View divider; the Add button stays entirely within the Filters section and below that divider.
- The phone drawer keeps the same visible inset, at least a 44 CSS pixel Add hit target, one scrolling editor body, and no horizontal viewport overflow.
- Sort remains immediately after Filters, and an overflowing editor still scrolls to its later settings and actions.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). The empty Filters state is the regression target.

```text
UI-01 desktop: View row | divider | inset | FILTERS  + Add | Sort
UI-02 phone:   drawer header | View row | divider | inset | FILTERS  + Add
```

Spacing is illustrative. The visible separation and contained action are required by `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.8`.

## Test-first implementation

1. Add a focused desktop scenario in `apps/web/e2e/tests/task/sidebar-filter-spacing.spec.ts`. Open the Tasks view gear in the desktop shell with no clauses. Measure the Filters label, Add button, View row bottom edge, and Filters section bounds in the browser. Expect at least 8 CSS pixels between the View divider and label. The current `pt-0`/`-mt-1` combination should fail. Record the measured RED gap.
2. Add `apps/web/e2e/tests/task/mobile-sidebar-filter-spacing.spec.ts` using the existing phone task-picker entry. Assert the drawer's Filters gap, Add target size and containment, one scrolling editor body, and no document horizontal overflow. The existing inset should pass while the compact Add button's 24px target should fail.
3. Set the Filters top inset to `pt-2.5` on desktop and in the drawer, remove the desktop `-mt-1` header margin and Add button's `-my-1` margin, and give the drawer Add button a 44px minimum height. Run desktop GREEN and the phone scenario. Inspect a screenshot at each viewport and record the final gap.

## Likely files and dependencies

- `apps/web/components/task/sidebar-filter/sidebar-view-editor.tsx`
- `apps/web/e2e/tests/task/sidebar-filter-spacing.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-filter-spacing.spec.ts`
- `apps/web/e2e/tests/task/sidebar-filter-spacing-helpers.ts`

There are no work-order dependencies or parallel tasks. Existing page object: `apps/web/e2e/pages/sidebar-filter-popover.ts`.

## Exact verification

From the repository root, install dependencies once if `apps/node_modules` is missing. The E2E runner rebuilds the production web bundle unless `--no-build` is passed.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-filter-spacing.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-filter-spacing.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/sidebar-filter/sidebar-view-editor.tsx e2e/tests/task/sidebar-filter-spacing.spec.ts e2e/tests/task/mobile-sidebar-filter-spacing.spec.ts e2e/tests/task/sidebar-filter-spacing-helpers.ts)
(cd apps/web && pnpm exec prettier --check components/task/sidebar-filter/sidebar-view-editor.tsx e2e/tests/task/sidebar-filter-spacing.spec.ts e2e/tests/task/mobile-sidebar-filter-spacing.spec.ts e2e/tests/task/sidebar-filter-spacing-helpers.ts)
git diff --check
```

Run the desktop command first after adding its test and before changing the classes to record RED. Rerun it after the change for GREEN. If the second E2E command uses `--no-build`, do so only after the desktop run has rebuilt the current code.

## Results

RED reproduced the desktop label at 1.5px above the View divider and the phone Add button at 24px tall. GREEN desktop and phone bounding-box checks confirmed the label and Add action each have at least 8px of inset, the phone Add target is at least 44px, and both controls stay within Filters. The desktop test passed with 16 clauses overflowing the popover and its last settings and view action reachable; the phone test passed with one scrolling editor body and no horizontal overflow. Both focused Playwright tests passed once each. Captured desktop and phone screenshots were visually inspected.

The managed E2E runner built the production web bundle. TypeScript typecheck, focused ESLint, Prettier, and `git diff --check` passed. The desktop and phone layout changes add 10px of top inset and a touch-only 44px minimum for Add; no remaining layout risk was found.

For PR assets, both viewport specs passed again with `CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project chromium tests/task/sidebar-filter-spacing.spec.ts` and `CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sidebar-filter-spacing.spec.ts`. The captures wait for finite CSS animations before saving. A fresh desktop run measured a 3.9px box offset between the Filters wrapper and its immediately adjacent Sort disclosure; the test allows 4px for that rendered geometry. Both final screenshots were inspected and show the intended inset with synthetic task data.
