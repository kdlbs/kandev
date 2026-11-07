---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---

# Workflow delete removes steps requirements

## Overview

Deleting a workflow removed the workflow row and its session bindings but left
every `workflow_steps` row that belonged to it. The repository relied on an
`ON DELETE CASCADE` foreign key that is not present in real databases, so the
steps stayed behind with a `workflow_id` that no longer exists. Workspace
deletion already deletes steps explicitly and is not affected.

Tasks owns this contract because the task SQLite repository owns the workflow
delete and creates the `workflow_steps` table that real databases use.

## Terminology

- **Orphan step:** A `workflow_steps` row whose `workflow_id` names no row in
  `workflows`.

## Requirements

### REQ-TASKS-WORKFLOW-DELETE-STEPS-001: Deleting a workflow removes its steps

**Intent:** A deleted workflow leaves no steps behind, and steps left by earlier
deletes are removed once on upgrade.

**User story:** As an operator, I want deleting a workflow to remove its steps,
so that stale step rows do not accumulate in the database.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-DELETE-STEPS-001.1:** When a workflow is deleted, the
  system shall delete every `workflow_steps` row with that `workflow_id` in the
  same transaction as the workflow row. Steps of other workflows shall remain.
  This shall hold whether or not the `workflow_steps` table has a foreign key,
  on SQLite and PostgreSQL.
- **AC-TASKS-WORKFLOW-DELETE-STEPS-001.2:** When a workflow delete fails because
  the workflow does not exist, the system shall delete no step rows.
- **AC-TASKS-WORKFLOW-DELETE-STEPS-001.3:** The workspace-scoped bulk workflow
  delete used by end-to-end cleanup shall delete the steps of exactly the
  workflows it deletes.
- **AC-TASKS-WORKFLOW-DELETE-STEPS-001.4:** On the first boot after upgrade, the
  system shall delete every orphan step once and record that it did so. Steps of
  existing workflows shall remain. Later boots shall not repeat the cleanup.

## Out of scope

- Changing the order in which the task and workflow repositories initialize, or
  adding the missing foreign key to existing databases.
- Rows in other tables that reference a deleted step, such as
  `workflow_step_participants`.
