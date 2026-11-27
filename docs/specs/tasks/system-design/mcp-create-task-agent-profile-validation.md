---
status: current
system: tasks
requirements:
  - REQ-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001
---

# MCP Create-Task Agent Profile Validation System Design

## Purpose and boundaries

This design covers `agent_profile_id` validation on `create_task_kandev` and the
visibility of an auto-start failure that happens after the tool has returned.

It does not change the omitted-argument resolution precedence owned by
[`mcp-task-agent-profile-default.md`](../requirements/mcp-task-agent-profile-default.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001` | [Synchronous validation](#synchronous-validation), [Tool description](#tool-description), [Asynchronous launch failure](#asynchronous-launch-failure) |

## Confirmed current behavior

`internal/mcp/server/handlers.go` copies `agent_profile_id` into the WebSocket
payload verbatim. `internal/mcp/handlers/handlers.go` then treats any non-empty
value as an explicit profile:

- `mcpTaskAgentProfileDefault` returns early for a non-empty value, so the
  per-user setting is not read;
- `useCreatorRuntime` requires `agentProfileID == ""`, so creating-session
  inheritance is skipped;
- `inheritFromTask` sets `agentProfileExplicit`, so parent and source
  inheritance is skipped;
- `resolveMCPFinalAgentProfile` returns early, so the workspace default is
  skipped.

The value reaches `config.AgentProfileID`, the task metadata key
`agent_profile_id`, and the deferred-launch record. Task creation succeeds.
`launchAutoStartTask` then starts a goroutine after the tool result has been
returned; `LaunchSession` fails in
`orchestrator.startTask` → `resolveEffectiveAgentProfile` →
`runtime.ValidateProfile` → `GetAgentProfile` with `sql.ErrNoRows`, and the
error is written to the backend log and dropped.

The only synchronous profile error today is `errMCPAgentProfileRequired`, raised
when the resolved profile is empty. The error machinery exists; it never fires
for a non-empty invalid value.

## Synchronous validation

Validation belongs in the backend handler, not in the MCP server shim, because
the shim has no profile store and both the session-bound and external MCP
transports reach the same handler.

`internal/mcp/handlers` gains a profile-existence check at the point where the
final profile is resolved, before `taskSvc.CreateTask`. It runs when the caller
supplied a non-empty `agent_profile_id`:

1. If the value equals `current_task` or `workspace_default`, return a
   validation error naming `mcp_task_agent_profile_default` and stating that
   omitting the argument selects the configured policy. These two strings are
   matched exactly; the check exists because the tool description names them and
   callers reasonably read them as argument values.
2. Otherwise resolve the profile through the existing agent-settings reader. A
   miss returns the same validation error code used for other create-task
   argument failures, with a message naming the unresolvable ID.

The reader is the interface the handler already holds for launch metadata; no
new dependency direction is introduced. A resolution failure that is not a miss
(store unavailable) fails the call rather than creating a task, because a task
created with an unverified profile is the outcome this requirement removes.

The check runs after workspace and parent admission so the caller sees the more
specific error first, and before any write, so
`AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.2` holds with no compensating
delete.

Omitted `agent_profile_id` skips the check entirely and keeps the existing
resolution path unchanged.

## Tool description

`registerCreateTaskTool` builds `agentProfileDesc` for both kanban and external
modes. The description is restructured so the argument contract and the setting
are separate sentences: the argument accepts an agent profile ID; when it is
omitted, the per-user `mcp_task_agent_profile_default` setting selects between
its `current_task` and `workspace_default` policies. The existing schema test
that pins `current_task` in the description is updated to pin the sentence that
identifies it as a setting value.

## Asynchronous launch failure

`launchAutoStartTask` keeps its goroutine, because the tool must not block on a
session launch. Its failure handling changes from a log-only `return` to a
recorded outcome:

- the failure reason is stored on the task through the existing task service so
  it survives a backend restart;
- the task event stream carries the failure through `publishTaskEvent`, so the
  kanban view and any MCP caller polling the task observe it.

Reason text is the sanitized launch error. It carries no credentials and no
workspace paths beyond what existing launch errors already surface.

Validation removes the common cause of this path; the recording covers the
remainder (workspace preparation failures, executor admission refusals) that the
same goroutine already swallows.

## Failure modes

- A profile deleted between validation and launch still fails asynchronously and
  is now recorded rather than silent.
- A caller passing a profile ID from another workspace receives the same
  not-found-shaped validation error as an unknown ID, so no cross-workspace
  existence is leaked.
- A store outage fails the call. That is a visible failure the caller can retry,
  unlike the current silent half-success.

## Observability

- Structured log `mcp.create_task.profile_rejected` with the rejection reason
  class (`policy_value`, `unknown_profile`, `store_error`). The profile ID is not
  logged for the unknown case beyond its presence, matching existing MCP error
  logging.
- Structured log and task event for a recorded auto-start failure.
