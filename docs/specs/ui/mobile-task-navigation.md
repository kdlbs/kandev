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
- Workflow is a primary mobile Kanban navigation dimension, not a setting hidden in the secondary display menu. The current workflow and step are always visible together in the board navigation control, including when only one workflow exists.
- Opening the board navigation control exposes available workflows and the focused workflow's steps in one bottom drawer. Choosing a workflow makes it the active workflow for the board, task creation, and multi-select actions through the existing saved workflow selection; previous/next step buttons and horizontal swipe remain equivalent transient step shortcuts.
- The task list is the primary vertical scroller. The document and workflow container do not require horizontal scrolling.
- Search and live workflow/task updates choose a deterministic visible fallback if the focused workflow disappears.
- Pipeline is not offered or rendered on mobile. A saved desktop Pipeline preference falls back to Kanban on mobile without overwriting that preference.
- Tapping a mobile Home task opens that task directly. Task actions remain available from the card's explicit context menu; the intermediate task action sheet is absent.
- Editing a task from a mobile context menu exposes its title even after work has started. The existing lock on a started task's prompt remains unchanged.
- The mobile Home menu and Dockview task switcher open as inset, card-style bottom surfaces with internal vertical scrolling and safe-area spacing, rather than edge-to-edge side sheets.
- The active-session control at the top of mobile Dockview shows the active agent's icon beside its session label.
- Desktop and tablet Kanban, context menus, drag/drop, and workflow filtering retain their existing behavior.

## Scenarios

- **GIVEN** a mobile task switcher with a task in a workflow containing multiple steps, **WHEN** the user opens Task actions, **THEN** Move to is visible and selecting another step moves the task there.
- **GIVEN** a mobile action menu with more items than fit on screen, **WHEN** it opens, **THEN** it is inset within the viewport and scrolls internally with touch-sized rows.
- **GIVEN** a nested mobile action such as Move to, Link, or Send to workflow, **WHEN** the user opens it, **THEN** its choices remain within the same bottom-sheet area and are selectable without horizontal overflow.
- **GIVEN** tasks in several workflows, **WHEN** mobile Kanban opens, **THEN** exactly one workflow board is mounted and the visible board control names both its workflow and active step.
- **GIVEN** a workflow with several steps, **WHEN** the user opens the board drawer, **THEN** workflow choices and the active workflow's step choices are reachable in that same surface.
- **GIVEN** several workflows, **WHEN** the user chooses one from the board drawer, **THEN** that workflow becomes the visible and active workflow for subsequent task creation and board actions.
- **GIVEN** a workflow with several steps, **WHEN** the user selects a step or uses previous/next, **THEN** only the chosen step's cards are presented as the active column and its count/WIP state remains visible.
- **GIVEN** the focused workflow no longer matches search/filter results, **WHEN** the visible workflow set updates, **THEN** mobile Kanban focuses the first visible workflow without leaving an empty stacked board.
- **GIVEN** Pipeline is saved as the user's desktop view, **WHEN** Home opens on mobile, **THEN** Kanban renders, Pipeline is absent from the mobile view choices, and the saved preference remains Pipeline.
- **GIVEN** a task card on mobile Home, **WHEN** the user taps the card body, **THEN** the task route opens immediately and no task action sheet appears.
- **GIVEN** a started task on mobile Home, **WHEN** the user chooses Edit from its context menu, **THEN** the title can be changed while the prompt remains locked.
- **GIVEN** the mobile Home menu or Dockview task switcher is opened, **WHEN** its content exceeds the viewport, **THEN** an inset bottom card remains within the safe area and scrolls internally.
- **GIVEN** a mobile Dockview task with an active session, **WHEN** its chat panel is visible, **THEN** the active-session control shows the session agent's icon and label.
- **GIVEN** a desktop viewport, **WHEN** the same task menus and Kanban open, **THEN** their existing desktop interaction and layout remain unchanged.

## Out of scope

- Removing or redesigning Pipeline on desktop or tablet.
- Persisting the transient focused mobile step across reloads.
- Changing backend task-move contracts, workflow ordering, or task permissions.
- Unlocking or changing a started task's prompt.

## Implementation plan

[Mobile task navigation refinement](../../plans/mobile-task-navigation-refinement/plan.md)
