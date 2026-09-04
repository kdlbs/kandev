---
created: 2026-08-31
status: done
requirements:
  - REQ-UI-TASK-LAYOUT-PROFILES-001
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
legacy_specs: []
---

# Implementation Plan: Custom Default Layout Proportions

## Overview

Restore the saved proportions when a custom profile supplies the effective desktop default. One work order adds regression evidence before it changes the shared default-layout path.

## Confirmed root cause

The saved profile contains its column widths and nested split sizes. Explicit profile selection resolves those values for the current workbench.

Fresh task setup and Reset Layout use `performBuildDefault`. This function passes an empty pinned-width map to `applyLayout`. The layout manager then substitutes responsive built-in widths for pinned columns. A fast environment switch can take the same shortcut without calling `performBuildDefault`, so it must receive the same resolved custom-default widths.

## Scope

### In scope

- Preserve saved custom-default proportions for fresh desktop task environments.
- Preserve the same proportions after Reset Layout.
- Preserve the proportions when a fast switch enters an unsaved environment.
- Scale complete saved geometry to the current workbench before safety caps apply.
- Keep the runtime pinned-width state consistent with the applied geometry.

### Out of scope

- Changes to the saved-layout JSON format or backend persistence.
- Changes to explicit saved-profile selection, which already restores saved proportions.
- Changes to per-environment manual sash widths.
- Changes to mobile or tablet task layouts.

## Technical approach

Update `performBuildDefault` in `apps/web/lib/state/dockview-store.ts`. Detect when the base state comes from `userDefaultLayout` instead of an intent or built-in preset. Pass the same resolver result into the fast environment-switch path for an unsaved environment.

For that path, call `resolveCustomLayoutPinnedWidths` with the final state and measured workbench width. Pass the result to `applyLayout` and store it in `pinnedWidths`.

Keep an empty pinned-width map for code-defined presets and named intents. This rule preserves their current responsive width behavior.

## Tests

- **AC-UI-TASK-LAYOUT-PROFILES-001.9:** Add a focused test to `apps/web/lib/state/dockview-preset-persistence.test.ts`. The test proves that Reset Layout sends scaled custom-default widths to `applyLayout`.
- **AC-UI-TASK-LAYOUT-PROFILES-001.10:** Use a saved 700/300 profile on an 800px workbench. The expected right width is 240px before the safety caps.
- Add focused fast-switch coverage in `apps/web/lib/state/dockview-env-switch-pinned.test.ts` for an unsaved environment with a custom default.
- Keep existing built-in and explicit custom-layout tests green.

## E2E tests

- **AC-UI-TASK-LAYOUT-PROFILES-001.9 and AC-UI-TASK-LAYOUT-PROFILES-001.10:** Update `apps/web/e2e/tests/settings/layout-profiles.spec.ts`.
- Extend the fresh-default and Reset Layout scenario with captured center and right widths.
- Assert the saved right-column proportion after fresh setup and after Reset Layout.

## Mobile and tablet

`TaskLayout` selects separate mobile and tablet compositions before the desktop Dockview workbench. This repair changes only desktop Dockview state normalization.

The nearest mobile exemplar is `apps/web/components/task/mobile/session-mobile-layout.tsx`. It keeps its existing single-panel navigation and scroll ownership. No mobile Playwright case is required for this state-only desktop repair.

## Work orders

- [x] [Task 01: Restore custom-default proportions](task-01-restore-custom-default-proportions.md)

## Verification results

- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/dockview-preset-persistence.test.ts lib/state/dockview-store.test.ts` passed (44 tests).
- `cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts -- --grep "fresh tasks use the no-terminal default while existing tasks wait for Reset Layout"` passed (1 test).
- `cd apps/web && pnpm run typecheck` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed (61 tests).
- `node scripts/validate-public-docs.mjs` passed (41 pages).

## Risks

- A named intent must not inherit widths from an unrelated custom default.
- The stored pinned-width map must match the applied geometry before post-layout enforcement runs.
- Built-in presets must retain their responsive width defaults.
