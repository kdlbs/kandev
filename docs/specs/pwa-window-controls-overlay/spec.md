---
status: draft
created: 2026-08-26
owner: kandev
---

# Desktop PWA fused title bar

## Why

An installed desktop PWA currently shows a browser title bar above Kandev's application top bar. This extra full-width row uses vertical space without adding useful Kandev controls.

This extra row also weakens the visual connection between the installed app and the operating system.

## What

- In an installed desktop PWA that supports this capability, the existing Kandev top bar becomes the draggable window title bar. The browser title no longer uses a separate row.
- The system close, minimize, and maximize buttons remain visible. The browser menu also remains visible. These controls do not overlap the Kandev brand, workspace picker, sidebar toggle, breadcrumbs, task controls, or page actions.
- Buttons, links, inputs, menus, and other interactive controls in the fused title bar remain clickable. They do not start window dragging.
- The fused title bar follows window-control geometry changes after startup. It handles window size changes and left or right system-control placement without horizontal overflow.
- The browser window-control background follows Kandev's resolved light or dark theme. A theme change updates the dynamic overlay color without a PWA restart. Ordinary browser windows keep the existing media-specific `theme-color` fallback.
- Sidebar title-bar controls remain accessible when the desktop sidebar expands or collapses.
- Ordinary browser tabs, uninstalled app windows, and browsers without Window Controls Overlay keep the current desktop layout and behavior.
- The feature adds no settings, persistence, backend contract, or user-visible copy.

## Scenarios

- **GIVEN** Kandev starts as an installed desktop PWA with Window Controls Overlay visible and system controls on the left, **WHEN** the app shows an expanded or collapsed sidebar, **THEN** the sidebar and page or task top bars occupy the title-bar row, and all Kandev controls stay outside the system-control exclusion area.
- **GIVEN** Kandev starts as an installed desktop PWA with system controls on the right, **WHEN** a page with right-aligned top-bar actions appears, **THEN** the actions end before the system-control exclusion area and remain usable.
- **GIVEN** the desktop PWA uses the fused title bar, **WHEN** the browser reports a title-bar geometry change, **THEN** safe padding updates without a refresh and the document has no horizontal overflow.
- **GIVEN** the pointer starts in empty fused-title-bar space, **WHEN** the user drags, **THEN** the operating system moves the window. **GIVEN** the pointer starts on a Kandev interactive control, **WHEN** the user clicks, **THEN** the control keeps its existing action and does not move the window.
- **GIVEN** Window Controls Overlay is missing or hidden, **WHEN** Kandev renders in an ordinary desktop browser, **THEN** the existing 40px sidebar and page top bars and all existing layout geometry remain unchanged.
- **GIVEN** the operating system uses a light appearance and Kandev uses a dark theme, **WHEN** Window Controls Overlay is visible or the user changes the app theme, **THEN** the dynamic browser window-control color uses Kandev's dark background without light edges. Static media-specific colors remain available when the overlay is hidden. The light theme follows the same rule.

## Out of scope

- Mobile and tablet layout changes or mobile Playwright coverage.
- Changes to the native Tauri desktop title bar, menus, lifecycle, or bridge.
- Custom replacement operating-system window controls.
- User settings that enable or disable the fused title bar.

## Implementation plan

See [`../../plans/pwa-window-controls-overlay/plan.md`](../../plans/pwa-window-controls-overlay/plan.md).
