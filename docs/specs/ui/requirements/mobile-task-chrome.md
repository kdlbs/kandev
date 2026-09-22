---
status: active
system: ui
created: 2026-08-24
updated: 2026-09-22
owners:
  - Kandev
---

# Mobile Task Chrome Requirements

## Overview

Phone users need a compact task header whose controls match the scope of the
surface. Desktop Dockview layout management does not apply to the dedicated
phone task composition, Git operations belong with Changes, and task-level
actions already have a discoverable entry through the task drawer. Optional
plugin status and actions belong in the shared phone menu so they cannot consume
the task identity region.

## Terminology

- **Phone task workbench:** The dedicated task composition used below the
  `md` breakpoint.
- **Task drawer:** The task navigator opened from the phone header. Each task
  row exposes its applicable task actions, including workflow movement.
- **Changes surface:** The phone task panel selected from bottom navigation that
  owns working-tree, commit, change-request, and remote-contribution controls.

## Requirements

### REQ-UI-MOBILE-TASK-CHROME-001: Contextual Phone Task Chrome

**Intent:** Keep the phone task header focused on task context and navigation,
without duplicating desktop layout management, Git operations, or task actions.

**User story:** As a phone user, I want controls to appear in the surface where
they apply, so that the task header stays understandable and usable on a narrow
viewport.

#### Acceptance criteria

- **AC-UI-MOBILE-TASK-CHROME-001.1:** When a phone task workbench renders, the system shall not expose a layout-profile or saved-layout control in its top bar.
- **AC-UI-MOBILE-TASK-CHROME-001.2:** When a phone task workbench renders Chat, Plan, Files, Changes, Review, Terminal, or a plugin panel, the system shall not expose a general Git-actions overflow in its top bar.
- **AC-UI-MOBILE-TASK-CHROME-001.3:** When a phone user needs task-level actions, the top bar shall retain one touch-reachable task-drawer entry, and the active task's row shall continue to expose applicable actions such as moving the task to another workflow step without a second top-bar task-actions menu.
- **AC-UI-MOBILE-TASK-CHROME-001.4:** When a phone user selects Changes, applicable commit, push, change-request creation, pull, rebase, merge, and remote-contribution recovery actions shall remain reachable through the existing Changes controls and safety flows. Standalone recovery controls and their confirmation actions shall retain at least a 44 CSS-pixel hit target throughout the phone breakpoint below `md`.
- **AC-UI-MOBILE-TASK-CHROME-001.5:** When the phone task title is long, the top bar shall keep its retained actions inside the viewport, avoid document-level horizontal overflow, and provide at least a 44-by-44 CSS-pixel hit target for the task-drawer entry.
- **AC-UI-MOBILE-TASK-CHROME-001.6:** When the same task renders on tablet or desktop, existing task-top-bar and Dockview layout-profile behavior shall remain unchanged.
- **AC-UI-MOBILE-TASK-CHROME-001.7:** When one or more plugins contribute session top-bar status or actions, the phone task header shall keep those contributions out of its persistent row and expose them in the shared phone menu's Plugins section. The menu shall preserve current task, workspace, active-session, and task-session context, identify its presentation as mobile, keep multiple or long contributions inside the menu viewport, and provide at least a 44 CSS-pixel active target for plugin controls. Tablet and desktop shall retain the inline top-bar presentation.

## Out of scope

- Redesigning task-drawer contents or adding a second task-actions overflow.
- Redesigning the phone Changes panel, its Git eligibility rules, or its
  confirmation semantics.
- Removing layout profiles from desktop Dockview or from Settings.
- Changing backend task movement, Git operations, APIs, persistence, or
  permissions.
- Removing or reorganizing first-party phone task-top-bar actions other than
  the plugin contribution placement defined above.

## Navigation entry points

The [unified phone navigation package](../../../plans/unified-mobile-navigation/plan.md)
changes the hamburger to app navigation and the task-title button to the existing
task picker. Its [design](../system-design/unified-mobile-navigation.md)
owns that entry-point change; existing task actions, saved-view controller
lifetime, history, and desktop/tablet guarantees here remain compatibility
requirements. Historical hamburger descriptions above describe the preceding composition;
the unified navigation design defines the current entry points.

The September 2026 revision embeds the collapsible Tasks sidebar directly in
the shared phone menu, replacing the Task views action and dedicated pinned
shortcuts. Kanban/Threads/List title dropdowns open display options; Threads
saved-view editing remains inside that surface. See REQ-UI-MOBILE-MENU-004/005
in the unified navigation requirements for the current composition.

## Implementation plans

- [Mobile task plugin menu](../../../plans/mobile-task-plugin-menu/plan.md)
