---
status: active
system: tasks
created: 2026-10-07
owners:
  - kandev
---

# Workflow start selection requirements

## Overview

Workflow authors can rename or edit a step without changing the selected start
step. The task system owns this contract because it persists workflow definitions
and resolves the stage for later task creation. These criteria record the
accepted correction; delivery remains pending.

## Terminology

- **Ordinary update:** An existing step edit that can omit `is_start_step`.
- **Explicit selection:** An update supplying `is_start_step: true` or `false`.
- **Own committed selection:** The flag installed or preserved by the successful
  edit, before a subsequent caller changes selection again.

## Requirements

### REQ-TASKS-WORKFLOW-START-SELECTION-001: Preserve selection during ordinary edits

**Intent:** Independent step edits must not silently redirect later tasks.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-START-SELECTION-001.1:** When an ordinary edit omits
  `is_start_step`, it shall preserve the edited step's saved flag and shall not
  demote any other step. This holds when another caller successfully changes
  selection after the edit observes the step but before the edit persists,
  both when the edited step becomes selected and when it becomes unselected.
- **AC-TASKS-WORKFLOW-START-SELECTION-001.2:** Explicit `true` shall select the
  edited step and clear the previous selected step in the same workflow.
  Explicit `false` shall clear only the edited step's flag. An explicit edit
  applied after an earlier selection shall retain its explicit intent.
- **AC-TASKS-WORKFLOW-START-SELECTION-001.3:** A successful ordinary edit's
  response and emitted step-update events shall carry its own committed flag.
  Omission shall produce no demotion result or demotion event; explicit
  promotion shall report and emit updates for actual demotions.
- **AC-TASKS-WORKFLOW-START-SELECTION-001.4:** A failed edit shall leave saved
  names, flags, and modification timestamps unchanged, including any proposed
  demotions, and shall emit no successful step-update event. Foreign, missing,
  and read-only resources shall retain their existing rejection behavior.
- **AC-TASKS-WORKFLOW-START-SELECTION-001.5:** After a successful omission edit,
  ordinary task creation without an explicit stage or immediate agent start
  shall continue to use the selected start step. With no selected flag, the
  existing first-step-by-position fallback shall remain in effect.
- **AC-TASKS-WORKFLOW-START-SELECTION-001.6:** Direct full-configuration writes
  shall retain their explicit replacement behavior. Exact Host commands shall
  retain their workflow and step version checks, including rejection and
  rollback of stale edits. No new field or request version shall be required
  for ordinary updates.

## Related contracts

- [Workflow step ordering](workflow-step-ordering.md) owns positional edits.
- [Workflow step agent-start ownership](workflow-step-agent-start-ownership.md)
  owns the separate immediate-agent-start routing rule.
- [Settings manual save](../../ui/requirements/settings-manual-save.md) owns
  settings draft and Save interactions.

## Out of scope

- Omission preservation for other step fields or atomic settings Save.
- Global writer admission, universal patch semantics, event ordering across
  requests, new schemas, routes, or conflict-resolution UI.
- Changes to creation, portable import/export, workflow sync, deletion cleanup,
  step ordering, or agent-start destination selection.

## Implementation plans

- [Preserve workflow start selection](../../../plans/workflow-start-selection/plan.md)
