---
status: active
system: tasks
created: 2026-07-15
updated: 2026-08-24
owners:
  - kandev
---

# Task Archive Confirmation Requirements

## Overview

Users can choose whether user-initiated task archiving requires confirmation.
The safer confirmed behavior remains the default, while experienced users can
archive quickly without changing deletion or programmatic archive behavior.

## Requirements

### REQ-TASKS-ARCHIVE-CONFIRMATION-001: Task Archive Confirmation

**Intent:** Let a user control archive confirmation without changing the
meaning of archive, deletion, or agent-driven operations.

#### Acceptance criteria

- **AC-TASKS-ARCHIVE-CONFIRMATION-001.1:** When the preference is missing or
  enabled, a user archive action shall show the existing confirmation summary
  and optional subtask-cascade control.
- **AC-TASKS-ARCHIVE-CONFIRMATION-001.2:** When the preference is disabled, an
  archive action from any desktop or mobile task surface shall archive only the
  requested task without showing the confirmation dialog or cascading to
  subtasks.
- **AC-TASKS-ARCHIVE-CONFIRMATION-001.3:** When a confirmed archive uses the
  cascade option, the system shall exclude the archived tree from replacement
  task selection.
- **AC-TASKS-ARCHIVE-CONFIRMATION-001.4:** When an active task is archived
  successfully, the rendered task and URL shall change together to a surviving
  task or the task overview.
- **AC-TASKS-ARCHIVE-CONFIRMATION-001.5:** When preference persistence fails,
  the control and subsequent archive behavior shall remain at the previously
  persisted value.
- **AC-TASKS-ARCHIVE-CONFIRMATION-001.6:** When archive execution fails after
  temporary navigation, the system shall restore the original task and URL.
- **AC-TASKS-ARCHIVE-CONFIRMATION-001.7:** Delete confirmations and
  programmatic, API, CLI, MCP, or agent-driven archive operations shall remain
  unchanged.

## Out of scope

- Disabling confirmation for deletion or other destructive actions.
- Adding a cascade default when confirmation is disabled.
- Changing the archive API or task-switcher composition.

## System design

- [Task Archive Confirmation System Design](../system-design/archive-confirmation.md)

## Implementation plans

- [Archive Confirmation Preference](../../../plans/archive-confirmation-preference/plan.md)
- [Cascade Archive Navigation](../../../plans/cascade-archive-navigation/plan.md)
