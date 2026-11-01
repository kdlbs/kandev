---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-VISIBILITY-PATCH-001
---

# Workflow visibility patch system design

## Purpose and boundaries

Tasks owns the workflow row and the ordinary update path, so the visibility
change rides on that path instead of a new endpoint. The design reuses the
field-scoped workflow write from the
[field update design](task-field-updates.md). Built-in workflow seeding and
its startup visibility reconciliation stay owned by the SQLite defaults code;
this design only refuses to fight them.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKFLOW-VISIBILITY-PATCH-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `task/handlers.httpUpdateWorkflow` and `wsUpdateWorkflow` decode the optional
  `hidden` pointer and map the built-in rejection to `400` and
  `VALIDATION_ERROR`.
- `task/service.Service.UpdateWorkflow` validates the built-in rule and passes
  `hidden` to the same field-scoped write as the other fields.
- `task/repository/sqlite` reads `is_system` into `models.Workflow.IsSystem`
  through the shared workflow column list. The field is internal and is not
  serialized.
- The web `workflow.updated` handler already applies `hidden` and moves the
  active workflow when it becomes hidden. No frontend change is required; the
  web PATCH payload type gains an optional `hidden`.

## Data and contracts

- REST `PATCH /api/v1/workflows/:id` and WebSocket `workflow.update` accept
  `hidden?: boolean`. Omission and `null` keep the stored value.
- `service.UpdateWorkflowRequest.Hidden *bool` maps to
  `models.WorkflowFieldUpdate.Hidden`, the column that `SetWorkflowHidden`
  writes. The version-fenced path copies it into the full-row update.
- `service.ErrSystemWorkflowVisibility` identifies the built-in rejection.
- Responses use the existing workflow DTO; events use the existing
  `workflow.updated` payload, which already carries `hidden`.

## Control flow

1. The transport rejects read-only workflows (synced or Improve Kandev) with
   the existing conflict.
2. The service authorizes the workflow and reads it.
3. If `hidden` is supplied, differs from the stored value, and the workflow is
   built-in, the service returns `ErrSystemWorkflowVisibility` before any write.
4. Otherwise one supplied-column `UPDATE ... RETURNING` writes every supplied
   field, including `hidden`, and the service publishes `workflow.updated` with
   the returned row.

## Failure and recovery

The built-in rejection happens before the write, so no field or timestamp
changes and no event is published. Startup reconciliation rewrites built-in
visibility from the template, so an accepted change would not survive a
restart; rejecting it keeps the API honest. Supplying the stored value passes
so that bulk scripts can send the same body to every workflow.

## Persistence

No schema change. `workflows.is_system` already exists. Rows written before
that column default to `0` and read as user workflows. Hiding does not touch
tasks, steps, sessions, or history rows.

## Security

The change uses the existing workflow authorization and executor-mutation
checks of the ordinary update. No new permission is introduced.

## Observability

The existing `workflow updated` log line and `workflow.updated` event cover
accepted changes. Rejections return the error message to the caller.

## Related decisions

- None.
