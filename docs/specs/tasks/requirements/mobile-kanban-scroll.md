---
status: active
system: tasks
created: 2026-09-15
owners:
  - kandev
---

# Mobile Kanban scrolling

## Overview

Phone users must be able to scroll the board from task cards without picking
up or moving tasks. Tasks owns this change because it narrows the existing
[Kanban task reordering](kanban-task-reordering.md) interaction contract.
This phone exception supersedes that document's drag activation and
keyboard-reorder expectations on mobile.

## Requirements

### REQ-TASKS-MOBILE-KANBAN-SCROLL-001: Scrollable mobile task cards

**Intent:** Remove drag-to-move from the phone Kanban presentation.

#### Acceptance criteria

- **AC-TASKS-MOBILE-KANBAN-SCROLL-001.1:** Below 768 CSS pixels, task cards shall not initiate a drag,
  reorder, or cross-step move through pointer movement or a touch hold. No drag
  overlay, insertion indicator, or drag-only destination strip shall appear.
- **AC-TASKS-MOBILE-KANBAN-SCROLL-001.2:** A vertical swipe starting on a task card shall scroll an
  overflowing column, including after holding the card beyond the previous
  drag activation delay. It shall neither mutate task position/step nor open
  the task. The last task shall remain reachable.
- **AC-TASKS-MOBILE-KANBAN-SCROLL-001.3:** A card tap shall retain task navigation. The visible card
  menu shall retain Move to and its existing destination eligibility and
  failure handling. Horizontal column navigation shall remain available.
- **AC-TASKS-MOBILE-KANBAN-SCROLL-001.4:** Phone cards shall expose no keyboard pickup/reorder affordance.
  At 768 CSS pixels and wider, existing tablet/desktop drag and keyboard
  behavior shall remain available. Resizing shall select the corresponding
  behavior without changing saved view preferences or task order.

## Out of scope

Other draggable surfaces, tablet/desktop interaction redesign, replacement
manual ordering controls, server ordering changes, and new settings.

## Implementation Plans

- [Remove mobile Kanban dragging](../../../plans/remove-mobile-kanban-drag/plan.md)
