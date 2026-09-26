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

`AppNavSheet` embeds the collapsible Tasks body from `SessionTaskSwitcherSheet` through `MobileTaskNavigationProvider`. Mount the controller lazily on the first menu request, then retain it across menu closure and responsive changes so creation, editing, and confirmation dialogs keep their drafts. Pass workspace, workflow, archived-task, port-forwarding, and selection context through the outlet. The task title opens the separate picker through `ResponsiveTaskPicker`; there is no separate Task views action.

The embedded `SidebarFilterBar` provides saved view selection, filters, and editing. Task navigation from a non-task route pushes the task route so Back returns to the origin; in-workbench task switching retains its established replace policy. Dismissal restores the actual visible opener unless focus enters a launched dialog. Keep one menu scroller and independent Tasks collapse and New controls. Do not add Office task controls to this Kanban surface.

Only the navigation entry is gated by phone width. A controller already requested by the user remains mounted across breakpoints, so turning the phone sideways cannot discard a draft in a task dialog.

## Verification

A phone E2E starts at /github, opens app navigation, expands Tasks if needed, chooses a saved sidebar view, opens a matching task, and returns with Back. Cover dismissal and a workspace without tasks. Existing task-workbench switching tests protect its navigation policy.

## Navigation entry points

The [unified phone navigation package](../../../plans/unified-mobile-navigation/plan.md)
changes the hamburger to app navigation and the task-title button to the existing
task picker. Its [design](../system-design/unified-mobile-navigation.md)
owns that entry-point change; existing task actions, saved-view controller
lifetime, history, and desktop/tablet guarantees here remain compatibility
requirements. The unified navigation design defines the current entry points.

The September 2026 revision embeds the collapsible Tasks sidebar directly in
the shared phone menu, replacing the Task views action and dedicated pinned
shortcuts. Kanban/Threads/List title dropdowns open display options; Threads
saved-view editing remains inside that surface. See REQ-UI-MOBILE-MENU-004/005
in the unified navigation requirements for the current composition.
