---
status: shipped
created: 2026-07-30
owner: kandev
---

# Sidebar Task Completion Icons

## Why

The task sidebar currently gives every finished turn the same green circled check, so users
cannot tell whether the agent merely finished its current turn or the task also reached the end
of its workflow.

## What

- A task with a finished turn that is not on the last step of its workflow shows the green
  `progress-check` icon with a dashed circular outline.
- A task with a finished turn that is on the last step of its workflow keeps the current green
  `circle-check` icon with a continuous circular outline.
- The last step is the final ordered step of the workflow that owns the task.
- Only an exact match between the task's current step and its workflow's last known step is
  treated as workflow completion. Missing workflow or step data uses the progress-check icon
  rather than claiming the workflow is complete.
- Desktop sidebar rows and the mobile task-switcher drawer expose the same distinction through
  their shared task-row rendering.
- Running, background work, scheduling, pending clarification, pending permission, backlog,
  failure, and cancellation icon precedence remains unchanged.

## Scenarios

- **GIVEN** a task whose turn is finished on a non-final workflow step, **WHEN** the task appears
  in the desktop sidebar, **THEN** its green status icon is `progress-check` with a dashed circle.
- **GIVEN** a task whose turn is finished on the final workflow step, **WHEN** the task appears in
  the desktop sidebar, **THEN** its green status icon is the existing continuous
  `circle-check`.
- **GIVEN** finished tasks on non-final and final workflow steps, **WHEN** the user opens the
  mobile task-switcher drawer, **THEN** the rows show the same progress-versus-complete icon
  distinction as desktop.
- **GIVEN** a finished task whose workflow steps are unavailable or do not contain its current
  step, **WHEN** the row renders, **THEN** it shows `progress-check`.
- **GIVEN** a task with a higher-priority active or pending state, **WHEN** the row renders,
  **THEN** that existing state icon is shown regardless of the task's workflow position.

## Out of scope

- Changing task or session lifecycle states, workflow transitions, or step ordering.
- Changing status icons on kanban cards, graphs, task headers, recent-task switchers, or Office
  surfaces.
- Adding backend, API, persistence, or feature-flag state.
- Changing sidebar or mobile task-switcher layout, navigation, touch targets, or scroll behavior.
