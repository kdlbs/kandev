---
status: active
system: executors
created: 2026-10-10
owners:
  - kandev
---

# Executor deletion catalogue continuity

## Overview

Deleting an executor shall remove that owner's choices while retaining other
available executor profiles. The executor system owns the catalogue and owner
lifecycle, including the choices consumed by desktop and phone task creation.
This is distinct from the profile-editing and profile-card-refresh lifecycles
in the [profile editor requirements](profile-editor.md).

## Terminology

- **Owner:** The executor containing a set of executor profiles.
- **Current catalogue:** The executor and profile inventory available when an
  accepted deletion is applied, including live changes received while deleting.

## Requirements

### REQ-EXECUTORS-OWNER-DELETION-001: Preserve surviving catalogue choices

**Intent:** An accepted owner deletion shall not undo unrelated live changes.

#### Acceptance criteria

- **AC-EXECUTORS-OWNER-DELETION-001.1:** When deletion of an eligible non-system
  executor succeeds, only that executor and its nested profiles shall be removed
  from the current catalogue. Every other current executor shall retain its
  membership, order, name, type, status, configuration, provider metadata,
  timestamps and profile values and membership. This includes additions,
  updates and removals received during deletion. Previously removed owners or
  profiles shall remain absent, including in a mixed catalogue containing both
  retained and removed entries.
- **AC-EXECUTORS-OWNER-DELETION-001.2:** After an accepted owner deletion and the
  existing return to the executor settings hub, desktop and phone task creation
  and subtask choices shall retain every eligible surviving profile with its
  current name and owner metadata. Newly received eligible choices shall remain
  available; profiles belonging to the deleted owner and unrelated choices
  removed during deletion shall remain absent. Existing provider eligibility
  and selection policies shall retain their meaning.
- **AC-EXECUTORS-OWNER-DELETION-001.3:** Owner deletion shall retain the exact
  lowercase `delete` confirmation, system-executor exclusion, current transport
  request semantics, and existing successful navigation, pending-state and
  dialog behavior. Mismatched confirmation shall dispatch no deletion. A rejected
  deletion shall not publish a successful local catalogue removal or successful
  navigation; its existing rejection propagation and final dialog/pending-state
  cleanup shall retain their behavior.

## Out of scope

- Backend deletion semantics, authorization, running-resource cleanup, storage,
  schemas, or transport message changes.
- New error handling, notifications, retries, or deletion interaction redesign.
- Profile mutation, connection or policy saves, catalogue revision frameworks,
  provider registry changes, selection defaults, or task launch policy.
- Server ordering of deletion versus recreation of the same executor identity,
  request cancellation on unmount, or repairing previously corrupted catalogues.

## Implementation plan

- [Preserve the current catalogue during owner deletion](../../../plans/executor-delete-catalogue-preservation/plan.md)
