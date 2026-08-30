---
spec: docs/specs/pwa-window-controls-overlay/spec.md
created: 2026-08-26
status: implemented
---

# Implementation plan: desktop PWA fused title bar

## Overview

First, make the existing PWA manifest select the browser Window Controls Overlay mode. Then expose the live overlay geometry from the application shell.

Reuse the existing 40px desktop sidebar, page top bar, and task top bar as the fused title-bar surface. Keep Kandev controls outside the system-control exclusion area.

Interactive descendants must leave the draggable region. Browsers without this capability, and browsers that do not install the PWA, keep the current layout.

## Frontend

### PWA capability contract

- `apps/web/public/manifest.webmanifest`: add `display_override`, put `window-controls-overlay` first, use `standalone` as the explicit fallback, and keep the existing `display` field.
- `apps/web/global.d.ts`: add narrow TypeScript declarations for `navigator.windowControlsOverlay`, visibility, the title-bar rectangle, and the `geometrychange` event. Do not add a general browser-capability abstraction.
- `apps/web/hooks/use-window-controls-overlay.ts`: add a focused hook. It reads the initial visibility and geometry, subscribes to `geometrychange`, rereads the geometry after listener registration, and removes the listener on unmount. It returns an inactive state when the API is missing or hidden.
- `apps/web/hooks/use-window-controls-overlay.test.tsx`: use a controllable fake overlay to cover a missing API, initial visibility, geometry changes, hiding, the startup reread, and unmount cleanup.
- `apps/web/lib/browser/pwa-manifest.test.ts`: lock the opt-in and `standalone` fallback so later manifest cleanup does not restore an extra browser title bar.

### Root shell and desktop application bars

- `apps/web/src/app-shell.tsx`: call the overlay hook once. Set stable `data-window-controls-overlay` state, numeric safe-area CSS variables, and a stable E2E anchor on the root `h-dvh` shell. Do not persist browser capability state.
- `apps/web/components/app-sidebar/app-sidebar-header.tsx` and `apps/web/components/app-sidebar/app-sidebar.tsx`: mark the desktop sidebar header as the left fused-title-bar segment. In expanded and collapsed states, keep the brand, workspace picker, and sidebar toggle outside the left system-control area. If the collapsed header must extend beyond the 56px rail, limit clipping to the navigation and footer regions.
- `apps/web/components/page-topbar.tsx` and `apps/web/components/task/task-top-bar.tsx`: mark the shared desktop top bars as draggable title-bar segments. Apply live right safe-area padding to page and task actions. Mark interactive descendants as non-draggable.
- `apps/web/app/globals.css`: apply overlay height, left and right safe-area padding, drag and non-drag regions, theme background, and overflow protection only when the root state says that the overlay is visible. Use the geometry variables from the shell. Keep current class behavior when the state is missing or hidden.
- `apps/web/components/theme-provider.tsx`: synchronize the resolved application theme with a dynamic `theme-color` meta element while the overlay is visible. Preserve the static media-specific entries for the normal browser fallback. Remove the dynamic override when the overlay becomes hidden.
- `apps/web/components/theme-provider.test.tsx`: cover a light system with a dark application theme, runtime changes back to light, preservation of static media entries, and removal of the dynamic override when the overlay is hidden.
- `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`: install a deterministic `navigator.windowControlsOverlay` fake before navigation. Verify expanded and collapsed sidebar geometry, page and task action geometry, live `geometrychange`, clickable and non-draggable controls, theme metadata, and the desktop fallback when the API is missing. Use bounding-box relationships instead of fixed platform pixels.

The implementation keeps the existing `AppShell -> AppSidebar + page/task topbar` ownership model. It reuses the geometry assertion pattern from `apps/web/e2e/tests/layout/topbar-alignment.spec.ts`.

The implementation does not change the native Tauri integration boundary defined by `docs/decisions/0039-native-desktop-integration-boundary.md`.

## Tests

- **Capability lifecycle:** The missing API stays inactive. Mounting reads visible geometry. Listener registration triggers an immediate reread. `geometrychange` updates state. Hiding clears active geometry. Unmount removes the listener. File: `apps/web/hooks/use-window-controls-overlay.test.tsx`.
- **Manifest contract:** Window Controls Overlay is the first override, and `standalone` remains the fallback. File: `apps/web/lib/browser/pwa-manifest.test.ts`.
- **Desktop fallback:** When the overlay is inactive, the existing 40px title-bar and top-bar geometry stays unchanged. File: `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`.
- **Theme contract:** The dynamic overlay color follows the resolved application theme. Static media-specific colors remain available outside the overlay path. File: `apps/web/components/theme-provider.test.tsx`.

## E2E tests

- **Left system controls:** With a visible overlay rectangle that starts after the left exclusion area, expanded and collapsed sidebar controls stay to its right and remain clickable.
- **Right system controls:** With a visible overlay rectangle that ends before the right exclusion area, page and task top-bar actions stay to its left.
- **Live geometry:** After `geometrychange`, safe layout boundaries move without a refresh, and `documentElement.scrollWidth <= clientWidth` remains true.
- **Ordinary desktop browser:** When the API is missing or `visible=false`, root-shell and top-bar geometry match the existing desktop layout.
- **Theme synchronization:** When system and application themes differ, the dynamic overlay color uses the resolved application background. A theme change updates it immediately, and hiding the overlay removes it.

All scenarios are in `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts` and use the desktop `chromium` project. The requested scope does not add mobile or tablet tests.

## Verification

Run the focused Red-Green-Refactor commands:

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run hooks/use-window-controls-overlay.test.tsx lib/browser/pwa-manifest.test.ts
cd apps/web && pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts
```

After focused checks pass, run the repository commands required by the task:

```bash
make fmt
make typecheck test lint
```

Stage files with explicit paths and create a Conventional Commit only after the required checks pass. Push the integration branch only after implementation, review, verification, and commit checks are complete.

## Verification results

The final focused Vitest suite passed for four files and eight tests. The PWA Playwright suite passed three Chromium scenarios. Typecheck and Web lint passed for the final implementation.

The full repository test history contains unrelated local-environment failures. These include the Docker-dependent Web test, backend environment checks, and a missing local `golangci-lint` binary. None of these failures came from the PWA overlay files.

## Implementation waves and parallel candidates

Wave 1 (sequential):

- [x] [Task 01: PWA overlay capability](task-01-pwa-overlay-capability.md)

Wave 2 (sequential, depends on Task 01):

- [x] [Task 02: fused desktop title-bar layout](task-02-fused-desktop-titlebar-layout.md)

Wave 3 (sequential, depends on Task 02):

- [x] [Task 03: synchronize fused title-bar theme color](task-03-sync-titlebar-theme-color.md)

The three tasks share the root shell and frontend capability contract. Run them in order in the primary session.

## Risks

- Browser-controlled title-bar rectangles differ by operating system and can change at runtime. Consume the reported geometry instead of guessing macOS or Windows control positions.
- The collapsed 56px sidebar can be narrower than some left system-control areas. Verify clipping and spacing with adjacent top bars.
- Headless Playwright does not launch an installed PWA window. The deterministic overlay fake proves application geometry and event handling. A visual check in an installed PWA remains useful when the environment supports it.
