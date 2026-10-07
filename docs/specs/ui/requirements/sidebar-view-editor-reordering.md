---
status: active
system: ui
created: 2026-10-07
owners:
  - kandev
---

# Sidebar view editor reordering requirements

## Overview

Drag handles replace paired reorder arrows in the sidebar view editor, leaving more space for rule fields.
UI owns this reusable editor interaction. Existing contracts own sort priority, task-row presentation, and personal color settings.

This additive interaction contract extends [sort chains](sidebar-running-first-activity-sort.md) and
[automatic colors](sidebar-automatic-task-colors.md). It does not change their saved data or ranking semantics.

## Requirements

### REQ-UI-SIDEBAR-VIEW-REORDER-001: Compact and accessible list ordering

**Intent:** Users reorder editor lists through visible grips, with an explicit alternative for touch and keyboard users.

#### Acceptance criteria

- **AC-UI-SIDEBAR-VIEW-REORDER-001.1:** Sort rules, automatic color rules, and task-row details shall expose leading drag handles.
  Sort and automatic-color rows shall show no separate up/down reorder buttons.
  Their fine-pointer control region shall consume less horizontal space than the current control region.
- **AC-UI-SIDEBAR-VIEW-REORDER-001.2:** Dragging a handle shall move its complete item to any valid position within its list.
  Intermediate items shall retain their relative order. Field values, colors, directions, enabled states, and visibility shall remain attached to their items.
- **AC-UI-SIDEBAR-VIEW-REORDER-001.3:** Each reorderable item shall expose a visible More menu with Move up and Move down actions.
  Boundary actions shall be disabled. The menu shall work by tap and keyboard without dragging, hovering, or long pressing.
  Opening a menu shall not reorder an item.
- **AC-UI-SIDEBAR-VIEW-REORDER-001.4:** Handles shall support keyboard pickup, movement, drop, and cancellation with visible focus and localized position announcements.
  After a reorder, focus shall remain on the same item's control.
- **AC-UI-SIDEBAR-VIEW-REORDER-001.5:** Cancelled, unchanged, outside-list, or stale-item drops shall not change data or create a settings write.
  Selectors, switches, remove actions, and scrolling outside a handle shall not start a reorder.
- **AC-UI-SIDEBAR-VIEW-REORDER-001.6:** Phone and coarse-pointer users shall retain the existing drawer with stacked sort cards and one editor scroll body.
  Handles, More controls, remove actions, and menu actions shall have targets of at least 44 CSS pixels.
  Controls shall remain within their cards and viewport, without document horizontal overflow.
- **AC-UI-SIDEBAR-VIEW-REORDER-001.7:** Sort and task-row reorders shall use the current view's draft and save/discard behavior.
  Automatic-color reorders shall use existing immediate personal settings updates and write-failure recovery.
  Reload shall restore saved order. Reordering shall preserve the open task and conversation.
- **AC-UI-SIDEBAR-VIEW-REORDER-001.8:** A single sort rule shall remain editable with reorder and removal unavailable.
  Custom shall remain a standalone sort. Existing addition limits and validation messages shall retain their behavior.

## Out of scope

- Adding filter order, multiple grouping dimensions, or reorderable editor section headings.
- Changing task-list group order, navigation customization, view chips, or Threads sorting.
- Changing sort comparators, personal-color matching, persistence schemas, or workspace ownership.

## Implementation plan

- [Sidebar view editor reordering](../../../plans/sidebar-view-editor-reordering/plan.md)
