---
id: "03-sync-titlebar-theme-color"
title: "Synchronize fused title-bar theme color"
status: done
wave: 3
depends_on: ["02-fused-desktop-titlebar-layout"]
plan: "plan.md"
spec: "../../specs/pwa-window-controls-overlay/spec.md"
---

# Task 03: synchronize fused title-bar theme color

## Root cause

`index.html` had two `theme-color` entries selected only by the operating system `prefers-color-scheme` value. Kandev can set its theme independently.

When the system uses light mode and the application uses dark mode, browsers such as Vivaldi still use the light value for the window-control area. The fused title bar then shows light edges.

## Acceptance criteria

- The dynamic overlay `theme-color` matches Kandev's resolved light or dark theme while the overlay is visible.
- A theme change updates the browser window-control color without a page refresh or PWA restart.
- Static media-specific `theme-color` entries remain available for ordinary browser windows.
- Mobile and tablet layouts stay unchanged, and no user-visible copy is added.

## TDD and verification

Add a regression case in `apps/web/components/theme-provider.test.tsx` for a light system with a dark application theme. Confirm that the current implementation keeps the light `theme-color` and produces the expected RED result. Run the focused checks after the smallest implementation:

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/theme-provider.test.tsx
cd apps/web && pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts
```

Then run the repository checks required by the task:

```bash
make fmt
make typecheck test lint
```

## Possible files to change

- `apps/web/index.html`
- `apps/web/components/theme-provider.tsx`
- `apps/web/components/theme-provider.test.tsx`
- `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`
- `docs/specs/pwa-window-controls-overlay/spec.md`
- `docs/plans/pwa-window-controls-overlay/plan.md`
- `docs/plans/pwa-window-controls-overlay/task-03-sync-titlebar-theme-color.md`

## Output contract

Record RED and GREEN evidence, installed-Vivaldi dark-theme visual evidence, focused tests, repository checks, blockers, and risks. Change only the desktop fused-title-bar contract.

## Results

- RED: `pnpm --filter @kandev/web test -- --run components/theme-provider.test.tsx` failed as expected because the dark application theme still left two system-selected `theme-color` entries.
- GREEN: The final focused Vitest suite passed four files and eight tests. It covers an opposite system and application theme, dark `#181818`, runtime change to light `#ffffff`, static media fallback preservation, and dynamic override cleanup.
- The desktop `chromium` Playwright project passed three scenarios. The fused scenario simulates a light system, a dark application theme, and a visible Overlay API. It verifies the dynamic `theme-color` value is `#181818`.
- The implementation keeps static media-specific metadata for ordinary browser windows. It adds one dynamic metadata element only while `navigator.windowControlsOverlay.visible` is true and removes it when the overlay hides.
- Typecheck and Web lint passed for the final implementation. A visual check in an installed Vivaldi PWA remains an environment-specific follow-up.
- Mobile and tablet layouts remain unchanged, and no user-visible copy was added.
