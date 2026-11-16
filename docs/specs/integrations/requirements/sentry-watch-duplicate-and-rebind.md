---
status: draft
system: integrations
created: 2026-10-24
owners:
  - kandev
---

# Sentry Watch Duplicate and Instance Rebinding Requirements

## Overview

Integrations owns Sentry issue-watch configuration. One application often
runs in several regions, each reporting to its own dedicated Sentry instance
with the same organization, project, and environment names. Users need to copy
a watcher configured for one instance and point the copy at another, and to
move an existing watcher to a different instance, without re-entering the
fields that do not depend on the instance.

## Terminology

- **Instance:** A named Sentry connection configured in the watcher's
  workspace.
- **Instance-scoped fields:** The watcher's organization and project
  selections. Their options are loaded from one instance.
- **Instance-independent fields:** Every other watcher field: environment,
  levels, statuses, lookback period, query, workflow and step, repository and
  base branch, agent and executor profiles, prompt, poll interval, in-flight
  cap, and enabled state.
- **Import history:** The record of the Sentry issues a watcher has already
  turned into tasks. It prevents the same issue from creating a second task.
- **Legacy unbound watcher:** A watcher migrated without an instance binding.
  It polls the instance its workspace resolves to at poll time.

## Requirements

### REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001: Duplicate a watcher

**Intent:** Start a new watcher from an existing watcher's configuration.

**User story:** As a user watching one regional Sentry instance, I want to
duplicate that watcher, so that I can point the copy at another region's
instance without retyping its configuration.

#### Acceptance criteria

- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.1:** Each Sentry watcher
  row, in the desktop table and in the phone list, shall offer a Duplicate
  action placed between the Reset and Delete actions, with a localized
  accessible name and tooltip.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.2:** Activating Duplicate
  shall open the watcher dialog in create mode, titled as a duplicate, with
  every field of the source watcher prefilled, including its instance,
  instance-scoped fields, and the Enabled value displayed in the source row
  (including an unsaved Enabled toggle). Duplicating shall not save or discard
  that source-row draft. The instance shall be selectable.
  An empty stored prompt shall remain empty in the duplicate draft, rather
  than being replaced with the fresh-create default. Existing readiness rules
  may require the user to fill it before Create.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.3:** For a valid draft,
  Create shall persist a new watcher with its own identity, no import history,
  and no poll yet. The source watcher shall remain unchanged. Cancel shall
  persist nothing.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.4:** Duplicating a legacy
  unbound watcher shall require an instance before Create, and shall keep the
  existing automatic selection of a workspace's sole instance.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.5:** If a copied legacy
  filter is rejected by current create validation, Create shall show the
  existing localized error toast and keep the duplicate draft open for
  correction. It shall not update the source or silently normalize the filter.

### REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002: Instance change keeps valid fields

**Intent:** Changing the instance in the watcher dialog refreshes only what the
new instance invalidates.

**User story:** As a user switching a watcher's instance, I want my
organization, projects, and other settings kept where they still apply, so
that a region switch needs no re-entry.

#### Acceptance criteria

- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.1:** When the user
  changes the instance in the create, duplicate, or edit dialog, the dialog
  shall load the organization and project options of the newly selected
  instance.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.2:** The selected
  organization shall be kept when the new instance offers it; otherwise the
  organization and projects shall be cleared. Each selected project shall be
  kept only when the new instance offers it under the kept organization.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.3:** Every
  instance-independent field shall keep its current value across an instance
  change.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.4:** Save shall stay
  unavailable until the carried-over organization and project selections have
  been checked against the new instance. When an option list cannot be loaded,
  the selection it would verify shall be cleared. While verification is pending,
  carried selections shall not be interactive options that can bypass that
  verification by being toggled.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.5:** In create and
  duplicate mode, if reconciliation clears an unavailable organization and the
  new instance offers exactly one organization, that organization shall be
  selected automatically with no carried projects. Edit mode shall leave the
  organization empty for explicit selection.

### REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003: Change an existing watcher's instance

**Intent:** Move an existing watcher to another instance of its workspace
without missing issues or duplicating tasks.

**User story:** As a user maintaining a watcher, I want to change its Sentry
instance in place, so that I keep its configuration and existing tasks.

#### Acceptance criteria

- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.1:** The edit dialog
  shall allow selecting any instance of the watcher's workspace. Its instance
  help text shall not state that the instance is immutable.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.2:** While the selected
  instance differs from the saved instance, the edit dialog shall show, before
  Save, a localized warning that the change clears the watcher's import
  history, keeps existing tasks, and imports matching issues from the new
  instance on the next poll. This warning shall also appear when explicitly
  binding a legacy unbound watcher.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.3:** Saving a different
  instance shall persist the new binding and, in the same atomic write, clear
  the import history, the last-polled time, and the last recorded error. Tasks
  created before the change shall remain, keep their original instance
  reference, and continue to count toward the in-flight cap.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.4:** After an instance
  change, an issue from the new instance shall create a task even when its
  short ID equals one imported from the previous instance. An issue observed
  from the previous instance after the change shall create neither a task nor
  an import-history entry, and shall not delete or alter the new instance's
  history through a delayed release or task attachment.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.5:** An update that omits
  the instance, or repeats the saved instance, shall leave the binding and the
  import history unchanged. An update naming an instance outside the watcher's
  workspace shall fail with `SENTRY_INSTANCE_NOT_FOUND` (HTTP 404), and an
  update naming an empty instance shall fail with
  `SENTRY_INSTANCE_REQUIRED` (HTTP 400). A failed update shall change nothing.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.6:** Binding a legacy
  unbound watcher to any explicit instance shall always clear its import
  history and poll/error state under
  `AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.3`, even if that instance
  is the one it currently resolves to.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.7:** Reset shall delete
  only tasks still recorded in the watcher's current import history, including
  archived tasks in that history. Tasks outside that history shall survive even
  when they are descendants of a recorded task; surviving direct children of
  a deleted task shall be detached to top-level tasks, not deleted. Recorded
  tasks shall still be deleted when they are children of another recorded task.
  The confirmation shall state that tasks imported before an instance change
  are kept, including when preview fails.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.8:** If the saved binding
  changes after an update has read it, including changing away and back to the
  same instance, the update shall fail with HTTP 409 (`SENTRY_WATCH_CONFLICT`)
  without clearing history or overwriting settings. The dialog shall retain
  its draft and show the existing error toast.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.9:** While a live
  dispatch owns task creation for the watcher, rebinding, Reset, and deletion
  shall fail with HTTP 409 (`SENTRY_WATCH_CONFLICT`) without destructive side
  effects. Once that dispatch finishes, the user may retry. No old-instance
  task shall be created after a successful rebind commits.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.10:** Reset shall
  exclusively own the watcher's import scope from before listing task IDs
  through task deletion and final history clearing. Rebinding, another Reset,
  deletion, and new task dispatch shall not overlap that operation. A failed
  admission shall perform no task deletion. If rebind wins admission first,
  Reset shall operate only on the new scope, keeping prior-instance tasks.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.11:** An empty history
  reservation alone shall not be treated as evidence of running task creation.
  After live dispatch ownership has ended, Reset may safely clear abandoned
  reservations, and rebind may discard them with the old history. No elapsed-
  time heuristic or user timing instruction shall authorize clearing a live
  dispatch's reservation.
- **AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.12:** Every edit-dialog
  update shall be conditional on the import scope captured when that dialog
  opened, including edits that do not change the instance. If rebind or Reset
  advances that scope before Save, the update shall fail with HTTP 409
  (`SENTRY_WATCH_CONFLICT`), retain the draft, and leave settings/history/tasks
  unchanged, even when the instance changed away and back or changed before
  the server reads the request. Passive same-scope hydration shall not reset
  the draft or replace its opening precondition. Closing and reopening takes
  a new baseline; a conflict shall never silently retry against the new scope.

## Out of scope

- Duplicating a watcher into another workspace, or server-side clone APIs.
- Copying import history or tasks to a duplicate.
- Retaining per-instance import history so that switching back to an earlier
  instance skips its previously imported issues.
- Unbinding an instance from a watcher.
- Moving existing tasks between watchers or instances.
- Duplicate actions for other integrations' watchers.
