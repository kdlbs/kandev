---
status: active
system: tasks
created: 2026-10-08
owners:
  - kandev
---

# Task comment draft preservation requirements

## Overview

Users can prepare their next task instructions while an earlier comment is
being sent. A successful earlier send must preserve that unsent work. Tasks
owns this contract because it governs authoring instructions on a task, rather
than reusable editor presentation or Office event publication.

This capability covers the editable first-party task comment composer. Plan
comments, session messages, and agent-message annotations have separate owners
and delivery contracts.

## Terminology

- **Submitted draft:** The exact text present when a comment send is admitted,
  including leading/trailing whitespace and line breaks.
- **Current draft:** The exact text currently present in that same mounted
  composer. Typing and inserted content both contribute to it.
- **Acknowledged send:** A comment request that completes successfully.

## Requirements

### REQ-TASKS-COMMENT-DRAFT-001: Preserve the next unsent task comment

**Intent:** Earlier comment acknowledgements shall clear only the text that
remains equal to the submitted draft, without destroying later instructions.

#### Acceptance criteria

- **AC-TASKS-COMMENT-DRAFT-001.1:** While a comment send is pending, the
  composer shall remain editable. If its current draft differs from the
  submitted draft when that send succeeds, the current draft shall remain
  exactly intact, including multiline content, whitespace-only changes,
  whitespace-only drafts, and deliberate deletion to an empty draft.
- **AC-TASKS-COMMENT-DRAFT-001.2:** A successful send shall clear an unchanged
  submitted draft. Editing away and then restoring the exact submitted draft
  before acknowledgement shall also remain eligible to clear. Equal trimmed
  content with different raw whitespace shall not be treated as unchanged.
- **AC-TASKS-COMMENT-DRAFT-001.3:** A failed response or rejected request shall
  retain the exact current draft, whether unchanged or edited after admission,
  and shall show the existing error feedback. Failure shall not restore the
  older submitted text over a newer draft, refresh comments as a success, or
  automatically retry.
- **AC-TASKS-COMMENT-DRAFT-001.4:** Each admitted send shall post its submitted
  text with outer whitespace trimmed and user attribution. Later edits shall
  not change that request. Acknowledged sends shall invoke the existing
  comment-refresh callback exactly once, including when the current draft is
  preserved; failed or inadmissible sends shall not invoke it.
- **AC-TASKS-COMMENT-DRAFT-001.5:** Existing send admission shall remain the
  same: empty or whitespace-only drafts shall not send; pending sends shall
  disable the send button and block another keyboard submission; plain Enter
  shall submit eligible text, and Shift+Enter shall remain available for line
  breaks. Keyboard and button sends shall obey the same preservation rule.
- **AC-TASKS-COMMENT-DRAFT-001.6:** After a send settles, the composer shall
  allow the next eligible deliberate send through the existing controls. A
  preserved next draft shall post its own trimmed content and clear on its own
  unchanged success. A failed send shall remain explicitly retryable without
  carrying the previous send's clearing decision into the retry.
- **AC-TASKS-COMMENT-DRAFT-001.7:** Independently mounted task composers shall
  own their drafts and pending sends independently. One composer's success or
  failure shall not clear, replace, refresh, or block another composer's draft.
- **AC-TASKS-COMMENT-DRAFT-001.8:** Desktop and phone shall receive the same
  draft preservation and admission behavior through their existing composer.
  The correction shall retain current composition, localized copy, touch
  controls, scrolling, and navigation.

## Out of scope

- Draft persistence, remount/restoration, session or task navigation ownership,
  unmounted feedback, and cross-tab state.
- Changes to comment transport, API, backend, events, optimistic timelines,
  authorization, or other chat inputs.
- File processing or utility prompt delivery redesign. Content inserted into
  the current draft is covered by the same equality rule; new attachment or
  prompt delivery guarantees are not introduced.
- A generic draft manager, framework, feature flag, or new UI feedback.

## System design and implementation

- [Task comment draft preservation design](../system-design/comment-draft-preservation.md)
- [Implementation plan](../../../plans/preserve-next-task-comment/plan.md)
