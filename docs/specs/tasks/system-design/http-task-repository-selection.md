---
status: current
system: tasks
created: 2026-11-25
requirements:
  - REQ-TASKS-HTTP-REPOSITORY-SELECTION-001
---

# HTTP task repository selection design

## Purpose and boundaries

The task HTTP handler owns the two request formats. The task service owns admission, repository resolution, and task allocation.
Workspaces supply the registered repository identities and branch settings.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.1`, `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.2` | [Request normalization](#request-normalization) |
| `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.3`, `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.4` | [Validation and allocation](#validation-and-allocation) |

## Request normalization

`httpCreateTaskRequest` binds `repository_ids` to a `[]string` alongside the existing `repositories` slice.
Each slice is nil for omission or JSON `null`. An empty array yields a non-nil slice.

After JSON binding, `httpCreateTask` resolves a supplied `external_id` through `FindTaskByExternalIDForCreate` before request-specific validation.
If the identity exists, the handler returns that task without validating repository selection or the other create fields.
If the lookup misses, the handler continues with normal validation and `Service.CreateTask` repeats the lookup to cover a concurrent create.

When only the ID slice is present, `httpCreateTask` allocates a `[]httpTaskRepositoryInput` of the same length.
It copies each ID into `RepositoryID` at the same index and assigns the result to `body.Repositories`.
An empty ID slice produces a non-nil empty selection. Each entry leaves branch and checkout options at their defaults.

`convertCreateTaskRepositories` validates and converts the normalized entries.
`convertTaskRepositories` passes the normalized presence to `Service.CreateTask`.
That presence controls [automatic project source selection](project-repository-context.md#selection).
Parent inheritance and branch selection follow the same service path as canonical requests.

## Validation and allocation

For an identity miss, the handler rejects two non-nil slices before it calls `Service.CreateTask`.
It uses `rejectedCreateMessage` for HTTP 400 and names both fields in the error.
Malformed JSON values use the existing binding error. Entries with an empty ID use the existing repository input error.

The service checks workspace access and resolves repository references through the canonical path.
For a new task, it prepares the full repository selection before inserting a task row or publishing `task.created`.
Existing `external_id` winners follow the [task creation retry contract](external-id-idempotency.md) and bypass request-specific validation after successful JSON binding.

## Related decision

[ADR for REST task repository IDs](../../../decisions/2026-11-25-rest-task-repository-ids.md)
