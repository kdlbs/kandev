---
status: draft
system: tasks
created: 2026-09-27
owners:
  - kandev
---

# Bulk Session Removal Requirements

## Overview

Task sessions are persisted conversations. Users need a task-scoped way to
permanently remove all sessions or every session except the selected one,
without confusing that operation with hiding Dockview panels. The existing
single-session deletion contract, including retained task workspaces, remains
the authority for each removed conversation.

## Terms

- **Selected session:** The persisted task session represented by the tab or
  phone-picker row from which the removal action was opened.
- **Bulk target set:** The persisted sessions of that selected session's task
  that the requested scope includes. It includes hidden and other-group
  sessions, and excludes panels that are not task sessions.

## Requirements

### REQ-TASKS-BULK-SESSION-REMOVAL-001: Permanently remove task sessions in bulk

**Intent:** As a task user, I want to permanently remove selected groups of
session conversations so that the task no longer retains conversations I do
not need.

#### Acceptance criteria

- **AC-TASKS-BULK-SESSION-REMOVAL-001.1:** The desktop session-tab menu shall
  show **Close Others**, a separator, **Remove Others**, and **Remove All** in
  that order. Close Others shall continue to hide only sibling Dockview panels.
- **AC-TASKS-BULK-SESSION-REMOVAL-001.2:** Remove Others shall target every
  persisted session of the selected session's task except the selected
  session. Remove All shall target every persisted session of that task,
  including the selected session. Neither action shall target another task or
  a non-session panel.
- **AC-TASKS-BULK-SESSION-REMOVAL-001.3:** A bulk action shall not begin when
  its target set is empty, the task session list is loading, or any target is
  `RUNNING` or `STARTING`; the control shall expose the reason and require the
  user to stop active sessions first. The client shall take a fresh eligible
  snapshot before confirmation and shall require a new confirmation when a
  state change invalidates the pending snapshot.
- **AC-TASKS-BULK-SESSION-REMOVAL-001.4:** Before deletion, the user shall see
  a destructive confirmation that names the action and exact singular or
  plural target count, states that conversation histories are permanently
  deleted, and states that the task workspace and files remain. Cancellation,
  Escape, or dismissal shall send no deletion request.
- **AC-TASKS-BULK-SESSION-REMOVAL-001.5:** On confirmation, the system shall
  delete each target through the existing session-deletion contract. Remove
  Others shall retain the selected session and its active selection. Remove All
  shall leave the task with zero sessions and an operable empty-session state.
- **AC-TASKS-BULK-SESSION-REMOVAL-001.6:** The system shall prevent duplicate
  bulk submissions. While removal is pending, it shall report progress and
  block another submission. On the first failed or refused deletion it shall
  stop, retain every undeleted session, report completed and remaining counts,
  and require a fresh confirmation to retry.

### REQ-TASKS-BULK-SESSION-REMOVAL-002: Reachable and localized bulk removal

**Intent:** As a phone user, I need the same safe permanent-removal outcome as
desktop users without relying on a desktop-only control.

#### Acceptance criteria

- **AC-TASKS-BULK-SESSION-REMOVAL-002.1:** The phone Sessions picker shall
  expose Remove Others and Remove All through visible touch controls and a
  hosted confirmation step in its existing sheet.
- **AC-TASKS-BULK-SESSION-REMOVAL-002.2:** Phone controls shall provide
  accessible names, focus and dismissal behavior, touch targets of at least
  44 CSS pixels, safe-area clearance, and no horizontal document overflow.
- **AC-TASKS-BULK-SESSION-REMOVAL-002.3:** All new labels, singular and plural
  counts, destructive warnings, eligibility explanations, progress, and
  partial-failure feedback shall be localized in every shipped locale.

## Exclusions

- Automatically stopping or force-deleting active sessions.
- Task, workspace, worktree, branch, or Quick Chat task deletion.
- A bulk backend API, transactional rollback, or changes to single-session
  deletion.
- Kanban preview-tab menu changes.
