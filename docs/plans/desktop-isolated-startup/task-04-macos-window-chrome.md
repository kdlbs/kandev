---
id: "04-macos-window-chrome"
title: "Integrate macOS window controls"
status: done
wave: 4
depends_on:
  - "03-conflict-recovery-ui"
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
acceptance_criteria:
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.10
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.11
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
  - ../../specs/desktop/system-design/isolated-startup.md
---

# Task 04: Integrate macOS window controls

## Summary

Remove the visually separate macOS title strip while keeping native traffic
lights in Kandev's existing top row. Carry the layout from the first startup
frame through conflict recovery and the backend-served app.

## In scope

- Configure the macOS main window with Tauri's overlay title-bar style,
  decorations enabled, and its centered native title hidden. Preserve native
  close, minimize, full-screen, menu, and window-state behavior.
- Keep the traffic-light hit area clear in the first-paint startup HTML,
  loading, failure, conflict, and temporary-test window surfaces.
- Have the web shell identify the macOS Tauri overlay without changing the
  browser PWA overlay contract. Fit the native controls into the existing
  sidebar/page header row in expanded and collapsed sidebar states. Keep the
  workspace switcher and collapse button in the expanded row, and move the
  56px rail's brand and expand button below the native controls.
- Provide a drag region only on noninteractive header space. Keep workspace
  selection, search, navigation, and the conflict fallback clickable.
- Preserve decorated windows on Windows and Linux.

## Out of scope

- Custom HTML window buttons, a new app-wide top bar, and changes to mobile
  web layout or browser/PWA chrome.

## Acceptance

1. The macOS window matches `UI-00a` in the [plan](plan.md#ascii-ui-preview):
   Kandev content reaches the top edge, with native traffic lights at the
   upper left and no separate centered-title strip.
2. The same controls are reachable while loading, on a conflict or error, in
   either sidebar state after navigation, and in every temporary test window.
   Content and actions do not sit beneath the native hit area at the minimum
   supported width or in full screen.
3. Dragging from an empty header region moves the window, while interactive
   header controls and the fallback action keep their normal pointer behavior.
4. Linux and Windows retain their native window frame; the installed PWA's
   browser-provided overlay remains unchanged.
5. At the 320px sidebar minimum, long workspace names truncate inside the
   switcher while its chevron and the collapse button remain visible. In the
   56px rail, the expand button is below the traffic lights. Expanding or
   hover-revealing the sidebar restores the workspace switcher.

## Verification

```bash
(cd apps && pnpm --filter @kandev/desktop typecheck)
(cd apps && pnpm --filter @kandev/web typecheck)
(cd apps && pnpm --filter @kandev/desktop e2e)
```

Add targeted rendered checks for macOS Tauri overlay layout, both sidebar
states, hover reveal, long workspace names, startup/error/conflict surfaces,
and the unaffected PWA layout. Check the toggle and switcher hit targets at
the 960px window minimum and 320px sidebar minimum.
Inspect an actual macOS build for native traffic-light positioning, window
dragging, full-screen behavior, and minimum-size hit targets; a Linux or
headless smoke cannot prove native macOS window chrome.

## Files likely touched

- `apps/desktop/src-tauri/tauri.conf.json`
- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/capabilities/default.json`
- `apps/desktop/index.html`
- `apps/desktop/src/styles.css`
- `apps/web/src/app-shell.tsx`
- `apps/web/components/app-sidebar/app-sidebar-header.tsx`
- `apps/web/components/page-topbar.tsx`
- `apps/web/app/globals.css`
- Focused desktop and web layout tests

## Dependencies

Task 03's startup markup and loaded styles, so the overlay inset can be
applied once to the final loading and conflict surfaces.

## Risks

Tauri notes that macOS overlay title-bar height varies by OS version. Fixed
safe space must be checked on a real Mac. The backend-served app cannot infer
Tauri's overlay geometry from the browser PWA API; its shell hint must be
specific to macOS desktop. Adding a full header drag region would intercept
workspace-picker and search interaction.

## Parallelism

`sequential`

## Inputs

- [Desktop requirements](../../specs/desktop/requirements/desktop-tauri-app.md),
  [desktop design](../../specs/desktop/system-design/desktop-tauri-app.md),
  [startup design](../../specs/desktop/system-design/isolated-startup.md),
  and existing PWA overlay behavior in `apps/web/app/globals.css`.
- [Tauri title-bar configuration](https://v2.tauri.app/reference/config/#titlebarstyle)
  and [window customization](https://v2.tauri.app/learn/window-customization/).

## Results

Passed desktop and web typechecks, focused React tests, web lint and i18n
checks, the Chromium layout test, and the mobile layout/repository-discovery
tests. The rendered test covers expanded, collapsed, and hover-revealed
sidebar states with a long workspace name at the 960px window width. The Linux
build cannot verify native macOS traffic-light placement, dragging,
full-screen behavior, or native hit targets; run those checks on macOS before
release.
