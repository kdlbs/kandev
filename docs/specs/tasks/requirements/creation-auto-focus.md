---
status: active
system: tasks
created: 2026-09-13
owners:
  - kandev
---

# Task creation auto-focus

## Overview

Let users create tasks without interrupting the work they are viewing. Tasks
owns this contract because it governs completion of user-initiated task creation
across listing, task-detail, and mobile entry points.

Auto-focus means automatically opening or selecting the newly created task or
session, including navigation and replacing a task preview. It does not mean
keyboard focus within the creation form.

## Requirements

### REQ-TASKS-CREATION-AUTO-FOCUS-001: Optional focus after creation

**Intent:** Preserve current creation behavior by default while allowing users
to keep their current working context.

#### Acceptance criteria

- **AC-TASKS-CREATION-AUTO-FOCUS-001.1:** Settings > Task Behavior shall offer a per-user “Auto-focus new tasks” switch, enabled for new users and existing users without a saved value. With it enabled, each entry point shall preserve its current navigation and selection behavior.
- **AC-TASKS-CREATION-AUTO-FOCUS-001.2:** After the user saves the switch as disabled, successful task creation shall leave the current route, selected task/session, preview, and active layout unchanged. This includes ordinary creation, creation with an agent, passthrough agents, and planning tasks. With no task selected, none shall be selected automatically.
- **AC-TASKS-CREATION-AUTO-FOCUS-001.3:** Disabling auto-focus shall preserve task creation, requested agent or plan execution, task-list updates, successful dialog dismissal, and manual opening of the created task. It shall not change existing creation errors, retries, or cancellation behavior.
- **AC-TASKS-CREATION-AUTO-FOCUS-001.4:** Saving either switch value shall persist it across reloads through the user's settings. Unsaved edits and discarded edits shall not alter creation behavior. A failed save shall retain the previously effective value and expose the existing settings error/retry flow.
- **AC-TASKS-CREATION-AUTO-FOCUS-001.5:** Desktop and phone shall expose the same setting with visible explanatory text, an accessible switch, and the existing shared Save changes flow. Phone controls shall have a touch target of at least 44px, with wrapped text and no horizontal page overflow. After creation with auto-focus disabled, keyboard focus shall return to the surviving creation opener or an appropriate control on the unchanged surface.

## Out of scope

Per-task overrides; changing agent auto-start policy; changing explicit task
selection, task editing, additional-session creation, or Quick Chat behavior;
adding navigation for background/API/MCP creation. No new release toggle.

## Implementation plans

- [Creation auto-focus plan](../../../plans/task-creation-auto-focus/plan.md)
