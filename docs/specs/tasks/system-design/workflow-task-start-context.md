---
status: current
system: tasks
created: 2026-11-16
updated: 2026-11-16
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007
---

# Workflow context for ordinary task starts

## Purpose and boundaries

The task system owns workflow placement and initial prompt composition.
The agent runtime consumes the prepared prompt and session configuration.
This design extends [workflow start ownership](workflow-step-agent-start-ownership.md) at the ordinary `startTask` boundary.
It preserves workflow entry, reset, admission, and provider transport ownership.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007` | [Step identity](#step-identity), [Composition and compatibility](#composition-and-compatibility), [Verification](#verification) |

## Step identity

`orchestrator.Service.startTask` handles `LaunchSession(IntentStart)` and the `StartTask` wrappers.
It already resolves an omitted request step into `workflowSessionConfigStepID` from the task's stored `WorkflowStepID`.
Session routing, source binding, and `applyWorkflowSessionConfigBeforeLaunchForStep` consume this value.

The incoming `workflowStepID` retains its existing meaning for task movement, session creation, events, launch options, and replay payloads.
Prompt composition uses a separate local value:

1. An explicit step argument remains authoritative.
2. An ordinary omitted-ID start uses `workflowSessionConfigStepID`.
3. An Office run or explicitly spawned helper without a step argument retains an empty prompt step.

The existing `isOfficeTask` result and `opts.PreserveWorkflowSourceBinding` identify the separate launch contracts.
`ProfileExplicit` controls profile routing only. It does not disable ordinary workflow guidance.
No new lookup, setting, public field, or resolver is required.

## Composition and compatibility

`startTask` passes the prompt step to its existing `applyWorkflowAndPlanMode` call.
The call retains `task.IsEphemeral`, passthrough mode, plan mode, and `EntryOptions.SkipStepPrompt`.
`buildWorkflowPromptWithTrustedContextOptions` remains the owner of template substitution, workflow instructions, saved references, and skip semantics.
One-time move instructions retain their existing append and sentinel behavior.
`applyLaunchPromptContext` adds the existing launch context afterward.

| Launch shape | Prompt step |
| --- | --- |
| Ordinary start with an explicit step | Explicit argument |
| Ordinary start with only a stored step, including an explicit profile | Existing resolved step |
| Office start without a step argument | Empty. The Office launch context owns its prompt. |
| `spawn_session_kandev` without a step argument | Empty. `PreserveWorkflowSourceBinding` retains the helper contract. |
| Task without a step | Empty |
| Ephemeral task | Existing guard bypasses workflow injection |
| Passthrough task | Same visible template rules. Existing guards exclude hidden reference context. |

The correction does not assign the resolved step back to the incoming argument.
It does not invoke a workflow move, `ProcessOnTurnStart`, or `processOnEnter` to obtain context.
The [saved-prompt design](saved-prompt-delivery.md) and [initial brief design](initial-task-brief.md) retain their existing authority.
In particular, explicit `start_created` input preservation remains a separate contract.

## Failure, persistence, and security

If the task read used to resolve an omitted step ID fails after task classification succeeds, the ordinary start logs the failure and continues with the caller prompt, or the task description when the caller prompt is empty. It does not compose a workflow-step template in this degraded path. This preserves the existing launch behavior and is covered by acceptance criterion `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.8`.

A step read failure during source binding still fails before launch.
The correction adds no retry, fallback step, or successful dispatch after an existing launch failure.
The task row remains the stored-step source. Existing metadata and message persistence retain configuration and composed prompt content.
No schema migration, backfill, metric, or additional logging is required.
Authorization, MCP context ownership, and prompt sanitization retain their existing order.

## Verification

The service regression uses SQLite, the real scheduler/executor path, and a mock agent manager.
The launch callback captures `LaunchAgentRequest.TaskDescription` and reads persisted runtime overrides before launch returns.
It asserts a distinct step sentinel, selected profile, model, and reasoning option together.
An explicit-current-step start provides the positive control.

Additional cases cover distinct explicit steps, template variants, supplied input, skip overrides, plan mode, and saved references.
Negative controls cover Office, helper, ephemeral, and unbound-task starts.
Message capture compares stored and dispatched instructions.
These cases prove composition at the runtime request boundary without provider access or browser setup.

## Related records

- [Current-step prompt delivery plan](../../../plans/task-start-current-step-prompt/plan.md)
- [Workflow session configuration ADR](../../../decisions/2026-08-01-workflow-session-original-configuration.md)

This correction reuses existing ownership and configuration contracts. It needs no new architecture decision.
