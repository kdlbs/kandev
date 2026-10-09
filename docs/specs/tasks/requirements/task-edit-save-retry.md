---
status: active
system: tasks
created: 2026-10-08
owners:
  - kandev
---

# Task edit save retry requirements

## Overview

Users shall be able to retry a failed save of an existing task without retyping
their edits. Tasks owns this contract because it owns task metadata and the
meaning of a confirmed save. The shared editor exposes that outcome from task
cards, task actions, and the sidebar, including phone entry points.

The active [sidebar editing contract](sidebar-task-edit.md) defines entry and
initialization behavior. [Dependency editing](task-dependency-detail-editing.md)
and [runner-switch effects](runner-switch-before-materialization-effects.md)
define independent save stages. This requirement fills the missing ordinary
task-field save failure outcome; it does not replace those contracts.

## Terminology

- **Current edit:** The same task's open editor, without an explicit dismissal,
  task change, navigation, or unmount.
- **Draft:** The user's current field values, including whitespace and empty
  editable instructions. Submission may apply existing normalization.
- **Confirmed fields:** Values acknowledged by a successful save stage. Keeping
  a draft available does not establish that it was saved.

## Requirements

### REQ-TASKS-EDIT-SAVE-RETRY-001: Retry a failed current task edit

**Intent:** Preserve the user's editing work when a save fails, while keeping
confirmed task data and partial outcomes truthful.

#### Acceptance criteria

- **AC-TASKS-EDIT-SAVE-RETRY-001.1:** When an existing task's field save fails
  before field-save acknowledgement, the current editor shall remain open,
  show its existing failure feedback, and retain the exact editable title and
  instructions draft. This applies to an ordinary server rejection or transport
  failure, without requiring a recognized error category.
- **AC-TASKS-EDIT-SAVE-RETRY-001.2:** After that failure, the save action shall
  leave its busy state and permit a valid retry. The retry shall submit the
  current draft with the existing normalization and admission rules. A successful
  retry shall use the normal success outcome and close the editor.
- **AC-TASKS-EDIT-SAVE-RETRY-001.3:** Until a successful acknowledgement, task
  surfaces shall retain confirmed values and shall not present the failed draft
  as saved or publish a save-success outcome.
- **AC-TASKS-EDIT-SAVE-RETRY-001.4:** Both the ordinary edit-save action and
  update-without-agent action shall provide the same failure/retry outcome.
  Started tasks shall retain their existing instructions and repository locks.
- **AC-TASKS-EDIT-SAVE-RETRY-001.5:** Recognized branch-policy failures shall
  continue to permit correction and retry. Existing runner-switch, repository,
  dependency-save, and save-then-launch failure outcomes shall retain their
  confirmed-stage meaning, feedback, and admission rules. A stage already
  committed shall not be represented as rolled back.
- **AC-TASKS-EDIT-SAVE-RETRY-001.6:** The shared editor shall provide these
  outcomes on desktop and phone entry points, without requiring navigation or
  reopening to retry. Explicit cancel/dismiss shall keep its current behavior.

## Out of scope

- Draft retention across explicit dismissal, task changes, navigation, unmount,
  reload, or restart; global or persistent edit drafts.
- Changes to editable fields, validation, permissions, runner eligibility,
  dependency admission, repository policy, or fresh-branch consent.
- Task creation, new-session creation, Office inline editing, and lightweight
  Rename surfaces.
- Backend/API changes, new transactions, rollback guarantees for multi-stage
  saves, or assertions that an ambiguous transport failure proves no server write.

## Implementation plans

- [Task edit save retry](../../../plans/task-edit-save-retry/plan.md).
