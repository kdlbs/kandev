---
status: active
system: ui
created: 2026-08-22
owners:
  - kandev
---

# Task transcript history visibility requirements

## Overview

Task transcripts open on a bounded newest window and load older history as the
user navigates upward. A bounded window is not the start of the conversation.
The transcript must not substitute the task description for an unloaded user
prompt, and upward navigation must not require routine use of a manual loader.

## Terms

- **Visible start:** The first stored user prompt, normally prompt `#1`.
- **Task-description fallback:** A synthetic user row used only for legacy
  history that contains no stored user prompt.
- **Preload region:** The area near the oldest loaded edge where upward
  pagination can continue without another user action.

## Requirements

### REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001: Task transcript history visibility

**Intent:** Users can navigate continuously from the newest response to the
first prompt without mistaking a bounded history window for the conversation
start.

#### Acceptance criteria

- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.1:** When older session history remains and the loaded window contains no user-authored message, the transcript shall not show the task description as a user prompt.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.2:** The transcript shall show each stored user prompt at its chronological position after that prompt is loaded.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.3:** When prompt `#1` is loaded, the transcript shall treat it as the visible start and shall not expose older internal rows through transcript pagination or an older-page control.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4:** When persisted history is exhausted without a stored user prompt, the transcript shall show a non-empty task description as the first user prompt and shall not add an empty fallback.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5:** When the user reaches the oldest loaded edge, the transcript shall load older history without a separate button action.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6:** After an older page commits, the transcript shall continue loading while the oldest-page sentinel remains in the preload region, whether the page extends a collapsed activity group or adds a standalone row.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.7:** Automatic older-history loading shall stop when the sentinel leaves the preload region, prompt `#1` is loaded, persisted history is exhausted, or a request fails to make progress.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.8:** When an older-page request fails or makes no progress while older history remains, the transcript shall show a retry control. Routine successful pagination shall not show this control.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9:** The transcript shall keep the reader's current visual position stable while older content is inserted.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10:** Opening a task shall request only the bounded newest window until the user navigates upward.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.11:** Desktop and mobile transcripts shall provide the same history boundary, continuous upward loading, recovery, and scroll-position behavior.
- **AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.12:** When a previously opened session is revisited after more than one message page was persisted while it was inactive, the transcript shall reconcile to a contiguous newest window and upward pagination shall reach every persisted user prompt without gaps or duplicate rows.

## Exclusions

- Backfill or repair of missing persisted message records.
- Changes to backend message ordering or cursor contracts.
- Loading the complete transcript before the user navigates upward.
- Rendering internal rows that precede stored prompt `#1`.
