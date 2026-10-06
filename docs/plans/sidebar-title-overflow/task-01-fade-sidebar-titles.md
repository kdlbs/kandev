---
id: "01-fade-sidebar-titles"
title: "Fade overflowing sidebar titles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-TITLE-OVERFLOW-001
acceptance_criteria:
  - AC-UI-SIDEBAR-TITLE-OVERFLOW-001.1
  - AC-UI-SIDEBAR-TITLE-OVERFLOW-001.2
  - AC-UI-SIDEBAR-TITLE-OVERFLOW-001.3
  - AC-UI-SIDEBAR-TITLE-OVERFLOW-001.4
  - AC-UI-SIDEBAR-TITLE-OVERFLOW-001.5
  - AC-UI-SIDEBAR-TITLE-OVERFLOW-001.6
  - AC-UI-SIDEBAR-TITLE-OVERFLOW-001.7
system_design:
  - ../../specs/ui/system-design/sidebar-title-overflow.md
---

# Task 01: Fade Overflowing Sidebar Titles

## Summary

Add a conditional title fade using the existing clipping hook and hover scroller.
Deliver desktop and phone evidence in the same work order.

## In scope

- `TaskItemTitle` hook/ref integration and scoped mask CSS.
- Proportional fade sizing, hybrid fine-pointer hover, and forced-colors mask removal.
- Focused component tests and desktop/mobile browser checks.
- Screenshot inspection of light/dark default, selected, and hovered rows.

## Out of scope

- Changes to the shared scroller, row geometry, state, or preferences.
- Other labels, title surfaces, and new disclosure controls.

## Acceptance

- The real title span fades only during idle overflow and updates after resizing or title changes (.1/.2/.6).
- Existing hover scrolling reveals the ending, and full DOM text and adjacent metadata remain intact (.3/.4/.5).
- Forced-colors mode removes the mask; narrow title widths retain more than two-thirds of their width before fading (.1/.7).
- Phone task navigation and visible actions remain usable and contained (.5).

## ASCII UI preview

```text
UI-01 Desktop sidebar
[state] Investigate fresh session fa~~~| [PR] [7s]
[state] Short title [PR]                   [7s]

UI-02 Phone task picker, inset drawer
| [state] Investigate fresh sess~~~| [7s] [actions] |
|             vertically scrolling rows            |
```

Use the [full preview](plan.md#ascii-ui-preview) for annotations and before/hover states.
Only the title fades. The phone retains its current picker and visible touch actions.
Criteria: AC-UI-SIDEBAR-TITLE-OVERFLOW-001.1 through .7.

## Verification

Before the first package command in a fresh worktree, install dependencies from `apps`.
Write focused tests first and record their failure before production changes.
Run the following commands from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/task-item-title-fade.test.tsx hooks/use-is-title-truncated.test.tsx)
(cd apps/web && pnpm exec eslint components/task/task-item.tsx components/task/task-item-title-fade.test.tsx e2e/tests/task/sidebar-title-overflow.spec.ts e2e/tests/task/mobile-sidebar-title-overflow.spec.ts e2e/tests/task/sidebar-title-width.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-title-overflow.spec.ts tests/task/sidebar-title-width.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-title-overflow.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner rebuilds the application and enforces resource limits.
Keep desktop and phone runs sequential. Record screenshot paths and actual results.

## Files likely touched

- `apps/web/components/task/task-item.tsx`
- `apps/web/app/globals.css`
- `apps/web/components/task/task-item-title-fade.test.tsx` (new)
- `apps/web/e2e/tests/task/sidebar-title-overflow.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-sidebar-title-overflow.spec.ts` (new)
- `apps/web/e2e/tests/task/sidebar-title-width.spec.ts`
- This work order, its plan, and paired specification lifecycle fields.

## Dependencies

None.

## Risks

Confirm the actual outer span's overflow and mask behavior in Chromium and inspect the prefixed WebKit declaration.
Keep badge adjacency and short-title width unchanged.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-title-overflow.md).
- [System design](../../specs/ui/system-design/sidebar-title-overflow.md).
- `apps/web/hooks/use-is-title-truncated.ts` and its tests.
- `apps/packages/ui/src/scroll-on-overflow.tsx` (read-only reference).
- Existing `task-item.test.tsx`, `sidebar-title-width.spec.ts`, and `mobile-sidebar-views.spec.ts` patterns.

## Results

Implemented on 2026-10-06. `TaskItemTitle` now observes clipped width and applies a scoped alpha mask to overflowing title text. The existing hover scroller reveals the ending without a mask, while fitting titles and adjacent row content retain their current layout.

Verification passed:

- `pnpm exec vitest run components/task/task-item-title-fade.test.tsx hooks/use-is-title-truncated.test.tsx` (2 files, 8 tests).
- Scoped ESLint for `task-item.tsx`, the component test, both overflow E2E specs, and `sidebar-title-width.spec.ts`.
- `pnpm run typecheck`.
- Managed Chromium title-overflow and title-width specs (5 tests), including the forced-colors and narrow-width regressions.
- Managed Pixel 5 mobile title-overflow spec (1 test).
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, Prettier checks, and `git diff --check`.

Captured screenshots in `apps/web/.pr-assets/`:

- `sidebar-title-overflow--sidebar-title-light-rows.png`
- `sidebar-title-overflow--sidebar-title-light-hover.png`
- `sidebar-title-overflow--sidebar-title-dark-rows.png`
- `sidebar-title-overflow--sidebar-title-dark-hover.png`
- `mobile-sidebar-title-overflow--mobile-sidebar-title-overflow-picker.png`

Review-fixup checks: the forced-colors and narrow-width browser assertions failed on the original PR head for the expected mask values, then passed after the CSS changes. The six screenshots above were recaptured after fixup commit `f3afb57e`.
