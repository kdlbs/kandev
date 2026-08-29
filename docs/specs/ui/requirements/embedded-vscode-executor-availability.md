---
status: active
system: ui
created: 2026-07-30
owners:
  - Kandev
---
# Embedded VS Code Executor Availability Requirements

## Overview

Kandev starts **VS Code (Embedded)** as a code-server process inside the active task session's execution environment. The Kandev backend host and that execution environment can use different operating systems: a Windows desktop can run a Linux Docker task, while a Linux Kandev server can control another supported remote runtime.

## Requirements

### REQ-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001: Embedded VS Code Executor Availability

**Intent:** Kandev starts **VS Code (Embedded)** as a code-server process inside the active task session's execution environment. The Kandev backend host and that execution environment can use different operating systems: a Windows desktop can run a Linux Docker task, while a Linux Kandev server can control another supported remote runtime.

#### Acceptance criteria

- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.1:** The active task session's executor runtime is authoritative for **VS Code (Embedded)** availability. The Kandev backend host matters only when that executor runs directly on the host.
- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.2:** The backend reports an explicit `embedded_vscode` capability for each task-session status response. The frontend does not reconstruct support from executor names or browser platform data.
- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.3:** Kandev computes support as follows:
- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.4:** When Kandev later supports an SSH or other executor on additional operating systems, the backend capability resolver must be updated before advertising embedded-editor support there.
- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.5:** The task-detail topbar's primary editor action and adjacent menu use the active session's capability. Switching sessions updates the compatible editor set.
- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.6:** The existing enabled and installed checks still apply to every editor. Only `internal_vscode` is affected by the new capability.
- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.7:** If a saved default is unavailable for the active session, the primary action uses the first remaining compatible editor without changing the saved preference.
- **AC-UI-EMBEDDED-VSCODE-EXECUTOR-AVAILABILITY-001.8:** If no compatible editor remains, both topbar editor controls are disabled and no editor is launched.

## Migrated source detail

## Why

Kandev starts **VS Code (Embedded)** as a code-server process inside the active task session's
execution environment. The Kandev backend host and that execution environment can use different
operating systems: a Windows desktop can run a Linux Docker task, while a Linux Kandev server can
control another supported remote runtime.

The previous host-wide Windows rule therefore hid a working embedded editor from every task on a
Windows Kandev host, including Linux-backed Docker tasks. Availability must follow the environment
that will actually run code-server.

## What

- The active task session's executor runtime is authoritative for **VS Code (Embedded)**
  availability. The Kandev backend host matters only when that executor runs directly on the host.
- The backend reports an explicit `embedded_vscode` capability for each task-session status
  response. The frontend does not reconstruct support from executor names or browser platform
  data.
- Kandev computes support as follows:

  | Executor type | Execution platform used for the decision | Embedded VS Code |
  | --- | --- | --- |
  | Local / Local PC (`local`) | Kandev backend host | Available on Linux and macOS; unavailable on Windows |
  | Worktree (`worktree`) | Kandev backend host | Available on Linux and macOS; unavailable on Windows |
  | Local Docker (`local_docker`) | Linux container | Available, including when Kandev runs on Windows |
  | Remote Docker (`remote_docker`) | Linux container | Available |
  | Sprites (`sprites`) | Linux sandbox | Available |
  | SSH (`ssh`) | Supported remote platform | Available while SSH execution remains limited to Linux and macOS |
  | Unknown or unresolved executor | Unknown | Unavailable |

- When Kandev later supports an SSH or other executor on additional operating systems, the backend
  capability resolver must be updated before advertising embedded-editor support there.
- The task-detail topbar's primary editor action and adjacent menu use the active session's
  capability. Switching sessions updates the compatible editor set.
- The existing enabled and installed checks still apply to every editor. Only
  `internal_vscode` is affected by the new capability.
- If a saved default is unavailable for the active session, the primary action uses the first
  remaining compatible editor without changing the saved preference.
- If no compatible editor remains, both topbar editor controls are disabled and no editor is
  launched.
- A direct request to open `internal_vscode` for an unsupported or unresolved session is rejected
  as an unavailable editor. UI filtering is not the enforcement boundary.
- The visitor's browser or WebView operating system never controls this capability.
- The phone task-detail layout remains unchanged. It continues to use its compact mobile topbar
  without the desktop editor action.

## API surface

The `task.session.status` WebSocket response includes:

```json
{
  "capabilities": {
    "embedded_vscode": true
  }
}
```

`capabilities` is backend-owned session execution metadata. `embedded_vscode` is always a boolean
for an authorized, existing session. Missing capability data from an older backend, a missing
executor, or an unrecognized runtime is treated as `false` by the frontend.

`POST /api/v1/task-sessions/:id/open-editor` returns the existing editor-unavailable conflict
response when `internal_vscode` is selected for a session whose capability is false.

The boot runtime's `hostOS` field is removed because it is no longer an editor-availability
contract and has no other consumer.

## Failure modes

- If task-session status has not loaded yet, the embedded editor is withheld while other enabled
  editors remain usable.
- If the executor lookup or runtime mapping fails, the backend reports
  `embedded_vscode: false`.
- A stale or handcrafted client cannot bypass the rule: the open-editor service resolves the
  session executor and rejects unsupported embedded-editor requests.
- A supported runtime can still fail during code-server download or startup. Existing panel errors
  and task-environment logs remain the diagnostic surface for network, archive, process, and proxy
  failures.
- Changing active sessions cannot reuse the previous session's capability.

## Scenarios

- **GIVEN** Kandev runs on Windows and the active session uses Local or Worktree, **WHEN** a user
  opens the task-detail editor menu, **THEN** **VS Code (Embedded)** is absent and other compatible
  editors remain available.
- **GIVEN** Kandev runs on Windows and the active session uses Local Docker, **WHEN** a user opens
  the task-detail editor menu, **THEN** **VS Code (Embedded)** is available.
- **GIVEN** a task has one unsupported local session and one supported Docker session, **WHEN** the
  user switches between them, **THEN** the editor menu follows the newly active session.
- **GIVEN** Kandev runs on Linux or macOS and the active session uses Local or Worktree, **WHEN** a
  user opens the editor menu, **THEN** **VS Code (Embedded)** is available.
- **GIVEN** the saved default is **VS Code (Embedded)** but the active session does not support it,
  **WHEN** the primary editor action is activated, **THEN** Kandev opens the first compatible
  fallback without changing the saved default.
- **GIVEN** the active session executor is missing or unrecognized, **WHEN** task details render,
  **THEN** the embedded editor is absent and cannot be opened through the API.
- **GIVEN** an unsupported session, **WHEN** a client directly requests `internal_vscode`,
  **THEN** the backend returns the editor-unavailable conflict response.
- **GIVEN** a visitor's browser reports Windows but the active task runtime is supported, **WHEN**
  the editor menu opens, **THEN** **VS Code (Embedded)** remains available.
- **GIVEN** a phone user opens task details for any executor, **WHEN** the mobile topbar renders,
  **THEN** no desktop editor action is exposed and the header does not overflow horizontally.

## Out of scope

- Replacing code-server with Microsoft VS Code Server.
- Adding or packaging a native Windows code-server runtime.
- Changing Kandev's pinned code-server version or installation source.
- Adding Windows as a supported SSH execution platform.
- Proactively hiding the embedded editor from editor settings, file-level **Open with** menus,
  layout presets, or the Dockview add-panel menu. The backend availability guard still applies to
  open-editor API requests.
- Changing external editor discovery, custom editor commands, or hosted-editor behavior.
- Adding an editor action to the phone task topbar.

## Implementation plan

- [Embedded VS Code executor availability plan](../../../plans/embedded-vscode-executor-availability/plan.md)
