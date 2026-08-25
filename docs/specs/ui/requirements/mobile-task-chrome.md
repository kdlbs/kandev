---
status: active
system: ui
created: 2026-08-24
owners:
  - Kandev
---

# Mobile Task Chrome Requirements

## Overview

Phone users need a compact task header whose controls match the scope of the
surface. Desktop Dockview layout management does not apply to the dedicated
phone task composition, Git operations belong with Changes, and task-level
actions already have a discoverable entry through the task drawer.

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

## Out of scope

- Redesigning task-drawer contents or adding a second task-actions overflow.
- Redesigning the phone Changes panel, its Git eligibility rules, or its
  confirmation semantics.
- Removing layout profiles from desktop Dockview or from Settings.
- Changing backend task movement, Git operations, APIs, persistence, or
  permissions.
- Removing or reorganizing other phone task-top-bar actions.
