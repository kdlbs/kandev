---
status: deprecated
system: ui
created: 2026-07-29
owners:
  - Kandev
---
# Embedded VS Code Windows Availability Requirements

## Overview

Kandev installs **VS Code (Embedded)** from code-server standalone release archives, while code-server does not publish a Windows standalone release for that installation path. Every person viewing the same Kandev instance should see editor availability derived from where Kandev is running, not from their own browser device.

## Requirements

### REQ-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001: Embedded VS Code Windows Availability

**Intent:** Kandev installs **VS Code (Embedded)** from code-server standalone release archives, while code-server does not publish a Windows standalone release for that installation path. Every person viewing the same Kandev instance should see editor availability derived from where Kandev is running, not from their own browser device.

#### Acceptance criteria

- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.1:** The authoritative platform is the operating system of the running Kandev backend started by the user or service operator. The current visitor's browser or WebView platform does not affect embedded-editor availability.
- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.2:** When Kandev is running on Windows, the task-detail topbar editor action excludes the editor whose kind is `internal_vscode`.
- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.3:** The topbar's primary editor button and its adjacent editor menu use the same host-compatible editor set. A saved embedded-editor default therefore cannot launch the embedded editor from a Windows-hosted Kandev instance.
- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.4:** If a saved default is unavailable on the Kandev host, the primary button resolves to the first remaining enabled editor without changing the saved default.
- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.5:** Other enabled editor integrations remain available on Windows under their existing installed and configuration rules.
- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.6:** When no compatible editor remains, the topbar editor controls remain disabled and do not launch an editor.
- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.7:** Kandev instances running on macOS, Linux, or an unknown platform retain the existing embedded-editor behavior, regardless of the visitor's platform.
- **AC-UI-EMBEDDED-VSCODE-WINDOWS-AVAILABILITY-001.8:** The phone task-detail layout remains unchanged; it already uses a separate mobile topbar without the desktop editor action.

## Migrated source detail

> Archived on 2026-07-30. This host-wide rule is superseded by
> [Embedded VS Code Executor Availability](embedded-vscode-executor-availability.md), which derives
> availability from the active session's executor runtime.

## Why

Kandev installs **VS Code (Embedded)** from code-server standalone release archives, while
code-server does not publish a Windows standalone release for that installation path. Every person
viewing the same Kandev instance should see editor availability derived from where Kandev is
running, not from their own browser device.

## What

- The authoritative platform is the operating system of the running Kandev backend started by the
  user or service operator. The current visitor's browser or WebView platform does not affect
  embedded-editor availability.
- When Kandev is running on Windows, the task-detail topbar editor action excludes the editor whose
  kind is `internal_vscode`.
- The topbar's primary editor button and its adjacent editor menu use the same host-compatible
  editor set. A saved embedded-editor default therefore cannot launch the embedded editor from a
  Windows-hosted Kandev instance.
- If a saved default is unavailable on the Kandev host, the primary button resolves to the first
  remaining enabled editor without changing the saved default.
- Other enabled editor integrations remain available on Windows under their existing installed and
  configuration rules.
- When no compatible editor remains, the topbar editor controls remain disabled and do not launch
  an editor.
- Kandev instances running on macOS, Linux, or an unknown platform retain the existing
  embedded-editor behavior, regardless of the visitor's platform.
- The phone task-detail layout remains unchanged; it already uses a separate mobile topbar without
  the desktop editor action.

## API surface

The SPA boot payload's `runtime` object includes:

```text
hostOS string
```

`hostOS` is the Go runtime operating-system identifier of the running Kandev backend, such as
`windows`, `darwin`, or `linux`. It is read-only boot metadata and is not persisted.

## Scenarios

- **GIVEN** Kandev is running on Windows and another compatible editor is available, **WHEN** any
  visitor opens the task-detail topbar editor menu, **THEN** the compatible editor is shown and
  **VS Code (Embedded)** is absent.
- **GIVEN** Kandev is running on Windows, the saved default is **VS Code (Embedded)**, and another
  compatible editor is available, **WHEN** a visitor activates the primary topbar editor button,
  **THEN** Kandev opens the compatible fallback and does not launch the embedded editor.
- **GIVEN** Kandev is running on Windows with no compatible editor other than
  **VS Code (Embedded)**, **WHEN** a visitor views task details, **THEN** the topbar editor controls
  are disabled.
- **GIVEN** Kandev is running on Linux and a visitor's browser reports Windows, **WHEN** the visitor
  opens the task-detail topbar editor menu, **THEN** **VS Code (Embedded)** remains available.
- **GIVEN** Kandev is running on Windows and a visitor's browser reports Linux or macOS, **WHEN**
  the visitor opens the task-detail topbar editor menu, **THEN** **VS Code (Embedded)** is absent.
- **GIVEN** a phone user opens task details, **WHEN** the mobile task topbar renders, **THEN** no
  desktop editor action is exposed.

## Out of scope

- Adding or packaging code-server support for Windows.
- Upgrading Kandev's pinned code-server version.
- Making availability depend on the operating system inside a Docker, SSH, Sprites, or other task
  executor independently of the Kandev backend host.
- Removing **VS Code (Embedded)** from editor settings, file-level **Open with** menus, layout
  presets, or the Dockview add-panel menu.
- Persisting platform-specific editor preferences or changing the editor API.

## Implementation plan

- [Embedded VS Code Windows availability plan](../../../plans/embedded-vscode-windows-availability/plan.md)
