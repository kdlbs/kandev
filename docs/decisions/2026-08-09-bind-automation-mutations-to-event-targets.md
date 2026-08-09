# ADR-2026-08-09: Bind Automation Mutations to Event Targets

**Status:** accepted
**Date:** 2026-08-09
**Area:** backend, agentctl, protocol, security, workflow

## Context

The `github_pr_merged` automation creates an ordinary automation-run task and asks its agent to
archive the task whose pull request merged. The event evaluator has already resolved and authorized
that target, but the generic `archive_task_kandev` tool accepts an arbitrary task id. A narrow prompt
can describe the right target, yet it cannot stop a mistaken or manipulated agent from naming another
task reachable by the run task's owner.

Kandev already binds some task-scoped MCP operations to server-owned context. For example,
`set_task_title_kandev` derives its target from the current MCP server instead of accepting an
arbitrary task id. The merged-PR flow needs the same structural property without replacing the
automation engine's existing create-and-start-task action model.

## Decision

When an event-driven automation authoritatively selects a task that a later agent tool call may
mutate, the selected target is persisted on the automation-run task as server-owned metadata. For
`github_pr_merged`, the persisted value is the validated `trigger_data.task_id` under
`automation_target_task_id`.

The in-session MCP server injects its current run-task id as `caller_task_id` into archive requests
sent to the backend.
This caller id is transport context, not an advertised or caller-editable tool argument. At the
backend mutation boundary, `archive_task_kandev` loads the caller task. If the caller is a
`github_pr_merged` automation run, the requested task id must exactly match the persisted event target.
A missing target, malformed metadata, or mismatch is rejected without archiving any task.

The bound check is additive to the existing owner authorization. Ordinary task sessions and
automation runs from other trigger types retain the generic archive behavior. The automation prompt
remains editable and still tells the agent what to do, but it is guidance rather than the security
boundary.

The run metadata is immutable for this purpose after creation. Moving the intended target between
workspaces can still make the normal owner authorization reject the archive; the binding never grants
additional reach.

## Consequences

- A merged-PR automation run cannot archive a task other than the one selected by its event, even if
  its prompt is edited or the model supplies a different id.
- No native automation action type or second execution engine is introduced.
- The current run-task id crosses the agentctl-to-backend request as trusted server context, so tests
  must prove it is injected rather than accepted from the MCP schema.
- Automation-run metadata becomes part of the enforcement path. Missing or malformed target metadata
  fails closed and must be observable in logs and tool errors.
- The generic archive tool keeps its existing contract for callers outside this specific bound flow.

## Alternatives Considered

1. **Rely on the narrow trigger data and pinned prompt.** Rejected because prose cannot constrain a
   tool argument and the calling owner may reach tasks outside the automation's workspace.
2. **Add a native archive action to the automation engine.** Rejected because it creates a second
   action model for one trigger and bypasses the existing run-task transcript and lifecycle without
   being necessary for deterministic enforcement.
3. **Add a dedicated merged-PR archive MCP tool.** Rejected because the existing archive handler can
   enforce the caller-bound target with less protocol and tool-discovery surface.
4. **Restrict every automation run to one mutation target.** Rejected because other trigger types do
   not currently define a single authoritative task target and may intentionally perform broader
   workflows.
