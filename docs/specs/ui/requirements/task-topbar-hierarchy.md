---
status: active
system: ui
created: 2026-09-23
owners:
  - kandev
---

# Task topbar hierarchy

## Overview

Task chrome gives identity and progress priority over diagnostics and workspace
configuration. UI owns this presentation contract; task, workflow, plugin, and
host-metric state retain their existing owners.

## Requirements

### REQ-UI-TASK-TOPBAR-001: Focused task chrome

**Intent:** Keep the current task readable without removing existing controls.

#### Acceptance criteria

- **AC-UI-TASK-TOPBAR-001.1:** Desktop shall retain task identity, workflow
  navigation, linked review status, plugin actions, panel visibility, and task
  actions in the header. Long titles shall truncate without overlap or document
  overflow at supported desktop widths.
- **AC-UI-TASK-TOPBAR-001.2:** Workspace layout, editor, folder, and debug
  controls shall be reachable through a labelled Task tools disclosure. Existing
  availability, archived-state, disabled-state, and action semantics shall remain.
- **AC-UI-TASK-TOPBAR-001.3:** With host metrics enabled and the status bar off,
  the task header shall use one System metrics disclosure. Opening it shall show
  the enabled host metrics in the selected presentation style. Enabling the
  status bar or disabling metrics shall remove the duplicate header entry.
- **AC-UI-TASK-TOPBAR-001.4:** Human assignment shall use a compact avatar or
  unassigned icon, with the full current name available on hover/focus and in the
  picker. Assignment actions and authentication gates shall remain unchanged.
- **AC-UI-TASK-TOPBAR-001.5:** Touch disclosures shall use an inset drawer with
  at least 44 px action targets, contained scrolling, and predictable dismissal.
  Phone navigation shall retain its existing native workflow and tools routes.

## Out of scope

No persisted preference, plugin API, workflow policy, metric collection, or
task lifecycle changes. Other page headers retain their current composition.

## System design

[Task topbar hierarchy](../system-design/task-topbar-hierarchy.md).
