---
status: implemented
created: 2026-08-22
---

# Task transcript history visibility

## Why

The transcript can lose the original task prompt after a reload. Long activity
groups can also make older history difficult to reach. An older page can add
tool events only inside an existing collapsed row. The user then sees no new
entry and must select **Load older messages** again.

Users need continuous upward navigation from the latest response to the first
prompt. This requirement includes sessions with more than 100 tool events.

## What

- When visible session history has no user-authored message, show the task
  description as the first user prompt.
- Keep the task description visible while agent or environment messages load.
- Treat a visible, stored user message as authoritative. Do not also show the
  task-description fallback in this case.
- When the user scrolls to the oldest loaded point, load older history without
  a separate button action.
- Continue automatic loading while older pages only extend a collapsed activity
  row.
- Do not require another user action before a new standalone transcript entry
  appears.
- Stop automatic loading in these cases:
  - Older content moves the load boundary above the preload region.
  - The session start is reached.
  - A request makes no progress.
- Keep the current reading position stable as older content appears above it.
- Keep the explicit load action as a fallback for an error or a no-progress
  response.
- Show the first prompt at the start after all older history is loaded.
- Apply the same transcript rule on desktop and mobile.

## Scenarios

### Agent-only history

- **GIVEN** a task has a description and visible agent or environment messages.
- **GIVEN** the session has no visible user message.
- **WHEN** the transcript loads or reloads.
- **THEN** the task description remains the first message.
- **THEN** the other visible messages follow it.

### Stored user prompt

- **GIVEN** a task has a description and a visible stored user message.
- **WHEN** the transcript loads.
- **THEN** the stored user message represents the prompt.
- **THEN** the transcript does not show a duplicate task-description fallback.

### Empty task description

- **GIVEN** a task has no description and has agent-only history.
- **WHEN** the transcript loads.
- **THEN** the transcript does not add an empty fallback message.

### Collapsed activity spans older pages

- **GIVEN** a transcript has more than 100 tool events in one collapsed activity
  row.
- **GIVEN** older pages contain no standalone transcript entry.
- **WHEN** the user scrolls to the oldest loaded point.
- **THEN** the transcript loads additional pages without repeated button
  actions.
- **THEN** the current reading position remains stable.

### First prompt after complete pagination

- **GIVEN** the first prompt is older than the initial message window.
- **WHEN** the user continues to scroll upward until the session start.
- **THEN** the first prompt is visible at the start of the transcript.
- **THEN** no older-page control remains.

## Out of scope

- Backfill or repair missing message records in the database.
- Change message persistence, API contracts, or transcript ordering.
- Expand activity groups by default.
- Remove the explicit older-page fallback action.
- Load the complete transcript before the user navigates upward.

## Implementation plan

See [the implementation plan](../../plans/task-prompt-transcript-visibility/plan.md).
