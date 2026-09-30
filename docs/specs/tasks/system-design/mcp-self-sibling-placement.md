---
status: current
system: tasks
requirements:
  - REQ-TASKS-SELF-SIBLING-001
  - REQ-TASKS-SELF-SIBLING-002
---

# MCP self sibling placement design

## Boundary and mapping

Tasks owns placement. MCP translates caller intent; task service remains the
final depth/persistence authority. This design changes the session-bound Kanban
creation path only, preserving [mode admission](mcp-workspace-mode.md).

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-SELF-SIBLING-001 | Request intent; Resolution; Result contract |
| REQ-TASKS-SELF-SIBLING-002 | Inheritance and coordination; Failure and compatibility; Verification |

## Request intent

`internal/mcp/server/handlers.go:createTaskHandler` replaces `self` with
`s.taskID` and sends internal boolean `parent_is_self: true` only for literal
self, preserving the distinction from an explicit UUID. Do not expose this
field in the registered tool schema.
Explicit IDs and omitted parent produce no marker. Preserve the existing
no-current-task error and `source_task_id`/`source_session_id` payload.

Add the boolean to `mcpCreateTaskRequest` in `handlers/create_task_mode.go`.
It carries placement intent, never authority. A marked request must have a
verified non-automation Kanban principal and its requested ParentID must equal
that principal's CallerTaskID. Reject a forged/mismatched marker, including a
trusted external request bearing it; keep current Office and automation policy.
The normal MCP schema must reject an agent-supplied `parent_is_self` argument.

## Resolution

Integrate a small helper into `admitMCPCreateTask` after
`validateMCPKanbanPrincipal` succeeds and before `admitMCPDestination` resolves
and authorizes the destination. Do not attempt a failing create and retry it.

1. For unmarked requests, retain the existing path.
2. Load the verified caller. Preserve ephemeral-task rejection before redirecting.
   A root keeps its requested parent unchanged.
3. For a non-Office child C, load C.ParentID as P. Require P to be a valid
   non-ephemeral Kanban root in C's workspace. Do not walk to P's parent if P
   is itself nested. Missing/invalid references fail before repository or
   creation side effects; inaccessible references must not disclose details.
4. Set req.ParentID to P.ID and carry request-scoped resolution metadata in
   `mcpCreateTaskAdmission`. Run normal destination workspace/workflow checks
   against P. Do not overwrite SourceTaskID or SourceSessionID.
5. Continue `resolveTaskRepositories`, `applyMCPTaskScopeDefaults`, launch
   metadata resolution, and service creation once with that effective parent.

Keep `resolveTaskRepositories` and `Service.validateSubtaskDepth` strict for
explicit IDs and non-MCP callers. Do not remove their Office exception.
Resolution uses the hierarchy read for this request; no new tree locking or
historical reparenting is introduced. Later service validation may reject an
invalidated destination; never retry by climbing to another ancestor.

## Inheritance and coordination

The effective parent owns parent-derived scope and materialization. Use the
existing override semantics for repository/base branch and workspace_mode.
Do not blindly replace all execution settings with P's settings:
`resolveMCPLaunchMetadataWithSource` keeps the verified creator/session inputs
and existing workflow/profile precedence. Ledger attribution in
`handleCreateTask` stays bound to SourceSessionID.

No changes to `parent_question.go`, `stop_task.go`, `task_target_access.go`,
peer-message dispatch, or `orchestrator/event_handlers_children_completed.go`
are needed. Their existing ParentID rules make P the coordinator. Completion
still aggregates all direct active children of P, including C and S; it is not
a completion callback for C's independently requested work.

## Result contract

Extend MCP-only `mcpCreateTaskResult` with optional `parent_resolution`:

```json
{
  "requested_parent_id": "C",
  "resolved_parent_id": "P",
  "reason": "kanban_depth_limit",
  "message": "Created a sibling task under P because the Kanban subtask depth limit was reached. P owns coordination; your session remains the creation source."
}
```

The enclosing result's existing `parent_id` always describes the returned task.
Emit this object only when this request was redirected, on each successful
result branch: ordinary created, created identity lost, found settled, and
found pending. For either found outcome, the message instead says the request
was resolved to P and an existing task was returned without creation or
reparenting; explicitly direct the caller to the returned `parent_id` for its
actual parent. Do not imply that an unrelated deduplicated task is a sibling or
that this caller created it. Preserve `deduplicated` and `creation_complete`.
A small result-decoration helper can keep all success branches consistent.

`createTaskHandler` already serializes the result map, removing only the echoed
description. Preserve the additive object through that boundary. MCP diagnostic
text stays English, following backend guidance; no browser localization changes.
Do not create a frontend renderer or change REST response DTOs.

## Failure and compatibility

Resolve before side effects and use existing admission error categories.
Missing/unauthorized parent is not found; inconsistent/invalid placement is a
validation error; disallowed caller identity retains unauthorized/forbidden
behavior. Preserve operational-error classification rather than turning all
lookup failures into successful fallback. No notice is attached to errors.

External-ID lookup retains its existing workspace-scoped semantics: the actual
returned task can have a different parent. Do not add a new uniqueness key or
mutate that task to fit this request. Retry safety covers already-valid retry
requests; it does not bypass current admission when the tree/context changed.
Settlement uses the service-normalized create request's external ID, even if
the refreshed task has lost it during synchronous workspace attachment. An
identity release must still suppress launch and return the surviving created
task with its actual identity and parent.

New server + old backend ignores the internal marker and retains the old depth
error; old server + new backend lacks the marker and also retains that error.
Fallback is available when both halves support it. No silent reinterpretation
of legacy explicit-ID traffic. Update the task tool descriptions and public
coordination/MCP references with the implementation, not during draft planning.
No storage migration, flag, rollout service or new telemetry counter is needed;
the additive result provides the placement evidence.

## Verification

Test literal-self propagation and full result JSON at the MCP server; use a
real dispatcher and SQLite task service for a root/child/sibling journey.
Test admission failures, inheritance, creator ledger, new/existing results,
identity-loss branch, deferred launch and strict explicit IDs at handlers.
Retain direct-parent authorization and service depth tests. Named tests and
exact commands are in the [work order](../../../plans/mcp-self-sibling-placement/task-01-self-placement.md).
No rendered UI changes or Playwright suite are required for this MCP contract.

## Related decisions

- [Automatic self placement](../../../decisions/2026-09-19-mcp-self-sibling-placement.md)
- [Idempotency](external-id-idempotency.md)
- [Plan](../../../plans/mcp-self-sibling-placement/plan.md)
