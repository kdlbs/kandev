---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RELATED-DISCOVERY-001
---

# Related task discovery design

## Purpose and boundaries

Align MCP discovery text with the existing task relation gate.
The task service remains the authority for target eligibility.
No permission, transport, persistence, or graph-projection change is required.

The paired [requirements](../requirements/related-task-discovery.md) own the observable contract.
The [repair plan](../../../plans/related-task-access-contract/plan.md) owns implementation and verification.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-RELATED-DISCOVERY-001` | Advertised contract; Request path; Compatibility and failure; Verification |

## Advertised contract

Update `registerRelatedTasksTool` in `apps/backend/internal/mcp/server/handoff_handlers.go`.
The tool description states the permitted target relations, same-workspace restriction, and unrelated-task denial.
The `task_id` help states the default and relation restriction.
The `verbose` help states that full descriptions do not expand access.

Keep the tool description within its existing 500-character limit.
Put the detailed relation rule in the tool description, rather than duplicating it in each property.
One possible description is:

> List a task's parent, children, siblings, blockers, and blocked tasks, with IDs, states, PRs, and Office document keys. task_id defaults to self. Another target must be an ancestor, descendant, sibling with a shared non-empty parent, or blocker of the caller, in the same workspace. Unrelated tasks are denied. Descriptions require verbose=true. verbose changes output only, never access.

The registered tool is shared by task and Office modes.
Both receive the same restriction without registration changes.
The blocker relation is directional: the target blocks the caller.
A target that only depends on the caller does not gain independent admission through that edge.

Update these existing public sections in the implementation PR:

- `docs/public/coordination.md`, the paragraph about conversation and relation discovery.
- `docs/public/agent-communication.md`, **Discovering task IDs**.
- `docs/public/automation-and-mcp.md`, the relation-tool reference near change-request fields.

The coordination page currently promises inspection of another same-workspace task.
Replace that promise with the relation restriction.
The other pages link to or summarize the same rule.
State that no compact permission for unrelated targets exists in this tool.
Do not suggest changing hierarchy or dependencies as an authorization workaround.

## Request path

1. `listRelatedTasksHandler` resolves an omitted target or `self` to `s.taskID`.
2. The server supplies `caller_task_id: s.taskID` in the internal backend request.
3. `handleListRelatedTasks` calls `HandoffService.ListRelatedForCaller`.
4. For a non-self target, the service evaluates `canReadDocuments` before constructing the relation projection.
5. An allowed request uses the existing `ListRelated` projection and change-request enrichment.
6. The MCP server removes descriptions after the backend response unless `verbose` is true.

`canReadDocuments` remains in `apps/backend/internal/task/service/handoff_access.go`.
Its same-workspace and relation predicates remain unchanged.
The registered schema does not expose `caller_task_id`.
The existing argument validator rejects a caller override before dispatch.

## Compatibility and failure

Preserve `RelatedTasks`, `RelatedTask`, `change_requests`, legacy GitHub `prs`, and document-key behavior.
Keep the existing error mapping from `ErrAccessDenied` to `FORBIDDEN`.
A forbidden request must not become a successful empty relationship list.

Compact output is not a minimal backend projection.
`ListRelated` constructs rich records, and the backend enriches change requests before the MCP server strips descriptions.
Therefore, bypassing `canReadDocuments` for `verbose: false` does not implement a separate metadata boundary.

The existing trusted backend `ListRelated` callers and missing-caller fallback in the backend handler remain outside this repair.
This design concerns the task-bound MCP tool, whose adapter supplies the caller.
It does not claim that every internal dispatcher path has the same identity semantics.

No database migration, runtime flag, setting, metric, or UI component is required.
This correction records an existing boundary; it does not choose a new authorization model.

## Verification

Use registered-tool metadata tests for discovery wording and schema help.
Use registered-handler tests for caller binding, argument rejection, and forbidden-error propagation across all `verbose` forms.
Use task-service tests for the actual relation predicate, including independent root tasks.
Retain existing ancestor, descendant, sibling, blocker-direction, missing-task, and cross-workspace controls.

Public-doc validators check the edited pages and local links.
The work order names exact commands and the regression that must fail before the wording correction.
No browser is required because this repair changes no rendered product UI.

## Related decisions

- [Reject MCP actions on raw WebSocket](../../../decisions/2026-07-19-reject-mcp-actions-on-raw-websocket.md) defines the transport-origin boundary.
- [Session MCP definition details](../../../decisions/2026-08-18-session-mcp-tool-definition-details.md) defines the observed registered-tool catalog.
