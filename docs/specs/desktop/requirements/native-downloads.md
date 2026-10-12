---
status: active
system: desktop
created: 2026-09-25
owners:
  - kandev
---

# Native Downloads Requirements

## Overview

The installed desktop app uses the shared web interface, but saving an export is
an operating-system action. The desktop system owns that boundary for all
first-party downloads started inside its window. The feature that creates each
file continues to own its contents and access rules.

## Requirements

### REQ-DESKTOP-NATIVE-DOWNLOADS-001: Save an in-app download

**Intent:** A desktop user can choose where an explicitly requested file is
saved and can tell whether the save succeeded.

#### Acceptance criteria

- **AC-DESKTOP-NATIVE-DOWNLOADS-001.1:** When a user starts a file download in
  the desktop app, the system shall present the operating system's Save dialog
  with the file's suggested name before writing the file.
- **AC-DESKTOP-NATIVE-DOWNLOADS-001.2:** When the user selects a destination,
  the system shall save the requested file there with its original bytes. This
  applies to backend-served files and files generated in the interface, including
  the diagnostic log bundle.
- **AC-DESKTOP-NATIVE-DOWNLOADS-001.3:** When the user cancels the Save dialog,
  the system shall leave the destination unchanged and shall not report a
  successful save.
- **AC-DESKTOP-NATIVE-DOWNLOADS-001.4:** When a requested desktop save fails,
  the system shall show a visible failure and allow the user to try the action
  again. It shall not claim that a file was saved before completion.
- **AC-DESKTOP-NATIVE-DOWNLOADS-001.5:** Browser and phone downloads shall retain
  their browser-managed behavior and existing entry points.

## Out of scope

- Desktop updater package downloads, which are part of the signed update flow.
- Downloads opened in the external system browser.
- Changing export contents or permissions.

## System design

[Native downloads](../system-design/native-downloads.md).
