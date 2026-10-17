---
status: active
system: desktop
created: 2026-10-08
owners:
  - kandev
---

# Desktop Developer Tools Requirements

## Overview

Users need to inspect a running desktop web view when a problem occurs.
Opening the inspector must preserve the page and its accumulated diagnostic state.
Desktop owns this capability because inspection targets the native web view.
It extends the [native desktop menu contract](desktop-tauri-app.md).

## Requirements

### REQ-DESKTOP-DEVELOPER-TOOLS-001: Inspection of the running desktop app

**Intent:** Let users diagnose the installed application without a special build,
a feature toggle, or a restart before inspection.

#### Acceptance criteria

- **AC-DESKTOP-DEVELOPER-TOOLS-001.1:** Normal release builds shall enable
  developer-tools availability by default. No feature flag, setting, or
  environment opt-in shall be required. Kandev shall not open the inspector automatically.
- **AC-DESKTOP-DEVELOPER-TOOLS-001.2:** The native View menu shall contain
  Developer Tools. Selecting it shall open the inspector for the current
  application's main web view, including its startup and error pages.
- **AC-DESKTOP-DEVELOPER-TOOLS-001.3:** Cmd+Option+I on macOS and Ctrl+Shift+I
  on Windows/Linux shall open that same inspector while Kandev has focus.
- **AC-DESKTOP-DEVELOPER-TOOLS-001.4:** Opening, closing, or reopening the
  inspector shall not reload the inspected page, replace its web view, restart
  the backend, or stop agent sessions. Repeated activation shall keep the
  existing inspector available rather than toggle it closed.
- **AC-DESKTOP-DEVELOPER-TOOLS-001.5:** On macOS 13.3 and later, Safari's
  Develop menu shall also expose the running Kandev web view for inspection.
- **AC-DESKTOP-DEVELOPER-TOOLS-001.6:** Developer-tools availability shall
  preserve existing permissions for application scripts and the owned-origin
  checks on native commands. Browser and phone clients shall retain their
  existing browser-provided developer tools without a Kandev menu addition.

## Out of scope

- Automatic profiling, heap collection, uploads, or diagnostic transmission.
- Fixing the CPU and memory growth reported in issue 4100.
- Inspector UI customization, persisted inspector visibility, and mobile native tooling.
- New remote-debugging listeners, backend APIs, or application-script inspector commands.

## Design and delivery

- [System design](../system-design/developer-tools.md).
- [Implementation package](../../../plans/desktop-developer-tools/plan.md).
