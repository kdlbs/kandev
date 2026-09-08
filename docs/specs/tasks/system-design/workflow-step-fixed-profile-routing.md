---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-SESSION-SETTINGS-001
---

# Workflow Step Fixed-Profile Routing System Design

## Purpose and boundaries

The task and workflow system selects the task session that executes a workflow step.
A fixed step profile is a session-routing instruction.
This instruction does not depend on the ACP or CLI-passthrough transport.
The agent runtime controls how each selected session starts and stops its process.

Conditional session settings are separate. They modify the original session without changing its profile and do not use this routing flow.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKFLOW-SESSION-SETTINGS-001` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `orchestrator.Service.resolveStepAgentProfile` resolves the pinned step profile before the workflow default.
- `orchestrator.Service.preflightWorkflowStepCredentials` validates the executor and repository credential boundary before a transition persists.
- `orchestrator.Service.prepareWorkflowStepSession` selects the session that owns destination-step entry actions.
- `orchestrator.Service.switchSessionForStep` applies the destination step's start setting when it reuses a nonterminal destination session or prepares a new session in the task's existing environment, and the source step's end setting when it retires the previous session.
- The agent runtime stops the replaced execution and lazily starts or resumes the destination session according to that session's profile and transport.

## Data and contracts

`workflow_steps.agent_profile_id` is the fixed-profile override.
An empty value uses the workflow profile, then the active session profile.
A selected profile stays attached to its own `task_sessions.agent_profile_id`.
Routing never rewrites a live session to impersonate another profile.

`workflow_steps.profile_session_start_policy` governs whether the destination
step reuses an eligible session or starts a fresh conversation. The source
step's `workflow_steps.profile_session_end_policy` governs whether the replaced
session is completed or parked. The full lifecycle contract is in
[Workflow Profile Session Lifecycle](workflow-profile-session-lifecycle.md).

The destination session inherits the current task environment and executor profile. Its own agent profile determines ACP or CLI-passthrough launch behavior.

## Control flow

1. Before committing a workflow transition, resolve the destination step's effective profile and preflight its reusable or prospective session against managed Git credential requirements.
2. After the destination step is selected and before any entry action runs, resolve the effective profile and normalized start policy from that step and the source step's normalized end policy.
3. If the effective profile is empty or matches the active session, keep that session and make it primary when needed.
4. If the profile differs, select reuse or new-session behavior from the destination step's normalized start policy.
5. Promote the destination session, transfer queued messages and pending move state, then complete or park and stop the replaced session according to the source step's end policy.
6. Run destination-step entry actions with the destination session ID. Transport-specific action support is evaluated only after routing.

The active session transport does not stop profile resolution, credential preflight, or session switching.
Passthrough checks remain valid for operations that a CLI session cannot perform.
ACP-only mode changes are one example.

## Failure and recovery

Credential validation stops the operation before the workflow step changes.
Session preparation occurs before Kandev completes the current session.
If either operation fails, Kandev keeps the previous session active.
Kandev does not execute the destination step with the wrong profile.

If queue transfer fails, Kandev logs the error.
The queued state stays on the previous session for manual recovery.
Kandev never revives a terminal matching session.
Kandev creates a new session instead.

## Persistence

This flow uses existing workflow-step, task-session, task-environment, and
session-metadata records.
The repositories retain profile identity, primary ownership, terminal or parked
state, and workflow-switch provenance after a restart. The workflow-step row
stores both profile-session policies.

## Security

The transition keeps existing task authorization.
Managed Git credential preflight runs for every transport before the destination profile takes ownership.
Errors identify safe repository IDs and do not identify credential values.

## Observability

Profile changes emit the existing structured switch log.
The log contains the task, source session, current profile, and destination profile.
Preflight and preparation errors use the existing workflow transition error paths.
This change does not require a new metric.

## Related decisions

- [Task model unification](../../../decisions/0004-task-model-unification.md)
- [Agent model unification](../../../decisions/0005-agent-model-unification.md)
- [Per-CLI MCP server injection for passthrough mode](../../../decisions/0014-passthrough-mcp-injection-strategies.md)
- [Make workflow profile-session switching explicit](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
