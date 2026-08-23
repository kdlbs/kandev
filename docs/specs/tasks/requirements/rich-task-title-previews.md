---
status: active
system: tasks
created: 2026-08-18
owners:
  - tbd
---
# Rich task title previews Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-RICH-TASK-TITLE-PREVIEWS-001: Rich task title previews

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Task titles can be too long for a task row. Users also need a fast way to see
the direct subtasks and contribution state without opening the parent task.

## What

- A fine-pointer task row shows a preview after the user points at its title.
- The preview shows the full title and as many as 12 direct, active subtasks.
- Each subtask row shows its task state and its GitHub or GitLab contribution state.
- A keyboard user can focus the title trigger and open the same preview.
- A keyboard user can open a subtask without opening the parent task.
- A coarse-pointer device keeps the title's direct navigation action.
- On a task page, the mobile task switcher gives access to the same task tree.
- GitHub and GitLab status summaries use one provider-neutral presentation contract.

## Data model

The feature adds no persistent data. It reads task and subtask data from the
existing task store. It reads pull requests from `taskPRs.byTaskId`. It reads
merge requests from `taskMRs.byWorkspaceId[workspaceId]`.

The GitLab hydration hook shares an in-flight workspace request between mounted
consumers. A later mount can start a fresh request. A successful request replaces
the workspace cache. A failed request clears that workspace cache.

## API surface

The feature adds no HTTP route, WebSocket message, or plugin API. It uses the
existing workspace task, pull-request, and merge-request contracts.

The public status-summary presentation uses generic change-request concepts.
Provider-specific code supplies labels, icons, test identifiers, and status rows.

## Failure modes

- If a task has no active direct subtask, the preview shows only the full title.
- If GitLab refresh fails, the UI removes cached merge-request data for that workspace.
- If the device has no fine pointer, the UI does not mount a hover preview.

## Scenarios

- **GIVEN** a clipped title, **WHEN** a user points at it, **THEN** the preview shows the full title.
- **GIVEN** a parent with active subtasks, **WHEN** the preview opens, **THEN** it shows each direct subtask up to the limit.
- **GIVEN** a focused title trigger, **WHEN** a user presses Enter, **THEN** the preview opens and moves focus into its interactive content.
- **GIVEN** an open preview, **WHEN** a user presses Enter on a subtask, **THEN** the app opens the subtask and not the parent.
- **GIVEN** a coarse-pointer device, **WHEN** a user taps the title, **THEN** the app opens the task without a preview.
- **GIVEN** two mounted GitLab hydration consumers, **WHEN** both request one workspace, **THEN** the frontend sends one request.
- **GIVEN** cached GitLab data, **WHEN** its refresh fails, **THEN** the cached workspace data is removed.

## Out of scope

- The feature does not add nested preview levels.
- The feature does not add backend status projection fields.
- The feature does not change mobile task navigation.

## Implementation plan

See [the implementation plan](../../../plans/rich-task-title-previews/plan.md).