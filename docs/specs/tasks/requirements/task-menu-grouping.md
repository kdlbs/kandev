---
status: draft
system: tasks
created: 2026-09-10
owners:
  - Kandev
---

# Task menu grouping requirements

## Overview

Task menus group related actions so users can find them as the action list grows.
The task system owns this contract because the groups describe task operations across task surfaces.
This document extends [task actions menus](task-actions-menu.md) to cover ordering across cards and task rows.

## Requirements

### REQ-TASKS-MENU-GROUPING-001: Consistent action groups

#### Acceptance criteria

- **AC-TASKS-MENU-GROUPING-001.1:** Single-task row menus shall use the following group order and action order:

  | Group | Actions in order |
  | --- | --- |
  | Mark | Pin or Unpin, Color, Priority |
  | Edit | Edit, Rename, Duplicate |
  | Relationships | Create subtask, Nest under, Link, Detach from parent |
  | Move | Move to, Send to workflow |
  | Plugins | Admitted primary plugin actions in registration order |
  | Remove | Archive, Delete |

- **AC-TASKS-MENU-GROUPING-001.2:** Card context and three-dot menus shall use Priority, Edit, Link, Detach from parent, Move to, Send to workflow, primary plugins, Archive, Delete.
  Group boundaries shall match the table. Preview and desktop detail menus shall inherit this order with their existing availability exceptions.
- **AC-TASKS-MENU-GROUPING-001.3:** Menus shall show one thin divider between nonempty groups, with no visible group headings.
  Menus shall show no leading, trailing, or consecutive dividers. Archive and Delete shall share a group.
- **AC-TASKS-MENU-GROUPING-001.4:** Menus shall preserve existing labels, icons, eligibility, disabled states, callbacks, confirmations, selection semantics, and navigation outcomes.
  Duplicate shall remain disabled where it already appears. Archive shall retain neutral styling, and Delete shall retain destructive styling.
- **AC-TASKS-MENU-GROUPING-001.5:** Plugin Edit actions shall remain in the card-only Edit submenu. Plugin Link actions shall remain under Link.
  Primary plugin actions shall remain flat, reactive, and ordered by registration, in their separate group before removal actions.
- **AC-TASKS-MENU-GROUPING-001.6:** Bulk row menus shall retain only existing bulk actions, ordered as Pin, movement actions, then Archive and Delete.
  Archived and unresolved menus shall retain their existing action sets, with the same grouping and divider rules.
- **AC-TASKS-MENU-GROUPING-001.7:** Touch users shall reach the same applicable groups through existing visible task-row and card action controls.
  Phone menus shall scroll internally, respect safe areas, and keep nested choices and the last action reachable without document horizontal overflow.
  Touch action rows shall retain targets of at least 44px. Desktop menus shall retain their existing density.
- **AC-TASKS-MENU-GROUPING-001.8:** Keyboard users shall reach actions in visual order. Escape shall dismiss the menu and restore the existing opener focus behavior.
  Menu interaction shall not select, navigate to, or drag the underlying task as a side effect.

## Out of scope

- Adding Pin, Color, Rename, Duplicate, or nesting capabilities to cards.
- New action handlers, backend contracts, persistence, or localization keys.
- New plugin registration groups, classification by label, or wider plugin eligibility.
- Replacing the mobile menu shell or adding a mobile detail-header trigger.

## Implementation Plans

- [Task menu grouping](../../../plans/task-menu-grouping/plan.md)
