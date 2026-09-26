---
status: active
system: platform
created: 2026-08-04
owners:
  - kandev
---
# Prevent Host Sleep During Active Tasks Requirements

## Overview

Long-running local tasks can be interrupted when the computer hosting Kandev enters idle sleep. Operators need an explicit way to keep that host awake during active agent work without imposing a power-management policy on servers, containers, Kubernetes installations, or always-on machines that do not need it.

## Requirements

### REQ-PLATFORM-TASK-SLEEP-INHIBITION-001: Prevent Host Sleep During Active Tasks

**Intent:** Long-running local tasks can be interrupted when the computer hosting Kandev enters idle sleep. Operators need an explicit way to keep that host awake during active agent work without imposing a power-management policy on servers, containers, Kubernetes installations, or always-on machines that do not need it.

#### Acceptance criteria

- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.1:** Settings > General > Task Actions exposes an install-wide **Prevent host sleep while tasks run** setting to administrators.
- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.2:** The setting is disabled by default. Missing or older persisted settings resolve to disabled.
- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.3:** When enabled, Kandev requests that the operating system prevent idle system sleep while at least one task session is `STARTING` or `RUNNING`.
- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.4:** Kandev releases the request after the last session leaves `STARTING` or `RUNNING`, when the setting is disabled, and during graceful backend shutdown.
- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.5:** `WAITING_FOR_INPUT`, `IDLE`, `CREATED`, and terminal session states do not keep the host awake.
- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.6:** The request keeps the host system available for task execution but does not keep the display awake and does not override an explicit user sleep action, lid close, shutdown, low-power emergency, or platform policy.
- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.7:** The setting affects only the machine running the Kandev backend. It does not inhibit a Kubernetes node, container host, SSH executor, Docker executor, Sprites runtime, or another remote machine from inside an isolated Kandev deployment.
- **AC-PLATFORM-TASK-SLEEP-INHIBITION-001.8:** The Task Actions card explains the host boundary, the power/battery tradeoff, the disabled default, and that containerized/server deployments should normally leave the setting off.

## Migrated source detail

## Why

Long-running local tasks can be interrupted when the computer hosting Kandev enters idle sleep. Operators need an explicit way to keep that host awake during active agent work without imposing a power-management policy on servers, containers, Kubernetes installations, or always-on machines that do not need it.

## What

- Settings > General > Task Actions exposes an install-wide **Prevent host sleep while tasks run** setting to administrators.
- The setting is disabled by default. Missing or older persisted settings resolve to disabled.
- When enabled, Kandev requests that the operating system prevent idle system sleep while at least one task session is `STARTING` or `RUNNING`.
- Kandev releases the request after the last session leaves `STARTING` or `RUNNING`, when the setting is disabled, and during graceful backend shutdown.
- `WAITING_FOR_INPUT`, `IDLE`, `CREATED`, and terminal session states do not keep the host awake.
- The request keeps the host system available for task execution but does not keep the display awake and does not override an explicit user sleep action, lid close, shutdown, low-power emergency, or platform policy.
- The setting affects only the machine running the Kandev backend. It does not inhibit a Kubernetes node, container host, SSH executor, Docker executor, Sprites runtime, or another remote machine from inside an isolated Kandev deployment.
- The Task Actions card explains the host boundary, the power/battery tradeoff, the disabled default, and that containerized/server deployments should normally leave the setting off.
- The same card reports whether the current backend platform can provide the request and whether a request is currently active. Unsupported or failed inhibition never blocks task execution.

## Data model

The existing install-wide `settings` table stores key `task_sleep_inhibition` with this JSON value:

```json
{"enabled": false}
```

`enabled` is a non-null boolean. An absent key resolves to `false`. Runtime capability, activity, and error status are process-local observations and are not persisted.

## API surface

`GET /api/v1/system/sleep-inhibition` returns:

```json
{
  "settings": { "enabled": false },
  "status": {
    "platform": "linux",
    "supported": false,
    "active": false,
    "issue": "system_service_unavailable"
  }
}
```

- `platform` is `darwin`, `windows`, `linux`, or `other`.
- `issue` is omitted when no issue is present. Stable values are `unsupported_platform`, `system_service_unavailable`, and `request_failed`.
- `active` is true only while Kandev currently holds the native sleep-prevention request.

`PATCH /api/v1/system/sleep-inhibition` accepts `{ "enabled": boolean }` and returns the same response shape after persisting and reconciling the setting. Reads are available to authenticated users; mutation requires the admin role. Enabling an unavailable implementation is allowed so the saved preference survives deployment moves, but the response remains inactive and reports an issue.

## State machine

| Saved setting | Working sessions (`STARTING` or `RUNNING`) | Native request | Observable status |
| --- | ---: | --- | --- |
| Disabled | Any | Released | `active=false` |
| Enabled | 0 | Released | `active=false` |
| Enabled | 1 or more | Acquired once and shared | `active=true` |
| Enabled | 1 or more, platform unavailable/fails | Not acquired | `active=false`, `issue` set |

Session-state events trigger reconciliation against the authoritative session repository. A periodic reconciliation also repairs a missed event and retries a failed native request while work remains active. Multiple working sessions never create multiple native requests.

## Permissions

- Any authenticated user may read the configured value and current status.
- Only an administrator may change the install-wide setting.
- With authentication disabled, the existing synthetic administrator retains current single-user behavior.

## Failure modes

- On macOS, Kandev requests idle-system-sleep prevention without requesting display wakefulness. If the native facility cannot start or exits unexpectedly, tasks continue and status reports `request_failed`.
- On Windows, Kandev holds `ES_CONTINUOUS | ES_SYSTEM_REQUIRED` on one locked OS thread and clears it with `ES_CONTINUOUS` on release. A failed call leaves tasks running and reports `request_failed`.
- On Linux, Kandev obtains a blocking `sleep` inhibitor from `systemd-logind` over the system bus and holds its returned file descriptor. A missing system bus, missing logind service, container isolation, or denied policy reports `system_service_unavailable` or `request_failed` without affecting tasks.
- Other operating systems report `unsupported_platform` and never attempt inhibition.
- A settings-store or session-query error leaves the last successfully reconciled native request unchanged, records a warning, and retries during the next reconciliation. Shutdown always attempts release.

## Persistence guarantees

The enabled value survives backend restarts in the install-wide settings store. Native requests do not survive process exit. After startup reconciliation, Kandev reacquires a request only when the saved setting is enabled and an authoritative session is still `STARTING` or `RUNNING`; otherwise it remains released.

## Scenarios

- **GIVEN** a new or upgraded installation, **WHEN** an administrator opens Task Actions, **THEN** sleep prevention is off and no native request is held.
- **GIVEN** sleep prevention is off, **WHEN** a task session enters `STARTING` and then `RUNNING`, **THEN** no native request is held.
- **GIVEN** sleep prevention is on with no working sessions, **WHEN** the first session enters `STARTING`, **THEN** Kandev acquires one native request.
- **GIVEN** one native request and two working sessions, **WHEN** one session settles, **THEN** the request remains active for the other session.
- **GIVEN** sleep prevention is active, **WHEN** the last working session enters `WAITING_FOR_INPUT`, `IDLE`, or a terminal state, **THEN** Kandev releases the native request.
- **GIVEN** a working session and active native request, **WHEN** an administrator disables the setting, **THEN** Kandev releases the request immediately after saving.
- **GIVEN** the setting is enabled in Docker or Kubernetes without access to host power management, **WHEN** a task runs, **THEN** the task continues normally while the settings status reports that inhibition is unavailable.
- **GIVEN** an enabled setting and a working session across backend recovery, **WHEN** startup reconciliation completes, **THEN** Kandev reacquires the request only if the session still has a working state.
- **GIVEN** a phone-sized settings viewport, **WHEN** an administrator edits and saves the setting, **THEN** the same capability is available without horizontal overflow or the floating save control covering the card.

## Out of scope

- Waking a machine that is already asleep or scheduling wake timers.
- Keeping the display, screen saver, or keyboard backlight awake.
- Overriding explicit suspend, lid-close, shutdown, battery, thermal, or administrator policies.
- Preventing sleep on executor hosts or Kubernetes nodes from an isolated backend.
- A runtime feature flag or environment-variable override for this preference.

## Implementation plan

[Task sleep inhibition plan](../../../plans/task-sleep-inhibition/plan.md)
