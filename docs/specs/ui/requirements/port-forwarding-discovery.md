---
status: active
system: ui
created: 2026-08-07
owners:
  - kandev
---
# Task-scoped port-forwarding discovery Requirements

## Overview

People who host Kandev on a home machine and access it remotely need the services running in a task workspace to be reachable without knowing where the existing tunnel control is hidden. The current control is easy to miss and is gated by executor type, so local-executor tasks do not offer the capability even when their Kandev host is being accessed remotely.

## Requirements

### REQ-UI-PORT-FORWARDING-DISCOVERY-001: Task-scoped port-forwarding discovery

**Intent:** People who host Kandev on a home machine and access it remotely need the services running in a task workspace to be reachable without knowing where the existing tunnel control is hidden. The current control is easy to miss and is gated by executor type, so local-executor tasks do not offer the capability even when their Kandev host is being accessed remotely.

#### Acceptance criteria

- **AC-UI-PORT-FORWARDING-DISCOVERY-001.1:** The desktop task `+` launcher includes a checkable **Port forwarding** entry for the active task. It uses the existing port-forwarding capability; it does not create a Dockview panel.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.2:** The phone task-switcher drawer and the tablet task-switcher sheet expose the same active-task action with the same checked state, so the feature does not depend on a desktop-only menu.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.3:** The entry is available for both local and remote executors when the active task has a session and its agentctl is ready. It is visible but disabled, with an explanatory accessible label, while the session or agentctl is unavailable.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.4:** Selecting an unchecked entry persists the task preference, reveals the existing port-forwarding control in the task top bar, and opens the existing Port Forwarding dialog. The dialog continues to list detected/manual ports and start, stop, and link active tunnels using its current behavior.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.5:** Selecting a checked entry persists the preference as disabled and hides the top-bar control. It does not stop, delete, or otherwise change active tunnels. Re-enabling the preference exposes the same active tunnels again when the dialog refreshes.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.6:** The top-bar control is rendered on desktop and mobile whenever the persisted preference is enabled and the active session/agentctl is ready. Clicking it opens the existing dialog; stopping the last tunnel does not hide the control.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.7:** The preference is per task, not per browser, executor, or session. New tasks default to disabled.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.8:** A failed preference mutation leaves the previous checked state in place, does not open the dialog for an enable attempt, and surfaces an actionable error through the existing task UI feedback.

## Migrated source detail

## Why

People who host Kandev on a home machine and access it remotely need the services running in a
task workspace to be reachable without knowing where the existing tunnel control is hidden. The
current control is easy to miss and is gated by executor type, so local-executor tasks do not offer
the capability even when their Kandev host is being accessed remotely.

## What

- The desktop task `+` launcher includes a checkable **Port forwarding** entry for the active task.
  It uses the existing port-forwarding capability; it does not create a Dockview panel.
- The phone task-switcher drawer and the tablet task-switcher sheet expose the same active-task
  action with the same checked state, so the feature does not depend on a desktop-only menu.
- The entry is available for both local and remote executors when the active task has a session and
  its agentctl is ready. It is visible but disabled, with an explanatory accessible label, while
  the session or agentctl is unavailable.
- Selecting an unchecked entry persists the task preference, reveals the existing port-forwarding
  control in the task top bar, and opens the existing Port Forwarding dialog. The dialog continues
  to list detected/manual ports and start, stop, and link active tunnels using its current behavior.
- Selecting a checked entry persists the preference as disabled and hides the top-bar control. It
  does not stop, delete, or otherwise change active tunnels. Re-enabling the preference exposes the
  same active tunnels again when the dialog refreshes.
- The top-bar control is rendered on desktop and mobile whenever the persisted preference is enabled
  and the active session/agentctl is ready. Clicking it opens the existing dialog; stopping the
  last tunnel does not hide the control.
- The preference is per task, not per browser, executor, or session. New tasks default to disabled.
- A failed preference mutation leaves the previous checked state in place, does not open the dialog
  for an enable attempt, and surfaces an actionable error through the existing task UI feedback.

## Data model

The existing task metadata JSON object stores one product-owned key:

| Entity | Field | Type | Meaning |
| --- | --- | --- | --- |
| `tasks` | `metadata.port_forwarding_enabled` | optional boolean | Whether this task should expose its port-forwarding control. Missing and `false` both mean disabled. |

The key is persisted in the existing task metadata column and is included in the existing `TaskDTO`
and `task.updated` payload. Updating it MUST merge the key into the current metadata object and
preserve unrelated metadata keys. No schema migration or new task relationship is introduced.

The detected-port and tunnel records remain the existing session-scoped runtime state. This feature
does not make tunnels durable and does not change their current cleanup behavior when the backend or
session ends.

## API surface

Add the task-scoped preference mutation:

`PATCH /api/v1/tasks/:id/port-forwarding`

Request body:

```json
{ "enabled": true }
```

The body MUST contain a JSON boolean. A successful request returns the updated `TaskDTO` with the
merged metadata and publishes the normal `task.updated` event containing the same metadata. A
malformed body returns `400`; an inaccessible or missing task returns `404`; a persistence failure
returns `500`.

The existing port-forwarding WebSocket actions remain the runtime contract:

- `port.list`
- `port.tunnel.list`
- `port.tunnel.start`
- `port.tunnel.stop`

The existing session-scoped port-proxy URLs and their authorization rules are unchanged. No new
WebSocket action is required for the preference.

## State machine

| State | Trigger | Result |
| --- | --- | --- |
| Disabled | Task has no preference or `port_forwarding_enabled=false` | No task top-bar control is rendered; the launcher entry is unchecked. |
| Persisting enabled | User selects the unchecked launcher entry | The mutation is sent while the previous UI state is retained until success. |
| Enabled | Preference mutation succeeds with `enabled=true` | The launcher entry is checked, the eligible top-bar control appears, and the existing dialog opens. |
| Persisting disabled | User selects the checked launcher entry | The mutation is sent; existing tunnels are left untouched. |
| Unavailable | Session is missing or agentctl is not ready | The preference remains unchanged; the launcher entry is disabled and the top-bar control is hidden. |
| Mutation failed | The preference endpoint rejects or cannot persist the request | The prior enabled/disabled state remains authoritative, the dialog is not opened for a failed enable, and an error is shown. |

Dialog open/closed and tunnel inactive/active are independent runtime states. Closing the dialog does
not change the preference. Disabling the preference does not transition an active tunnel to stopped.

## Permissions

The new preference route uses the existing task visibility authorization: a caller must be allowed
to mutate the task's workspace. It introduces no new role or agent permission. Port discovery and
tunnel operations continue to use the existing session/task authorization of the port APIs and
proxy routes.

## Failure modes

- If the active session is not ready, the UI fails closed for runtime access: the top-bar control is
  hidden and the launcher action is disabled rather than attempting a request against an unavailable
  agentctl.
- If the preference write fails, the UI rolls back any optimistic presentation, keeps active tunnels
  running, and shows the existing error feedback. It does not open a dialog that cannot be backed by
  a ready session.
- If a `task.updated` event arrives without metadata, clients preserve their cached metadata. An
  event that explicitly contains metadata is authoritative for the task preference.
- If port detection or tunnel start/stop fails, the existing Port Forwarding dialog reports the
  runtime error without changing the saved visibility preference.
- If another client changes the preference, the task's `task.updated` event reconciles the checked
  state; the server value wins over stale local optimistic state.

## Persistence guarantees

- The enabled/disabled preference survives page reloads, browser restarts, Kandev backend restarts,
  task session replacement, and revisiting the same task. It is shared with other clients that can
  see the task because it is task metadata.
- Active tunnels remain governed by their existing in-memory/session lifecycle. Disabling the UI
  preference leaves them running, while a backend restart may still clear them as it does today.
- The preference is not copied to other tasks, inherited from an executor, or stored as a global user
  setting.

## Scenarios

- **GIVEN** a local-executor task with a ready agentctl session and no preference, **WHEN** the user
  opens the desktop task `+` menu, **THEN** **Port forwarding** is visible and unchecked while the
  top-bar port-forwarding control is hidden.
- **GIVEN** the same local task, **WHEN** the user selects the unchecked entry, **THEN** the task
  preference is enabled, the top-bar control appears, and the existing Port Forwarding dialog opens.
- **GIVEN** a remote-executor task with a ready agentctl session, **WHEN** the user enables the
  entry, **THEN** it follows the same flow as the local task and does not use a separate remote-only
  UI path.
- **GIVEN** an enabled task with an active tunnel, **WHEN** the user disables Port forwarding,
  **THEN** the top-bar control disappears but the tunnel remains active and is still listed after the
  preference is enabled again.
- **GIVEN** an enabled task, **WHEN** the user reloads the task or opens it in another client,
  **THEN** the launcher entry remains checked and the ready top-bar control is visible.
- **GIVEN** a phone viewport, **WHEN** the user opens the task-switcher drawer and selects Port
  forwarding, **THEN** the drawer closes, the same preference mutation occurs, the mobile top-bar
  control appears, and the existing dialog opens without horizontal overflow.
- **GIVEN** a tablet viewport, **WHEN** the user opens the task-switcher sheet, **THEN** the same
  active-task action is reachable with a touch-sized row and uses the same preference state.
- **GIVEN** an active task whose agentctl is not ready, **WHEN** the user opens the task launcher,
  **THEN** Port forwarding is disabled with an accessible readiness explanation and no top-bar
  control is rendered.
- **GIVEN** the preference endpoint fails, **WHEN** the user attempts to enable Port forwarding,
  **THEN** the entry remains unchecked, the top-bar control remains hidden, and an error is shown.
- **GIVEN** a task with unrelated metadata, **WHEN** the user toggles Port forwarding, **THEN** all
  unrelated metadata remains unchanged in the response and subsequent task reads.

## Out of scope

- Adding a new tunnel transport, proxy mode, port discovery mechanism, or host-wide tunnel manager.
- Persisting running tunnels or restoring them automatically after a backend restart.
- Stopping tunnels when the visibility preference is disabled.
- Adding a separate Dockview panel or a global settings toggle.
- Automatically exposing every detected port without the user's existing per-port tunnel action.
