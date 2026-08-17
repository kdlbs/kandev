---
status: building
created: 2026-08-13
owner: kandev
---

# Sidebar empty task alignment

## Why

When the expanded desktop app sidebar has no tasks, its empty-state message starts farther left than the surrounding sidebar section titles. The inconsistent inset makes the task section look detached from the sidebar hierarchy.

## What

- In the expanded desktop app sidebar, the **No tasks yet.** message aligns its left text edge with the **Tasks** section title.
- The task list keeps its full sidebar width and transparent background while empty.
- Task rows, task-group headers, other sidebar sections, and other task-switcher hosts keep their existing insets.
- Phone navigation keeps its separate mobile composition and existing task-list inset.

## Scenarios

- **GIVEN** the expanded desktop app sidebar contains no tasks, **WHEN** the Tasks section renders, **THEN** the left text edge of **No tasks yet.** aligns with the left text edge of the **Tasks** title.
- **GIVEN** the desktop task list is empty, **WHEN** its layout is measured, **THEN** its transparent scroll surface still spans the full width of the sidebar navigation area.
- **GIVEN** a phone-sized viewport, **WHEN** the task listing renders, **THEN** the desktop app sidebar remains hidden and the mobile task surface keeps its existing composition and inset.

## Out of scope

- Changing task-row, task-group, filter, or non-task sidebar spacing.
- Changing empty states outside the desktop app sidebar Tasks section.
- Redesigning mobile task navigation or introducing a new mobile surface.

## Implementation plan

- [Sidebar empty task alignment plan](../../plans/sidebar-empty-task-alignment/plan.md)
