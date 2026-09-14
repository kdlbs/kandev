---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-TASK-VIEWS-001
---
# Mobile task-view access design

## Boundary and mapping

`REQ-UI-MOBILE-TASK-VIEWS-001` maps to the shared menu entry and overlay handoff described below. Existing sidebar view state and `SessionTaskSwitcherSheet` own filtering and task actions. This design does not merge those views with GitHub or Threads saved queries.

## Components and flow

Extend `AppNavSections` and its hoisted `useAppNavDialogs` controls with a localized Task views action for phone Kanban workspaces. Close the navigation surface before opening the task drawer. Mount the existing `SessionTaskSwitcherSheet` lazily in the hoisted dialogs, with the current workspace and workflow context and presentation=drawer. Do not mount it before the first request. Once requested, retain its controller until navigation or workspace changes so closing the drawer can hand off to task creation, editing, and confirmation dialogs without losing their state.

The existing drawer's `SidebarFilterBar` provides saved view selection/editing. Task navigation from a non-task route pushes the task route so Back returns to the origin; in-workbench task switching retains its established navigation policy. Dismissal restores a visible app-navigation trigger. Do not add Office task controls to this Kanban surface.

Only the navigation entry is gated by phone width. A controller already requested by the user remains mounted across breakpoints, so turning the phone sideways cannot discard a draft in a task dialog.

## Verification

A phone E2E starts at /github, opens app navigation, selects Task views, chooses a saved sidebar view, opens a matching task, and returns with Back. Cover dismissal and a workspace without tasks. Existing task-workbench switching tests protect its navigation policy.
