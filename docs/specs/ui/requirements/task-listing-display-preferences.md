---
status: active
system: ui
created: 2026-07-25
owners:
  - Kandev
---
# Task Listing Display Preferences Requirements

## Overview

People browse the same tasks in different ways depending on the device and the amount of detail they need. A phone user may consistently prefer the compact list, while a desktop user may prefer Kanban or Pipeline, and list users need an optional richer row without losing the current compact default.

## Requirements

### REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001: Task Listing Display Preferences

**Intent:** People browse the same tasks in different ways depending on the device and the amount of detail they need. A phone user may consistently prefer the compact list, while a desktop user may prefer Kanban or Pipeline, and list users need an optional richer row without losing the current compact default.

#### Acceptance criteria

- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.1:** Selecting **All Workflows** remains selected in Kanban and List even when the workspace has exactly one visible workflow.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.2:** The selected workflow filter continues to use the existing portable user setting and survives navigation and reloads.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.3:** The most recently selected task-listing view is one of `kanban`, `pipeline`, or `list` and is stored on the current device.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.4:** Opening the Home task surface restores the device's last selected view.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.5:** A missing, invalid, or unavailable device preference falls back to Kanban.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.6:** Pipeline remains unavailable on phone-sized layouts. A phone temporarily renders Kanban when Pipeline is the saved device preference without overwriting that saved preference.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.7:** List view adds a **Show task details** display option. It is disabled by default and appears in both desktop and mobile display-options surfaces while List is active.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.8:** Enabling **Show task details** enriches each list row with the same useful task context surfaced by Kanban cards when that data exists: repository slug chips, a truncated description, pull-request status, session count, parent-task context, and review-attention state.

## Migrated source detail

## Why

People browse the same tasks in different ways depending on the device and the
amount of detail they need. A phone user may consistently prefer the compact
list, while a desktop user may prefer Kanban or Pipeline, and list users need an
optional richer row without losing the current compact default.

## What

- Selecting **All Workflows** remains selected in Kanban and List even when the
  workspace has exactly one visible workflow.
- The selected workflow filter continues to use the existing portable user
  setting and survives navigation and reloads.
- The most recently selected task-listing view is one of `kanban`, `pipeline`,
  or `list` and is stored on the current device.
- Opening the Home task surface restores the device's last selected view.
- A missing, invalid, or unavailable device preference falls back to Kanban.
- Pipeline remains unavailable on phone-sized layouts. A phone temporarily
  renders Kanban when Pipeline is the saved device preference without
  overwriting that saved preference.
- List view adds a **Show task details** display option. It is disabled by
  default and appears in both desktop and mobile display-options surfaces while
  List is active.
- Enabling **Show task details** enriches each list row with the same useful
  task context surfaced by Kanban cards when that data exists:
  repository slug chips, a truncated description, pull-request status, session
  count, parent-task context, and review-attention state.
- Compact rows retain their current title, task-state icon, updated time, and
  actions when **Show task details** is disabled.
- The richer-row option is a portable user preference and applies on the user's
  other signed-in Kandev devices.
- Rich rows preserve the existing primary row action: tapping or clicking the
  row opens the task, while archive/delete and interactive metadata controls do
  not trigger row navigation.
- **Settings → General → Appearance → Startup Page** offers **Task overview**
  (the default) and **Last visited task**. The latter applies only when Kandev
  starts or the user opens bare home; it resumes the newest task recorded for
  the active workspace on that device.
- Home navigation and a task's Back action always open the task overview, even
  when **Last visited task** is selected. An explicit task, session, workflow,
  or overview URL also keeps its explicit destination.

## Data model

### Device-local view preference

`localStorage["kandev.taskListing.view.v1"]` contains a JSON string with one of:

- `"kanban"`
- `"pipeline"`
- `"list"`

The browser value is the durable source of truth for this explicitly
device-local preference, consistent with
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).
Existing users with no local value may use their current
`kanban_view_mode` once as a compatibility fallback before the local preference
becomes authoritative.

### Portable richer-row preference

The existing backend-owned user settings object adds:

| Field | Type | Default | Meaning |
|---|---|---:|---|
| `tasks_list_show_details` | boolean | `false` | Whether List renders enriched task rows. |

The frontend settings store exposes the same value as
`tasksListShowDetails`.

### Startup preference and local task target

The existing backend-owned user settings object also adds:

| Field | Type | Default | Meaning |
|---|---|---:|---|
| `startup_page` | string | `"task_overview"` | `"task_overview"` starts on the saved task listing; `"last_task"` resumes a local recent task when available. |

The portable setting is exposed as `startupPage`. Its target is deliberately
not portable: `localStorage["kandev.recentTasks.v1"]` remains the source of
the newest task on the current device. Resolution only considers entries for
the active workspace.

## API surface

The existing user-settings read response includes:

```json
{
  "tasks_list_show_details": false,
  "startup_page": "task_overview"
}
```

The existing user-settings update request accepts an optional
`tasks_list_show_details` boolean and optional `startup_page` string.
`startup_page` accepts `"task_overview"` or `"last_task"`; omitting either
field leaves its saved value unchanged.

No new endpoint is introduced.

## Failure modes

- If browser storage is blocked or full, changing views still navigates for the
  current interaction, but a future app open falls back to Kanban.
- If an old or malformed device value is found, Kandev ignores it and uses
  Kanban.
- If saving **Show task details** fails, the optimistic display remains for the
  current page; the backend value wins after the next authoritative reload,
  following existing user-settings behavior.
- An absent, malformed, or unknown `startup_page` value defaults to **Task
  overview**.
- When **Last visited task** has no recent task in the active workspace, or
  browser storage is unavailable, bare home falls back to the task overview.
- Missing repository or pull-request data omits only that metadata; it does not
  hide the task row or block navigation.

## Persistence guarantees

- The last selected task-listing view survives browser restarts on the same
  device and does not sync to other devices or users.
- **Show task details** survives Kandev and browser restarts and syncs through
  the authenticated user's backend settings.
- The startup-page choice survives Kandev and browser restarts and syncs through
  backend user settings; the exact recent task remains device-local.
- A saved recent task from another workspace never becomes the startup target.
- A phone's effective Kanban fallback for a saved Pipeline preference is never
  persisted over the saved device preference.

## Scenarios

- **GIVEN** a workspace with one visible workflow, **WHEN** the user selects
  **All Workflows** on Kanban, **THEN** the selector remains on **All
  Workflows** and the choice survives a reload.
- **GIVEN** a workspace with multiple visible workflows, **WHEN** the user
  selects **All Workflows** in either Kanban or List, **THEN** tasks from every
  workflow remain visible after navigation and reload.
- **GIVEN** List was the last view selected on a phone, **WHEN** the user opens
  Home again on that device, **THEN** List is selected and rendered.
- **GIVEN** Pipeline was the last view selected on a desktop, **WHEN** the user
  opens Home again on that device, **THEN** Pipeline is selected and rendered.
- **GIVEN** Pipeline is saved and the same browser is currently phone-sized,
  **WHEN** Home opens, **THEN** Kanban renders and the saved Pipeline value is
  unchanged.
- **GIVEN** no valid device view preference, **WHEN** Home opens, **THEN**
  Kanban renders.
- **GIVEN** **Show task details** is disabled, **WHEN** List renders tasks,
  **THEN** rows remain in the current compact presentation.
- **GIVEN** **Show task details** is enabled, **WHEN** a task has repositories,
  a description, pull requests, or review/session metadata, **THEN** the
  available metadata is visible in its row without horizontal page overflow.
- **GIVEN** a rich row on a phone, **WHEN** the user taps its body, **THEN** the
  task opens and all secondary row actions remain independently touchable.
- **GIVEN** the user enables **Show task details**, **WHEN** the page or app is
  reloaded, **THEN** the option remains enabled from backend user settings.
- **GIVEN** **Last visited task** is selected and the active workspace has a
  recent local task, **WHEN** Kandev starts or bare home opens, **THEN** that
  task opens.
- **GIVEN** **Last visited task** is selected, **WHEN** the user uses Home or a
  task's Back action, **THEN** the task overview opens instead of resuming a
  task.
- **GIVEN** the newest local recent task belongs to another workspace, **WHEN**
  bare home opens in the active workspace, **THEN** the task overview opens.

## Out of scope

- Syncing the last selected Kanban/Pipeline/List view between devices.
- Adding a phone-native Pipeline visualization.
- User-configurable selection or ordering of individual rich-row metadata
  fields.
- Changing existing sort, group, archive, pagination, or preview-panel
  behavior.
- Syncing the exact last visited task between devices.
- Redirecting explicit task, session, workflow, Home, or Back destinations to a
  recent task.
- Removing the legacy `kanban_view_mode` API field in this change.
