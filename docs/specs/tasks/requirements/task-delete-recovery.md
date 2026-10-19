---
status: active
system: tasks
created: 2026-10-16
updated: 2026-10-16
owners:
  - kandev
---

# Task deletion with retained workspace files

## Overview

A user must be able to remove a task when its workspace is damaged or
unavailable. The recovery action deletes the task and preserves its workspace
resources. It does not need a successful workspace inspection.

This capability belongs to tasks. It extends
[runtime cleanup](runtime-cleanup.md) with an explicit retention outcome.
Its separate document keeps this recovery contract within the specification
size limit. Normal deletion remains governed by the existing cleanup contract.

## Terms

- **Retain mode:** Delete the selected task records while keeping workspace
  files, Git registrations, branches, and file-bearing executor resources.
- **Stop pending:** Task deletion committed, but agent shutdown is not confirmed.
  Kandev retains the shutdown obligation for retry.

## Requirements

### REQ-TASKS-DELETE-RECOVERY-001: Delete a task without workspace inspection

**Intent:** Remove unusable tasks without requiring manual filesystem repair
or permitting unsafe file deletion.

#### Acceptance criteria

- **AC-TASKS-DELETE-RECOVERY-001.1:** When a user selects retain mode, task
  deletion shall not depend on reading workspace files, Git metadata, ownership
  markers, or a remote executor. Missing, corrupt, inaccessible, and unavailable
  workspaces shall not block that action.
- **AC-TASKS-DELETE-RECOVERY-001.2:** Retain mode shall preserve the workspace
  resources for every selected task, including clean resources in a mixed
  selection. It shall not authorize later automatic removal of those resources.
- **AC-TASKS-DELETE-RECOVERY-001.3:** Before deleting task records, Kandev shall
  durably retain the available resource inventory, retention decision, and
  runtime shutdown obligations. These records shall survive task deletion,
  workspace deletion, process restart, and completion of agent shutdown.
- **AC-TASKS-DELETE-RECOVERY-001.4:** Retain mode shall preserve authorization,
  confirmation, managed-task admission, descendant selection, and shared-resource
  ownership rules. Database or authorization failures shall report failure
  without claiming that the task was deleted.
- **AC-TASKS-DELETE-RECOVERY-001.5:** When agent shutdown fails after task
  deletion, Kandev shall report deletion as committed with shutdown pending.
  It shall retain the exact shutdown handles and retry obligation. A retry shall
  not restore the task or remove retained workspace resources.
- **AC-TASKS-DELETE-RECOVERY-001.6:** Single, bulk, and cascade delete dialogs
  shall offer an unchecked keep-files choice during normal inspection, loading,
  and inspection failure. Selecting it shall obtain a separate confirmation for
  retain mode and show the action "Delete task and keep files". Changing the
  selection, mode, or descendant scope shall invalidate the previous confirmation.
- **AC-TASKS-DELETE-RECOVERY-001.7:** Desktop and phone users shall complete
  the same recovery action from the existing delete entry points. The phone
  dialog shall retain its centered inset surface, internal scrolling, and
  reachable actions with touch targets of at least 44 CSS px. Keyboard users
  shall receive visible focus and announced loading, error, and outcome states.
- **AC-TASKS-DELETE-RECOVERY-001.8:** HTTP and MCP task deletion shall support
  explicit retain mode. Omitting the mode shall preserve existing deletion and
  discard-consent behavior. Responses shall distinguish retained workspace
  resources from pending shutdown and failed task deletion, without exposing
  another user's resource details.

## Exclusions

This change does not repair damaged Git data, explain a host reboot, or prove
SSD failure. It does not bypass permissions or unavailable database storage.
It does not change archive, session deletion, or discard consent for normal
deletion. A new resource browser and an action to release retained resources
are outside this change. Retained resources can use disk space indefinitely.
