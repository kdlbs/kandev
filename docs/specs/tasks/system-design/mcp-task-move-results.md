---
status: draft
system: tasks
requirements:
  - REQ-TASKS-MCP-MOVE-RESULTS-001
---

# MCP task move results

## Purpose and boundaries

The MCP backend handler owns destination-result classification before it selects immediate or deferred execution.
The task service remains the source of task data and authorization.
The queue and orchestrator retain ownership of real deferred transitions.

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-MCP-MOVE-RESULTS-001 | Classification and response |

## Classification and response

`Handlers.handleMoveTask` in `internal/mcp/handlers/config_task_handlers.go` first parses required fields and normalizes entry options.
A small helper then reads the task through `taskSvc.GetTask` and compares both workflow and step identifiers.
For different destinations, normal routing continues. No active-session permission is broadened.

For an equal destination, the helper validates the candidate before returning success:

1. Enforce `authz.ScopeTaskWrite` through the existing `AuthorizeWorkspaceScope` service method.
2. Reject archived tasks.
3. Resolve the workflow and validate workspace membership through existing service accessors.
4. Resolve the step through `workflowCtrl.GetStep` and validate workflow membership.
5. Use `workflowmove.ValidateEntryOptions` with `MoveChangePositionOnly`.

Target lookups preserve typed not-found errors. A missing or inaccessible workflow or step remains a validation error; operational repository and controller failures are logged and return `internal_error` without exposing the underlying error to the MCP caller.

Production requires the task and workflow dependencies. A missing validation dependency must fail closed, without a fabricated success.
Handler fixtures for this branch must supply real workflow steps or the existing workflow controller fixture.

The helper returns `dto.MoveTaskResponse` with `dto.FromTask(task)` and `moveDispositionApplied`.
It leaves `MoveID`, `EntryOptions`, and `WorkflowEntryIdentity` empty.
It does not use `synthesizeMovedTaskDTO`, because that function can substitute requested values.
It completes before `lookupSession`, `deferMoveTask`, and `applyMoveTaskImmediate`.
Thus a valid no-op does not depend on queue availability or session liveness.

Task-mode registration in `internal/mcp/server/server.go` and configuration-mode registration in `config_handlers.go` describe the same result.
Their move and prompt descriptions must limit deferral language to actual step changes.
Their position descriptions must agree with the existing server-owned ordering contract.
`moveTaskHandler` continues forwarding the existing response envelope.

## Concurrency, persistence, and recovery

The no-op performs no writes and requires no schema change or queue admission.
Its success refers to the authorized task snapshot. A later independent transition can still change that task.
The handler must not call the generic move service for this branch: that path checks active sessions and performs writes.

The no-op neither replaces nor consumes an existing pending move, including one for another destination.
It does not cancel previously accepted work or erase entry markers.

`orchestrator.applyPendingMove` retains its equal-target guard for older records and destinations reached after admission.
That guard already records applied move identity and consumes the matching record through the applicable queue path.
This change does not add a database sweep or infer why an older deployment retained a particular row.

## Failure and security

Malformed payloads and conflicting prompt aliases fail during existing normalization.
Task read failures, target lookup failures, authorization failures, and archive restrictions must not become `applied` responses.
Missing or inaccessible targets retain validation errors. Operational lookup failures return `internal_error`, and response messages must not expose database or controller details.
Responses must not reveal inaccessible task data.
Both workflow and step equality are required. Matching only the step identifier is insufficient.

## Validation and observability

Handler tests cover SQLite task state and a recording queue, with repeated requests and mixed session states.
A handler integration case uses the real message queue repository to prove that pending rows do not grow.
Existing deferred-move and immediate-move tests prove that actual transitions retain their behavior.
Tool tests cover both registrations and MCP response forwarding.

The no-op emits neither `pending move recorded` nor workflow transition events.
No new metric or logging stream is necessary.

## Related contracts

- [Kanban task ordering](kanban-task-reordering.md)
- [Workflow move overrides](../../workflow-step-move-overrides/spec.md)
- [Move override decision](../../../decisions/2026-08-13-workflow-move-overrides.md)
- [Implementation plan](../../../plans/mcp-same-step-move/plan.md)
