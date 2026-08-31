---
status: active
system: ui
created: 2026-08-31
owners:
  - kandev
---

# Command Panel Task Activity Icons Requirements

## Overview

The command panel lists tasks from all workflow steps. Users need a clear icon to find active work without opening each task.

The UI system owns this presentation contract. The task system continues to own task state, session state, and activity data.

## Requirements

### REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001: Task activity icon parity

**Intent:** Make active tasks easy to find and keep task-state icons consistent with the task sidebar.

#### Acceptance criteria

- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.1:** When a task is active, each command-panel result shall show the same spinning task icon as the task sidebar.
- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.2:** When a task is not active, each result shall show the same non-spinning task-state icon as the task sidebar, including the workflow-complete icon for a review task on its final workflow step.
- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.3:** The icon shall use task-level activity and session state. A workflow-step name shall not control the icon.
- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.4:** When live task data changes, an open command panel shall update the affected icon without a new search. An accepted live projection that clears foreground activity shall clear the spinner even when the original search response still reports activity.
- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.5:** The task title, workflow-step badge presentation and metadata format, selection behavior, and navigation result shall not change. The badge content shall reflect the accepted effective workflow placement.
- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.6:** Desktop and phone layouts shall show the same icon state without reducing title readability.
- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.7:** The icon shall have an accessible state description and shall not become a separate action.
- **AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.8:** When an accepted newer live task projection changes a task's workflow placement, an open command-panel result shall show the live workflow-step badge without a new search. A stale live projection shall not replace the HTTP placement.

## Out of scope

- Changes to task, session, workflow, or activity-state rules.
- Changes to task search ranking, result limits, or workflow-step badge presence, styling, or navigation semantics.
- New task or session API fields.
- Changes to task-row actions or command-panel navigation.
