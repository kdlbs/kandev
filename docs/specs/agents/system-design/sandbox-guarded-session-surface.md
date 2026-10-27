---
status: draft
system: agents
created: 2026-10-26
requirements:
  - REQ-AGENTS-SANDBOX-009
  - REQ-AGENTS-SANDBOX-010
---

# Guarded session surface system design

Extends [Sandbox policy system design](sandbox-policies.md), which owns task guards and admission. This file owns the dispatcher and approval surface for guarded sessions.

## Design

A task guard row in `sandbox_task_guards` is the only definition of a guarded session. `sandbox.Service.TaskGuarded(workspace, task)` reads it by task and checks the stored workspace. `mcp/scope.Resolver` calls it through the `GuardSource` interface while building the principal and sets `Principal.Guarded` for Kanban and Office task surfaces only. A read error fails principal resolution, so the dispatch is denied.

`guardedMCPDispatcher.RegisterFunc` runs `authorizeGuardedRequest` after the automation and coordinator guards. The guard looks the action up in `mcp/profile` `GuardedToolByAction` (bare tool names, classes `read`, `self`, `child_write`, `internal`, `denied`). Unlisted and `denied` actions return an unknown-action error. Others pass through `guardedPayload`, which keeps keys in document order, folds them case-insensitively, rejects duplicates, rewrites attribution fields, and re-marshals only canonical names. `child_write` and `self` references resolve through `taskSvc.GetTask` and `GetTaskSession`; a target must be the caller's task or a direct child in the caller's workspace. `read` actions resolve `task_id`, the task of `session_id`, `workflow_id`, and `workspace_id` and require the caller's workspace.

Child-task guard inheritance, executor admission, and weaker-executor refusal stay in `sandboxCreationAdmission` and `AdmitTask`. This surface adds no second floor. Because every child of a guarded task inherits a guard, a direct child is itself guarded, so `child_write` cannot reach an unguarded task. The guard read and the mutation are separate reads; reparenting between them is an accepted window.

agentctl treats an instance with `cfg.Sandbox != nil` or `cfg.SandboxRequired` as guarded. `nonCoordinatorAutoApproval` first calls `guardedKandevApproval`, which approves only `read` and `self` names after the exact `_kandev` suffix strip, and `adapterAutoApprove` returns false. No new carrier field or launch-path change is needed.

| Requirement | Mechanism |
| --- | --- |
| REQ-AGENTS-SANDBOX-009 | `scope/guard.go`, `mcp/profile/guarded_tools.go`, `mcp/handlers/guarded_authorization.go`, `guarded_payload.go` |
| REQ-AGENTS-SANDBOX-010 | `agentctl/server/process/manager_permission_policy.go`, `managed_tool_policy.go` |

Ordering: guard resolution precedes the dispatcher guard, which precedes the handler. Idempotency: the guard has no state; a retried call is evaluated afresh. Concurrency: the task guard is version-checked on write and never removed, so a session cannot become unguarded between calls. Empty or nil: a nil payload is `{}`; an empty reference references nothing.
