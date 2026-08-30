---
status: active
system: ui
created: 2026-08-22
owners:
  - kandev
---
# Task transcript history visibility Requirements

## Overview

The transcript can lose the original task prompt after a reload. Long activity groups can also make older history difficult to reach. An older page can add tool events only inside an existing collapsed row. The user then sees no new entry and must select **Load older messages** again.

## Requirements

### REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001: Task transcript history visibility

**Intent:** The transcript can lose the original task prompt after a reload. Long activity groups can also make older history difficult to reach. An older page can add tool events only inside an existing collapsed row. The user then sees no new entry and must select **Load older messages** again.

#### Acceptance criteria

- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.1:** When visible session history has no user-authored message, show the task description as the first user prompt.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.2:** Keep the task description visible while agent or environment messages load.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.3:** Treat a visible, stored user message as authoritative. Do not also show the task-description fallback in this case.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4:** When the user scrolls to the oldest loaded point, load older history without a separate button action.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5:** Continue automatic loading while older pages only extend a collapsed activity row.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6:** Do not require another user action before a new standalone transcript entry appears.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.7:** Stop automatic loading in these cases:
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.8:** Older content moves the load boundary above the preload region.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9:** When the first user prompt of a session is loaded, the transcript shall not show an older-page control.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10:** When a previously opened session is revisited after more than one message page was persisted while it was inactive, the transcript shall reconcile to a contiguous newest window and upward pagination shall reach every persisted user prompt without gaps or duplicate rows.

## Migrated source detail

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
- Reconcile a revisited session to one contiguous newest window before older
  pagination resumes. Stale cached rows must not create an unreachable gap.

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

### Revisit after inactive-session activity

- **GIVEN** a session was opened and its older rows remain cached.
- **GIVEN** more than one newest-page window is persisted while that session is
  inactive.
- **WHEN** the user revisits the session and navigates upward.
- **THEN** the transcript exposes every persisted user prompt in chronological
  order without a skipped middle range or duplicate row.

## Out of scope

- Backfill or repair missing message records in the database.
- Change message persistence, API contracts, or transcript ordering.
- Expand activity groups by default.
- Remove the explicit older-page fallback action.
- Load the complete transcript before the user navigates upward.

## Implementation plan

- [Hide redundant older-message controls](../../../plans/hide-redundant-older-messages-control/plan.md)
- [Reconcile inactive-session transcript windows](../../../plans/inactive-session-transcript-reconciliation/plan.md)
