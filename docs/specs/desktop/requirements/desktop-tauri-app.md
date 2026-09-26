---
status: active
system: desktop
created: 2026-06-23
updated: 2026-09-25
owners:
  - tbd
---
# Tauri Desktop App Requirements

## Overview

Kandev's installed desktop app should behave like a native application without duplicating the existing React product surface. Users need standard focused-window commands, reliable native updates and notifications, and preservation of their window state while Kandev continues to use the existing local Go backend and shared settings UI.

## Requirements

### REQ-DESKTOP-DESKTOP-TAURI-APP-001: Tauri Desktop App

**Intent:** Kandev's installed desktop app should behave like a native application without duplicating the existing React product surface. Users need standard focused-window commands, reliable native updates and notifications, and preservation of their window state while Kandev continues to use the existing local Go backend and shared settings UI.

#### Acceptance criteria

- **AC-DESKTOP-DESKTOP-TAURI-APP-001.1:** application menus with platform-appropriate accelerators;
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.2:** zoom in, zoom out, and actual-size commands (`Cmd` on macOS, `Ctrl` elsewhere);
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.3:** contextual `Cmd/Ctrl+W` behavior that never closes the window, backend, or application;
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.4:** `Cmd+,` on macOS to open the existing `/settings/general` page;
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.5:** New Task, Check for Updates, Help, external-link, and standard application commands;
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.6:** persisted window size, position, and maximized state, restored onto a visible display;
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.7:** signed, prompt-before-install desktop updates through the existing System > Updates page;
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.8:** native notifications for selected turn-finished, clarification-requested, and session-failure events;
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.9:** an origin-checked native directory
  picker for explicit repository discovery and task-folder selection, without
  exposing general filesystem access to the SPA.
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.10:** On macOS, the desktop window shall
  show Kandev content up to the top edge without a separate full-width title
  strip or centered window title. Native close, minimize, and full-screen
  controls shall stay visible and operable at the upper left during startup,
  conflict recovery, and normal app use. The app shall keep a usable window
  drag region without covering interactive content, including when the
  sidebar is collapsed or the window is at its minimum size. Windows and
  Linux shall retain their native window decorations.
- **AC-DESKTOP-DESKTOP-TAURI-APP-001.11:** The expanded macOS sidebar header
  shall keep the Kandev link, workspace switcher, and collapse button in the
  same row to the right of the native controls. Long workspace names shall
  truncate without hiding the switcher chevron or collapse button. In the
  collapsed 56px rail, the brand and expand button shall remain reachable
  below the native controls; the workspace switcher shall reappear when the
  sidebar expands, including after hover reveal. Neither sidebar state shall
  place an interactive target under the traffic lights.

## System design

The migrated technical source is split into [part 1](../system-design/desktop-tauri-app.md).
