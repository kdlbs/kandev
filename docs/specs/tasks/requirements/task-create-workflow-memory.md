---
status: active
system: tasks
created: 2026-08-08
owners:
  - Kandev
---
# Task Create Workflow Memory Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001: Task Create Workflow Memory

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Users who create tasks in workspaces with multiple workflows repeatedly have to
choose the same workflow whenever Create Task opens from All Workflows or a
different filter. The dialog should remember the workflow that actually created
the previous task in each workspace without coupling task creation to the
current board/list filter.

## Broken behavior

The task-create preference records repository, branch, agent profile, and
executor profile but omits workflow. An unlocked dialog therefore uses the
current page context when it has one and otherwise remains empty when multiple
workflows exist. The selector's local `lastUsedWorkflowId` only reorders options
within one mounted component; it is not a durable default.

## What

- The standard Create Task dialog remembers the workflow used by the most
  recent successful task creation separately for each workspace.
- A valid remembered workflow is selected even when the board or task list is
  currently filtered to another workflow.
- A workflow selected manually in the currently open dialog immediately becomes
  the effective selection for that submission.
- Feature-specific flows that explicitly lock a workflow keep their locked
  workflow and do not inherit the standard dialog's remembered default.
- Remembered workflow IDs are used only when they are visible choices in the
  current workspace. Missing, deleted, hidden, or cross-workspace IDs are
  ignored.
- Without a valid remembered workflow, the dialog falls back to a valid visible
  workflow supplied by its page context, then to the sole visible workflow.
- When exactly one visible workflow exists, that workflow is implicit and the
  workflow selector is omitted.
- When multiple visible workflows exist and neither remembered nor contextual
  defaults are valid, the selector remains visible and requires a choice.
- HTTP and WebSocket task creation record the effective workflow only after the
  task is created successfully.
- Cancelling a dialog or changing the selector without creating a task does not
  alter the remembered workflow.
- Desktop and mobile use the same workflow-resolution policy. The feature does
  not change dialog layout, navigation, scrolling, or touch behavior.

## Data model

`users.settings.task_create_last_used` gains:

```text
workflow_ids_by_workspace  object<string, string>
  key: workspace ID
  value: workflow ID used by the latest successful task creation in that workspace
```

The map is optional for compatibility with existing settings rows. An absent or
empty map means there is no remembered workflow. Updating one workspace entry
must preserve all other entries and the existing repository, branch, agent
profile, and executor profile fields.

## Failure modes

- If user settings are unavailable, the dialog does not invent or persist a
  browser-local workflow default; it uses the valid contextual or sole-workflow
  fallback once available.
- If a remembered workflow no longer belongs to the current visible workflow
  set, the dialog ignores it and continues through the fallback order.
- Failure to record last-used settings does not roll back an otherwise
  successful task creation; it follows the existing warning-only task-create
  preference behavior.

## Persistence guarantees

Workflow memory is a portable backend-owned user preference under
[ADR 0028](../../../decisions/0028-task-create-last-used-source-of-truth.md),
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md), and
[ADR-2026-08-08-workspace-scoped-task-create-workflow-memory](../../../decisions/2026-08-08-workspace-scoped-task-create-workflow-memory.md).
It survives backend restarts, browser changes, and workspace switches. The
frontend's queued overlay is transient and only bridges the interval between a
successful create response and the authoritative settings update.

## Scenarios

- **GIVEN** workspace A remembers Dev and the board is filtered to PR Review,
  **WHEN** the user opens standard Create Task, **THEN** Dev is selected.
- **GIVEN** workspace A remembers Dev and workspace B remembers Support,
  **WHEN** the user switches between the workspaces and opens standard Create
  Task in each, **THEN** each workspace restores its own remembered workflow.
- **GIVEN** Dev is restored and the user manually chooses PR Review, **WHEN**
  the user successfully creates the task, **THEN** the task uses PR Review and
  the next standard Create Task in that workspace restores PR Review.
- **GIVEN** a feature-specific task-create flow locks Support, **WHEN** the
  workspace remembers Dev, **THEN** Support remains selected and locked.
- **GIVEN** a remembered workflow was deleted or hidden, **WHEN** standard
  Create Task opens from a valid visible workflow context, **THEN** the context
  workflow is selected.
- **GIVEN** exactly one visible workflow and no valid remembered workflow,
  **WHEN** standard Create Task opens, **THEN** the workflow is implicit and no
  workflow selector is rendered.
- **GIVEN** several visible workflows and no valid remembered or contextual
  workflow, **WHEN** standard Create Task opens, **THEN** the workflow selector
  is visible with no selected workflow.
- **GIVEN** the user changes the workflow and cancels Create Task, **WHEN** the
  dialog is opened again, **THEN** the previously successful workflow remains
  the remembered default.

## Out of scope

- Changing the active board/list workflow filter when Create Task resolves a
  different remembered workflow.
- Remembering workflow choices from cancelled or failed task creation.
- Exposing workflow history in Settings.
- Changing hidden-workflow rules or feature-specific locked task-create flows.
- Changing the task-create dialog's desktop or mobile composition.