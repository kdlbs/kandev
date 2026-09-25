---
status: active
system: tasks
created: 2026-09-25
owners:
  - kandev
---

# Git Push Failure Dismissal Requirements

## Overview

The task conversation stores Git operation failures as messages with diagnostic metadata and recovery actions. A user who has addressed a failed push manually needs to acknowledge that one message without deleting its diagnostics or treating dismissal as evidence that Git is repaired.

## Terminology

- **Dismissal:** A persisted acknowledgment of one Git push failure message. It hides only that historical message and its actions; it is not a resolved state or snooze, does not represent a successful push, and does not change Git state. Fix remains an agent-help action.

## Requirements

### REQ-TASKS-GIT-PUSH-ERROR-DISMISSAL-001: Dismiss one Git push failure message

**Intent:** Let users clear a stale Git push failure card after they have handled it manually while preserving its diagnostic record and keeping later failures visible.

#### Acceptance criteria

- **AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.1:** When a Git push failure message is visible, desktop and phone users shall have an explicit, keyboard-accessible Dismiss control on the same card as its diagnostics and Fix action.
- **AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.2:** When a user dismisses a Git push failure, the system shall persist the shared acknowledgment on that exact message, hide its card and Fix action, preserve its diagnostic content and metadata, and keep it hidden after reload, reconnect, and task switching. Dismissal shall not indicate that the push succeeded or that Git state was repaired.
- **AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.3:** When a different Git push failure message is created after an earlier one is dismissed, the newer failure shall remain visible with its own diagnostics and actions; dismissing one message shall not hide another message or any unrelated error.
- **AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.4:** When saving a dismissal fails, the system shall show the existing localized request-failure toast, clear the dismissal control's pending/disabled state, and leave the entire card plus both Fix and Dismiss controls visible and retryable. The card shall not be hidden before persistence succeeds; retrying after a failure shall be able to succeed.
- **AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.5:** Existing `type: error` messages with the current `git_operation_error: true` marker and `operation: push` shall remain readable and dismissible without a data migration or new persisted action entry; messages for other Git operations and generic errors shall retain their current behavior.
- **AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.6:** The Dismiss control shall have a localized accessible name in every supported locale and a touch target of at least 44 pixels on phone and coarse-pointer layouts.
- **AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.7:** When a caller cannot read the session associated with the stored message, including when that session belongs to another workspace, or when the message is sessionless, the dismissal request shall be denied without changing the message or publishing an update event.

## Out of scope

- Automatically reconciling a failure with Git status or inferring that a push succeeded.
- Polling, attempted-HEAD correlation, an alert projection or revision subsystem, a new database table, or a generic notification framework.
- Undo, confirmation dialogs, settings, or dismissal of non-push Git errors and unrelated errors.

## Implementation Plans

- [Git push error dismissal](../../../plans/git-push-error-dismissal/plan.md)

## System design

- [Git Push Failure Dismissal System Design](../system-design/git-push-error-dismissal.md)
