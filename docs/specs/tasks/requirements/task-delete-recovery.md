---
status: active
system: tasks
created: 2026-10-16
updated: 2026-10-19
owners:
  - kandev
---

# Task deletion with unavailable workspaces

## Overview

A user must be able to remove a task when its workspace is damaged or
unavailable. Normal deletion can commit with explicitly authorized cleanup
still pending. The independent keep-files choice preserves workspace resources
without authorizing later automatic removal.

This capability belongs to tasks. It extends
[runtime cleanup](runtime-cleanup.md) with an explicit retention outcome.
Its separate document keeps these recovery contracts within the specification
size limit. Physical removal remains subject to the existing ownership checks.

## Terms

- **Retain mode:** Delete the selected task records while keeping workspace
  files, Git registrations, branches, and file-bearing executor resources.
- **Cleanup pending:** Task deletion committed, but some requested resource
  removal has not completed. Cleanup remains authorized and durably tracked.
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
  explicit retain mode. Omitting the mode shall select normal deletion. Without
  explicit consent to proceed with unknown workspace changes, existing admission
  behavior shall remain. Responses shall distinguish retained workspace
  resources from pending shutdown and failed task deletion, without exposing
  another user's resource details.

### REQ-TASKS-DELETE-RECOVERY-002: Normal deletion with incomplete inspection

**Intent:** Delete an unusable task without requiring permanent file retention,
while continuing to enforce safe resource removal.

#### Acceptance criteria

- **AC-TASKS-DELETE-RECOVERY-002.1:** When workspace inspection cannot complete,
  preflight shall distinguish unknown workspace changes from a clean result.
  Authorized users shall be able to confirm normal deletion with that uncertainty.
  Authorization, database inventory, and request-validation failures shall not
  become successful previews.
- **AC-TASKS-DELETE-RECOVERY-002.2:** When the user explicitly accepts unknown
  workspace changes and discard of local changes, a physical marker, Git,
  filesystem-access, or provider-availability failure shall not prevent deletion
  of the task records after their cleanup obligations are durably saved.
  Without that consent, an unknown workspace shall remain blocked.
- **AC-TASKS-DELETE-RECOVERY-002.3:** Deletion shall preserve all database-known
  cleanup targets, prior unfinished obligations, consent, and failure reasons
  before removing live owner records. Failed authorization or persistence shall
  leave the uncommitted task intact. Partial cascades shall identify committed
  members and preserve their cleanup obligations.
- **AC-TASKS-DELETE-RECOVERY-002.4:** Consent to unknown changes shall not
  override path, ownership, shared-resource, retained-resource, registration,
  or branch-identity safeguards. A failed inspection shall never prove absence.
  Resources lacking safe removal evidence shall remain protected from unrelated
  cleanup after task deletion and restart.
- **AC-TASKS-DELETE-RECOVERY-002.5:** For a mixed selection, each independently
  eligible resource shall progress through cleanup. One damaged resource shall
  not prevent cleanup of another resource whose shutdown and ownership checks
  pass. Shared and borrowed resources shall remain protected.
- **AC-TASKS-DELETE-RECOVERY-002.6:** Transient cleanup failures shall retain
  scheduled retries across restart. Retry exhaustion shall retain the operation,
  diagnostic, and resource protection. Neither outcome shall recreate the deleted
  task or silently turn requested cleanup into permanent retain mode.
- **AC-TASKS-DELETE-RECOVERY-002.7:** Single, bulk, and cascade dialogs on desktop
  and phone shall offer an unchecked acknowledgement of unknown workspace
  changes. Confirmation shall bind that acknowledgement, discard consent,
  cleanup mode, actor, and task scope. Changing any bound choice shall invalidate
  the previous confirmation. Phone actions shall retain reachable 44 CSS px
  touch targets, contained scrolling, and accessible status announcements.
- **AC-TASKS-DELETE-RECOVERY-002.8:** HTTP and MCP shall expose the same explicit
  acknowledgement and committed outcome. A response shall distinguish deleted
  tasks, pending workspace cleanup, and pending runtime shutdown. Post-commit
  failures shall not report the task as undeleted. Partial bulk results shall
  remain attributable to their selected roots.
- **AC-TASKS-DELETE-RECOVERY-002.9:** Retain mode shall continue to preserve all
  selected resources and avoid physical inspection. A caller shall not combine
  retain mode with consent to remove unverified workspace changes. Clean and
  dirty normal deletion without inspection failures shall keep their existing
  confirmation behavior.

## Exclusions

This change does not repair damaged Git data, explain a host reboot, or prove
SSD failure. It does not bypass permissions or unavailable database storage.
It does not change archive or session deletion. Consent cannot authorize
arbitrary paths or remove another task's resources. A new resource browser and
an action to release retained resources
are outside this change. Retained resources can use disk space indefinitely.
