---
status: shipped
created: 2026-07-17
owner: kandev
---

# Mobile Task Navigation

## Why

Mobile users need the same task controls as desktop without relying on long press, clipped popovers, or boards that scroll in two directions. Moving a task, changing workflow context, and choosing a workflow step must remain comfortable with one hand on a narrow viewport.

## What

- Task action controls are visible and touch-reachable on mobile.
- Mobile task actions preserve desktop capabilities, including same-workflow **Move to**, cross-workflow **Send to workflow**, linking, pinning, renaming, coloring, archiving, and deleting when those actions are available on desktop.
- Context and dropdown action menus below the app's 640px mobile breakpoint stay within the viewport, use bottom-sheet presentation, contain their own vertical overflow, respect the bottom safe area, and provide touch targets at least 44px high.
- Mobile Kanban renders one focused workflow and one focused step at a time when the user has several workflows.
- When the persisted workflow filter is “All workflows,” choosing a focused workflow changes only mobile presentation; it does not replace the saved filter.
- Users can change the focused workflow and step through labeled bottom drawers. Previous/next step buttons and horizontal swipe remain equivalent shortcuts.
- The task list is the primary vertical scroller. The document and workflow container do not require horizontal scrolling.
- Search and live workflow/task updates choose a deterministic visible fallback if the focused workflow disappears.
- Desktop and tablet Kanban, context menus, drag/drop, and workflow filtering retain their existing behavior.

## Scenarios

- **GIVEN** a mobile task switcher with a task in a workflow containing multiple steps, **WHEN** the user opens Task actions, **THEN** Move to is visible and selecting another step moves the task there.
- **GIVEN** a mobile action menu with more items than fit on screen, **WHEN** it opens, **THEN** it is inset within the viewport and scrolls internally with touch-sized rows.
- **GIVEN** a nested mobile action such as Move to, Link, or Send to workflow, **WHEN** the user opens it, **THEN** its choices remain within the same bottom-sheet area and are selectable without horizontal overflow.
- **GIVEN** All workflows with tasks in three workflows, **WHEN** mobile Kanban opens, **THEN** exactly one workflow board is mounted and the workflow drawer can focus either of the other workflows.
- **GIVEN** a workflow with several steps, **WHEN** the user opens the step drawer or uses previous/next, **THEN** only the chosen step's cards are presented as the active column and its count/WIP state remains visible.
- **GIVEN** the focused workflow no longer matches search/filter results, **WHEN** the visible workflow set updates, **THEN** mobile Kanban focuses the first visible workflow without leaving an empty stacked board.
- **GIVEN** a desktop viewport, **WHEN** the same task menus and Kanban open, **THEN** their existing desktop interaction and layout remain unchanged.

## Out of scope

- Redesigning Pipeline or List view visualization; shared mobile menu containment still applies to their action menus.
- Persisting the transient focused mobile workflow or step across reloads.
- Changing backend task-move contracts, workflow ordering, or task permissions.
