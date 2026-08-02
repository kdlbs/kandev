---
spec: docs/specs/ui/sidebar-diff-stat-priority.md
created: 2026-08-02
status: completed
---

# Implementation Plan: Sidebar Diff Stat Priority

## Overview

Refine the shared task-row visibility rules so a desktop row's diff totals remain the idle
affordance even after task selection leaves focus on the row. Keep the existing hover replacement
for the ellipsis, retain a focus-visible keyboard path on the action control itself, and leave the
mobile drawer's side-by-side controls unchanged.

## Frontend

### Shared sidebar task row

- Update `apps/web/components/task/task-item.tsx` so `DiffStatsRight` no longer hides totals for
  row-level `group-focus-within`; it should hide for an open menu, fine-pointer hover, or the
  named `group-focus-within/actions` state while the action trigger itself owns focus.
- Update `TaskMenuButton` so row-level focus does not make the ellipsis visible on rows with diff
  stats. Preserve a keyboard-focus selector on the button/control so Tab users can reveal and use
  the trigger, while rows without diff stats retain their existing row-focus disclosure.
- Keep the existing context-menu `menuOpen` and deletion behavior intact: an open menu remains
  associated with a visible trigger, while the normal idle state restores the totals.
- Do not alter the mobile classes or `apps/web/app/globals.css` overrides that intentionally show
  both controls and preserve the 44px action hit target below 640px.

## Tests

- **What:** an active/focused desktop row with diff stats keeps the totals in its idle visibility
  state; row focus is not included in either swapped-visibility class.
  **File:** `apps/web/components/task/task-item.test.tsx`.
  **How:** render a selected `TaskItem` with non-zero stats and assert its semantic elements and
  visibility-class contract.
- **What:** the context-menu-open state retains the existing visible action trigger and hides the
  underlying totals.
  **File:** `apps/web/components/task/task-item.test.tsx`.
  **How:** extend the action-state component coverage with non-zero diff stats.

## E2E Tests

- **Scenario:** an active desktop sidebar task preserves diff totals until hover, then reveals the
  action ellipsis and opens its context menu.
  **File:** `apps/web/e2e/tests/task/sidebar-diff-stats.spec.ts`.
  **What to verify:** after selecting a task with real non-zero status-summary stats, assert the
  idle action trigger is hidden, the diff badge is visible, and hover swaps the two before opening
  a menu item.
- **Scenario:** the phone task-switcher keeps diff totals and a non-overlapping touch-sized action
  control visible together.
  **File:** `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` (existing coverage).
  **What to verify:** retain the existing `mobile-chrome` geometry assertion; no mobile markup or
  interaction changes are planned.

## Mobile Parity

The nearest shipped mobile exemplar is
`apps/web/components/task/mobile/session-task-switcher-sheet.tsx`, which renders the shared
`TaskSwitcher` inside the phone task drawer. Desktop's hover replacement is not a valid touch
interaction, so the existing mobile hierarchy keeps both the diff badge and explicit overflow
button visible. The drawer remains the sole scroll owner; no composition, safe-area, or navigation
change is needed. Existing mobile E2E coverage proves the same task-action capability with a 44px
control and no overlap.

## Verification Results

- `rtk pnpm --filter @kandev/web test -- components/task/task-item.test.tsx` — final run passed,
  1 file / 30 tests. The initial RED run passed 27 tests and failed the new visibility assertion
  against the old `group-focus-within` classes.
- `rtk pnpm e2e:run tests/task/sidebar-diff-stats.spec.ts` — the first production-build run
  reached the new active-row assertion but failed because the test pointer was still hovering the
  clicked row. After moving the pointer away, the change-aware rerun
  `rtk pnpm e2e:run --no-build tests/task/sidebar-diff-stats.spec.ts` passed, 1 test; the final
  rerun also passed after asserting that the hovered trigger opens the existing context menu.
- `rtk pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts`
  — passed, 8 tests; the existing mobile diff/action geometry remains green.
- `rtk pnpm run typecheck` — passed with `tsc --noEmit` and no diagnostics.
- `rtk git diff --check` — passed.
- Fixup commit `8c0f56c0b` added scoped action-focus behavior, restored no-stats row-focus
  disclosure, and added the context-menu-open unit assertion; the targeted unit suite passed
  30/30, desktop E2E passed 1/1 after a production rebuild, mobile E2E passed 8/8, and web
  typecheck passed.
- `rtk pnpm e2e:run --no-build tests/task/sidebar-diff-stats-capture.spec.ts` — passed, 1
  desktop capture test; produced fresh idle and hover PNGs.
- `rtk pnpm e2e:run --no-build --project mobile-chrome
  tests/task/mobile-sidebar-diff-stats-capture.spec.ts` — final capture run passed, 1 test; the
  mobile screenshot was scrolled to show the task row's stats and action button together.
- The temporary capture specs were removed after capture. `apps/web/.pr-assets/manifest.json` is
  non-empty, PNGs were visually inspected, and the `pngquant-bin` fallback compressed them.

The fresh-worktree dependency bootstrap was not needed because `apps/node_modules` was already
present. E2E production assets were rebuilt by the first managed run; the later `--no-build` runs
used those fresh artifacts.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [Task 01: Preserve sidebar diff stats](task-01-preserve-sidebar-diff-stats.md) — done

## Open Questions

None.
