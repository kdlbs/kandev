# ADR-2026-09-25-macos-overlay-titlebar: Use native controls over Kandev content

**Status:** accepted
**Date:** 2026-09-25
**Area:** frontend

## Context

The macOS desktop app currently shows a full-width native title strip above
Kandev's own sidebar and page header. It duplicates the app's visual hierarchy
and consumes vertical space. The window still needs familiar close, minimize,
full-screen, and drag behavior on startup, error, and normal app pages.

## Decision

Use Tauri's macOS overlay title-bar style with native decorations enabled and
the native window title hidden. Keep the three macOS traffic lights at the
upper-left edge, visually within Kandev's existing header row. Reserve their
hit area in the startup page and the backend-served app, including collapsed
sidebar layout. Provide dragging through empty header regions. Keep Windows
and Linux native decorations and preserve the browser PWA overlay separately.

## Consequences

- No duplicate full-width title strip or custom HTML window buttons appear on
  macOS. The native controls retain their existing close and full-screen
  semantics.
- Both first-paint startup HTML and the ready app must clear the native hit
  area. The layout needs validation on a real Mac because title-bar geometry
  varies across macOS versions.
- The expanded sidebar keeps its workspace switcher and collapse button beside
  the controls. Its minimum width is 320px, so long workspace names truncate.
  In the 56px collapsed rail, the brand and expand button sit below the
  traffic lights; the adjacent page header clears the remaining hit area.
- The desktop overlay needs a shell-specific layout hint. The existing
  `navigator.windowControlsOverlay` geometry belongs to installed browser
  PWAs and cannot be assumed in Tauri.

## Alternatives Considered

- A fully undecorated window would remove the native controls and require
  replacement buttons plus more window-management privileges in web content.
- Keeping the visible native title strip would preserve today's behavior but
  leave the duplicated top row and centered title.
- A transparent title bar without content underneath would recolor the strip
  but still reserve its height above the Kandev header.
