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
contract; Git operation execution and pending-state lifetime remain owned by
their existing systems.

## Terminology

- **File action slot:** The leading Changes-row position that shows either the
  file-type icon, the stage or unstage action, or its pending indicator.
- **Pending file action:** A stage or unstage request whose per-file pending
  state has started and has not yet cleared.

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

## Out of scope

- Changing Git stage or unstage execution, pending-state creation, or
  pending-state cleanup.
- Changing bulk stage, bulk unstage, discard, edit, or commit feedback.
- Adding user-facing copy, notifications, cancellation, or progress estimates.
- Changing Changes-panel navigation, grouping, row density, or scroll
  ownership.
