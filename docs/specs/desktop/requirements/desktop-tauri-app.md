---
status: active
system: desktop
created: 2026-06-23
updated: 2026-08-27
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

## System design

The migrated technical source is split into [part 1](../system-design/desktop-tauri-app.md).
