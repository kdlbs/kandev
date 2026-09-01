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
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.6:** When a desktop user selects an
  Agent tab and reloads the same task, the selected current session shall remain
  the effective active session and Agent tab without creating a user pin. The
  restored selection must belong to the active task and current environment.
  If the selection is invalid or ambiguous, Kandev shall use the normal
  active-session fallback.

### REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002: Agent Tab Rename Isolation

**Intent:** Keep inline Agent-tab name editing independent from the desktop
workbench commands attached to the surrounding tab.

#### Acceptance criteria

- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.1:** When a fine-pointer user
  chooses **Rename** for an Agent tab, the tab shall show a focused inline input
  with its current name selected.
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.2:** While the inline rename input
  is active, double-clicking its text shall preserve native text selection and
  shall not maximize or restore the surrounding Dockview group.
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.3:** Outside rename mode,
  double-clicking an eligible desktop workbench tab shall retain the existing
  maximize-or-restore behavior.
- **AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.4:** Phone and tablet session
  controls shall retain their existing rename and navigation behavior without
  exposing desktop Dockview maximize gestures.

## Out of scope

- Changing session creation, lifecycle, ordering, or backend APIs.
- Changing saved desktop layout geometry or the active-panel rules for valid
  non-Agent panels.
- Changing session-name persistence or the rename commit, cancel, and failure
  behavior.
- Redesigning phone or tablet task navigation and session controls.

## Implementation plan

- [Command-center task focus repair](../../../plans/command-center-task-focus/plan.md)
- [Agent tab rename double-click isolation](../../../plans/agent-tab-rename-double-click/plan.md)
