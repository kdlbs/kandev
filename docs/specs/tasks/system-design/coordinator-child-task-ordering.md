---
status: current
system: tasks
requirements:
  - REQ-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001
---

# Coordinator Child Task Ordering System Design

## Purpose and boundaries

The MCP adapter changes the same `tasks.position` used by board drag ordering
and WIP admission. It delegates persistence and publication to the existing
`task/service.ReorderStepTasks` contract. Global ceiling replay follows the
[agents design](../../agents/system-design/session-concurrency-ceiling.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001` | [Control flow](#control-flow), [Authorization and failure](#authorization-and-failure) |

## Data and contracts

`reorder_child_tasks_kandev` accepts an array of child IDs and
`placement: in_place | front`. `internal/mcp/server` registers it only in the
Kanban task profile, outside the shared automation catalog. Its handler builds
a fresh `mcp.reorder_child_tasks` payload and injects `sender_task_id` from the
server's calling task. The public schema does not expose sender attribution.

The response contains `workflow_step_id`, `band`, `placement`, `revision`,
and `tasks` entries with `id`, `title`, and `position` in the new band order.

## Control flow

1. The backend checks the current trusted Kanban-task principal against the
   injected sender and workspace, then resolves the sender and named children and validates their
   workspace, direct-parent relationship, workflow step, and band.
2. It reads the workflow's tasks and selects the target step's band. The queued
   predicate is `!wip_admitted && queued_for_step_id == stepID`; all other tasks
   in the step are admitted. `models.StepOrderLess` establishes current order.
3. `in_place` replaces only named slots. `front` prepends named IDs and appends
   remaining IDs in their existing relative order.
4. The adapter submits the complete band to `ReorderStepTasks`. The repository
   validates membership and commits positions and revision transactionally.
   Its step snapshot joins the owning workflow and requires both task workflow
   and workspace to match, agreeing with the service's workflow list. Legacy
   rows with inconsistent workspace/workflow references cannot create a
   permanent conflict, enter the returned order or receive position updates.
5. The service publishes `task.reordered`; existing board consumers and queued
   admission use those positions without a separate frontend or queue model.

## Authorization and failure

Task-service reads retain their normal workspace visibility checks. The
existing reorder service retains `ScopeTaskWrite` authorization. The MCP
adapter additionally requires same-workspace direct-parent access and checks
that relationship again in the band snapshot before submitting an order.
Its child-specific service/repository path adds the expected parent and workspace
to each named child's position update inside the same reorder transaction.
A changed relation returns the existing parent-mismatch error and rolls back all
positions and the revision without publishing an event. Ordinary UI reordering
uses its existing unrestricted-by-parent path and authorization.
The automation dispatcher rejects this action as outside its catalog.
Prompt producers also omit the task-control extension for persisted
`automation_run` origins, including created-session, new-launch, workflow-entry
and message pre-wrap paths. A visible `automation_task` retains its normal Kanban
controls. Fresh-launch provenance reuses the same canonical task lookup as
Office ownership instead of trusting the scheduler's reduced API DTO.

On `ErrStepChanged`, the adapter reads the band again, revalidates the named
children, and retries once. Another membership conflict returns `CONFLICT`;
an invalid request returns a validation error; denied child access returns
`FORBIDDEN`. Existing whole-band reorder semantics govern concurrent reorders
with unchanged membership; the MCP adapter adds no revision precondition.

## Persistence and observability

No schema migration, priority update, dependency mutation, or session operation
is needed. The existing reorder revision and workspace-scoped `task.reordered`
event describe committed changes; MCP errors describe rejected operations.
