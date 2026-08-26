---
status: active
system: ui
created: 2026-08-26
owners:
  - kandev
---

# Task Agent Tab Reconciliation Requirements

## Overview

A desktop task can have several agent sessions. The task system owns session
membership, while the UI owns the projection of those sessions into task
workbench tabs. Every current session must appear without requiring a page
refresh, regardless of whether session hydration or workbench readiness
finishes first.

## Requirements

### REQ-UI-TASK-AGENT-TAB-RECONCILIATION-001: Task Agent Tab Reconciliation

**Intent:** Show a complete and current set of agent chats whenever a desktop
task workbench becomes usable.

#### Acceptance criteria

- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.1:** When the active task's session
  list and desktop workbench become ready in either order, the workbench shall
  show one Agent tab for every current task session. A page refresh shall not
  be necessary.
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.2:** The effective active session
  shall own the active Agent tab. Other current sessions shall appear as
  inactive sibling tabs without taking focus from a valid selected non-Agent
  panel.
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.3:** When session membership changes
  after workbench readiness, the UI shall add new session tabs and remove stale
  session tabs from the active task.
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.4:** When Cmd+K opens a task that
  already has multiple sessions, all Agent tabs shall appear during the first
  task render without a manual reload.
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.5:** Phone and tablet task surfaces
  shall continue to project the same task session membership through their
  existing session controls without mounting desktop workbench tabs.

## Out of scope

- Changing session creation, lifecycle, ordering, or backend APIs.
- Changing saved desktop layout geometry or the active-panel rules for valid
  non-Agent panels.
- Redesigning phone or tablet task navigation and session controls.

## Implementation plan

- [Command-center task focus repair](../../../plans/command-center-task-focus/plan.md)
