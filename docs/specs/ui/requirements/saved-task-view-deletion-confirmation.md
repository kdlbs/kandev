---
status: active
system: ui
created: 2026-09-06
owners:
  - kandev
---

# Saved Task View Deletion Confirmation Requirements

## Overview

Saved task views and saved integration queries are reusable user preferences,
but their current delete actions remove them immediately. A mistaken click can
therefore discard a useful filter definition without giving the user a chance
to verify the target.

The UI system owns this independent, reusable confirmation contract. Each task
surface continues to own its saved-view collection, persistence, active-view
fallback, and failure recovery.

## Terminology

- **Saved task view:** A named Kandev-owned task filter, view, or integration
  query that a user can select again.
- **Owning surface:** The task sidebar, Threads, Jira, GitHub, GitLab, or Azure
  DevOps UI that owns a saved task view and its delete action.
- **Fine-pointer confirmation:** A compact confirmation anchored to the delete
  action that opened it.
- **Coarse-pointer confirmation:** An in-place confirmation inside the current
  drawer, sheet, menu, or editor.

## Requirements

### REQ-UI-SAVED-TASK-VIEW-DELETION-001: Confirm saved task view deletion

**Intent:** Prevent accidental deletion of reusable task filters while keeping
each surface's existing delete behavior intact after explicit confirmation.

**User story:** As a user with saved task views, I want to verify the named view
before deleting it, so that an accidental action does not discard my filters.

#### Acceptance criteria

- **AC-UI-SAVED-TASK-VIEW-DELETION-001.1:** When a user invokes any in-tree
  delete action for a task-sidebar view, Threads view, Jira saved view, or
  GitHub, GitLab, or Azure DevOps saved query, the UI shall show confirmation
  before invoking the owning surface's delete operation.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.2:** The confirmation shall identify the
  saved task view by its current visible name and explain that only its saved
  filters and view settings are removed; tasks and connected-service data
  remain unchanged.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.3:** Cancel, Escape, dismissal of the
  owning surface, or disappearance of the confirmation target shall preserve
  the saved task view and shall not invoke its delete operation. When the
  initiating surface remains open, focus shall return to the delete action.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.4:** Explicit confirmation shall use the
  semantic destructive action treatment and invoke the existing delete
  operation exactly once. Existing active-view fallback, persistence, sync
  error, and optimistic rollback behavior shall remain unchanged.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.5:** A delete action that is unavailable
  because a view is built in, is the protected last view, or is temporarily
  disabled by an existing mutation shall remain unavailable and shall not open
  confirmation.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.6:** With a fine pointer, confirmation
  shall be anchored to the initiating delete action, stay within the viewport,
  and keep its owning menu, popover, or editor available until the user decides.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.7:** With a coarse pointer, confirmation
  shall replace the relevant row or action region inside the existing surface.
  It shall not stack another drawer, sheet, or modal, and its actions shall be
  visible without hover and at least 44 by 44 CSS pixels.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.8:** The delete trigger and confirmation
  shall be keyboard and screen-reader operable. The target name shall be
  programmatically available, Cancel shall receive initial confirmation focus,
  and using delete controls shall not also select the saved view or toggle an
  adjacent default marker.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.9:** At phone widths and with long saved
  task view names or translated copy, the owning surface shall retain one
  vertical scroll owner, keep actions reachable above the safe area, and create
  no document-level horizontal overflow.
- **AC-UI-SAVED-TASK-VIEW-DELETION-001.10:** Confirmation copy and accessible
  names shall use the active locale in every supported Kandev locale.

## Out of scope

- Changing saved-view models, limits, active-view selection, persistence,
  optimistic updates, rollback, or provider APIs.
- Deleting tasks, task data, provider records, or connected-service data.
- Confirming deletion of saved layouts, action presets, review watches,
  run-status filters, or other saved artifacts that are not task views.
- Adding confirmation to bespoke UI implemented only in an external plugin
  repository. Plugins that render Kandev's shared host controls inherit the
  host confirmation behavior.
- Adding a preference that bypasses or changes this confirmation.
