---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---

# Workflow visibility patch requirements

## Overview

Operators need to retire legacy workflows without losing their tasks, steps,
sessions, or history. Deleting a workflow is a hard delete that archives its
tasks, so it cannot serve as an archive. Workflows already carry a `hidden`
flag that management and picker surfaces honor, but the ordinary workflow
update request had no way to set it.

Tasks owns this contract because it owns persisted workflow definitions and
the ordinary workflow update API. The
[field update contract](task-field-updates.md) defines presence semantics for
the other ordinary workflow fields; this requirement adds `hidden` with the
same omission rules.

## Terminology

- **Archive (hide):** Set a workflow's `hidden` flag to true. The workflow row
  and everything that references it remain stored.
- **Built-in workflow:** A workflow row materialized from an embedded template
  by Kandev itself (`is_system = 1`). Startup reconciliation sets its `hidden`
  flag from the template.

## Requirements

### REQ-TASKS-WORKFLOW-VISIBILITY-PATCH-001: Archive and restore workflows through the ordinary update

**Intent:** Let an operator hide or unhide a user workflow through the existing
REST and WebSocket workflow update request, without deleting data.

**User story:** As an operator, I want to hide legacy workflows without deleting
them, so that pickers stay clean and the workflows can be restored later.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.1:** When an ordinary workflow update
  supplies `hidden: true` or `hidden: false`, the system shall persist that
  value and return it in the response. Tasks, steps, sessions, and history that
  reference the workflow shall remain unchanged, and no task shall be archived.
- **AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.2:** When an ordinary workflow update
  omits `hidden` or supplies JSON `null`, the system shall keep the stored value.
  Other supplied fields in the same request shall apply with their existing
  meanings.
- **AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.3:** A successful visibility change
  shall publish the existing `workflow.updated` event with the persisted
  `hidden` value, in the same shape as other workflow updates.
- **AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.4:** When an update supplies a
  `hidden` value that differs from the stored value of a built-in workflow,
  the system shall reject the whole request with a validation error (REST
  `400`, WebSocket `VALIDATION_ERROR`), change no field, and publish no event.
  Supplying the stored value is accepted as a no-op for that field. Existing
  read-only rules for synced and Improve Kandev workflows remain in force.

## Out of scope

- A management UI control for archiving workflows.
- Bulk update endpoints.
- Changing workflow deletion, task archive, or template visibility.
- Changing the startup reconciliation of built-in workflow visibility.
