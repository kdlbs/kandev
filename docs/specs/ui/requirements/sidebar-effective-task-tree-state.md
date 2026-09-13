---
status: active
system: ui
created: 2026-09-09
owners:
  - kandev
---

# Sidebar Effective Task Tree State Requirements

## Overview

Saved sidebar views render a parent task and its nested child tasks as one tree. When a view
groups or sorts by state, the tree must remain discoverable as active while any included member
is running, even when the parent itself is ready for review, waiting for input, or complete.

The UI system owns this derived presentation contract. The task system remains authoritative for
each task's persisted state and each session's runtime state.

## Terminology

- **Included task tree:** A root task and every nested descendant that remains in the active
  sidebar view after filtering.
- **Effective tree state:** The derived state used only to place and sort an included task tree.
  It does not replace any member's own state.

## Requirements

### REQ-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001: Active Task Tree Placement

**Intent:** Users can find all active delegated work in the sidebar without mistaking an active
task tree for completed work.

**User story:** As a user, I want a task tree with running child work to remain in progress, so
that parent readiness or completion does not hide active delegated work.

#### Acceptance criteria

- **AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.1:** When an included descendant is in progress,
  state grouping shall place the root and its included descendants in the **In progress** group,
  even when the root is ready for review, waiting for input, failed, cancelled, or completed.
- **AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.2:** When no included member is in progress but an
  included member is scheduling, state grouping shall place the tree in the **Scheduling** group.
- **AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.3:** State grouping shall place a task tree in the
  **Completed** group only when the root and every included descendant are completed.
- **AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.4:** Each parent and descendant row shall retain
  its own state indicator and accessible state label; effective tree state shall change only tree
  placement and state sorting.
- **AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.5:** State sorting shall use the same effective
  tree state as state grouping.
- **AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.6:** Desktop and mobile task sidebars shall derive
  the same effective state for the same included task tree.
- **AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.7:** A descendant excluded by the active sidebar
  filters shall not affect the visible tree's effective state.

## Out of scope

- Changing persisted task or session states.
- Changing row icons, labels, task-tree nesting, or group-header layout.
- Changing grouping dimensions other than state.
- Making a filtered-out descendant visible solely because it affects the unfiltered tree.
