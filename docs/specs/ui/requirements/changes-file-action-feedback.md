---
status: active
system: ui
created: 2026-09-03
owners:
  - kandev
---

# Changes File Action Feedback Requirements

## Overview

Working-tree file rows expose stage and unstage actions in the Changes panel.
On fine-pointer layouts, the action shares the file-type icon slot and normally
appears only while the row is hovered. Once an action starts, its progress
feedback must remain visible until the associated request finishes, even when
the pointer leaves the row. The UI system owns this transient presentation
contract and the per-file pending lifetime that drives it; Git operation
execution and refreshed repository status remain owned by their existing
systems.

## Terminology

- **File action slot:** The leading Changes-row position that shows either the
  file-type icon, the stage or unstage action, or its pending indicator.
- **Pending file action:** The latest stage or unstage request within the active
  session, environment, and checked-out branch generation that owns the pending
  state for one repository and file and has not reached its target state or
  failed.

## Requirements

### REQ-UI-CHANGES-FILE-ACTION-FEEDBACK-001: Persistent pending file-action feedback

**Intent:** Let users identify which file operation is still running without
keeping the pointer over its row.

**User story:** As a user staging or unstaging a file, I want its progress
indicator to remain visible until the operation finishes, so that I know when
the action is complete.

#### Acceptance criteria

- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.1:** When a user starts a stage or
  unstage action for one file, that row shall replace its stage or unstage
  control with the existing animated pending indicator for the lifetime of the
  file's pending state.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.2:** On a fine-pointer layout, moving
  the pointer away from a row with a pending file action shall keep the pending
  indicator visible and shall not restore the file-type icon while the action
  remains pending.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.3:** Pending feedback shall remain
  scoped to the acted-on repository and file. Other file rows shall retain
  their current icons and actions.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.4:** When the pending state clears,
  the row shall return to its current idle presentation or move to the section
  that reflects the refreshed staged state.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.5:** Coarse-pointer Changes surfaces
  shall retain their always-visible, touch-sized file actions and show the same
  pending indicator without introducing a hover dependency or reducing the
  existing touch target.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.6:** A successful stage or unstage
  request shall remain pending through stale status updates and clear only when
  refreshed status reaches the requested staged or unstaged state.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.7:** When the current stage or
  unstage request fails, its pending feedback shall clear. If a newer request in
  either direction has superseded it for the same repository and file, the
  older request's completion or failure shall not clear the newer request's
  pending feedback.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.8:** When one request spans multiple
  repositories and only some repository scopes fail, failed scopes shall clear
  immediately while successful scopes remain pending until their refreshed
  status reaches the requested state.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.9:** Pending ownership shall reset
  when the active session, environment, or checked-out branch generation
  changes. A callback from the prior scope shall not add, retain, or clear
  feedback in the successor scope.

## Out of scope

- Changing Git stage or unstage execution, transport, or repository status
  refresh.
- Changing discard, edit, commit, or stage-all control presentation.
- Adding user-facing copy, notifications, cancellation, or progress estimates.
- Changing Changes-panel navigation, grouping, row density, or scroll
  ownership.
