---
id: "02-fused-desktop-titlebar-layout"
title: "Fused desktop title-bar layout"
status: done
wave: 2
depends_on: ["01-pwa-overlay-capability"]
plan: "plan.md"
spec: "../../specs/pwa-window-controls-overlay/spec.md"
---

# Task 02: fused desktop title-bar layout

## Acceptance criteria

- When the overlay is visible, the shared sidebar, page, and task top bars become one draggable title-bar row. Every interactive descendant stays non-draggable and usable.
- The left and right system-control exclusion areas do not cover Kandev controls, including expanded and collapsed sidebar controls. Geometry changes do not create horizontal overflow.
- A desktop environment without the API, or with `visible=false`, keeps the existing 40px top bars. Mobile and tablet code paths stay unchanged.

## Verification

Write the Playwright scenarios first. Confirm the expected RED result against the current layout before changing production CSS or components. Run the command again after GREEN and REFACTOR:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts
```

Then run the repository checks required by the task:

```bash
make fmt
make typecheck test lint
```

Run the affected focused command again after every final production or test change. Record the result.

## Possible files to change

- `apps/web/components/app-sidebar/app-sidebar-header.tsx`
- `apps/web/components/app-sidebar/app-sidebar.tsx`
- `apps/web/components/page-topbar.tsx`
- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`
- `docs/plans/pwa-window-controls-overlay/plan.md`
- `docs/plans/pwa-window-controls-overlay/task-02-fused-desktop-titlebar-layout.md`

## Dependencies

Task 01 must provide the root overlay state and geometry-variable contract first.

## Parallel work

Run this task in sequence. The desktop application-bar components and E2E geometry assertions share one layout contract.

## Inputs

- All scenarios in the spec
- The `Root shell and desktop application bars`, `E2E tests`, and `Risks` sections of the plan
- `apps/web/e2e/tests/layout/topbar-alignment.spec.ts`
- `apps/web/e2e/tests/layout/task-topbar-long-title.spec.ts`
- `docs/decisions/0039-native-desktop-integration-boundary.md`

## Risks

- Do not use fixed offsets for operating-system controls. Consume the overlay rectangle.
- After applying left safe-area padding, `overflow-hidden` must not clip the collapsed-sidebar expand control.
- A fake API proves frontend behavior. It does not prove that a specific installed browser exposes the platform capability. Record a visual check in an installed PWA separately.

## Output contract

Report RED and GREEN E2E evidence, bounding-box relationships, verification results, changed files, real-PWA visual evidence or the exact reason it was not run, blockers, and risks. Update this task and the plan results.

## Results

- RED: `pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts` built successfully. The fused scenarios failed as expected because `[data-window-controls-overlay-region="sidebar"]` did not exist. The desktop fallback scenario passed.
- GREEN: The final command passed three Chromium scenarios. It verifies left and right safe areas, expanded and collapsed sidebars, `geometrychange`, draggable and non-draggable regions, task top-bar controls, theme metadata, and no horizontal overflow.
- The sidebar header moved outside the clipped navigation container. The collapsed header can extend beyond the 56px rail without hiding the expand control. Page and task top bars consume the same root geometry variables.
- The final focused Vitest suite passed four files and eight tests. Typecheck and Web lint passed.
- Headless Playwright uses a deterministic fake to verify the application contract. A visual check in an installed PWA remains a separate environment check.
- The requested scope covers desktop only. No mobile or tablet layout tests were added or changed.
