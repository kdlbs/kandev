---
status: current
system: tasks
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-001
---

# Subtask Completion Busy Parent Wake System Design

## Purpose and boundaries

This design covers how an `auto_start_agent` action that the workflow engine
runs for `on_children_completed` reaches a parent session that is still in a
turn. Trigger detection, child-set idempotency, and step transitions follow the
[subtask completion trigger requirements](../requirements/subtask-completion-trigger.md)
and are unchanged. The `on_enter` auto-start path is unchanged.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-TASKS-SUBTASK-COMPLETION-TRIGGER-001.1` (busy parent failure mode and scenario) | Queue when busy, Operation settlement |

## Queue when busy

`compileGenericActions` compiles a generic `auto_start_agent` action with
`AutoStartAgentAction{QueueIfBusy: true}`. The engine runs it through
`autoStartAgentCallback`, which honors that flag:

1. When the session is `RUNNING` or `STARTING`, the callback builds the step's
   workflow entry prompt (`buildWorkflowEntryPrompt`) and passes it to
   `autoStartStepPromptWithPromptContext` with `shouldQueueIfBusy`. That is the
   function the `on_enter` auto-start uses: for a running session it queues the
   prompt as a workflow message (`queueAutoStartPromptIfRunning`), merging any
   queued hand-off message and carrying a claimed step handoff as metadata. The
   ordinary queue drain delivers it after the current turn ends.
2. Otherwise the callback launches through `LaunchSession(IntentWorkflowStep)`
   as before. If that launch fails with a busy error (`isBusyLaunchError`: agent
   already running, prompt in progress, session not promptable, or a transient
   prompt error) and the session is now mid-turn, the callback queues the prompt
   as in step 1.

`isBusyLaunchError` is the same predicate the created-session auto-start uses
(`handleCreatedAutoStartLaunchFailure`). An uncertain prompt delivery is not a
busy error, so the callback never queues a prompt that may already have been
delivered.

## Operation settlement

A queued prompt is a successful action, so `evaluateChildrenCompleted` returns
the non-transition result and `processOnChildrenCompleted` marks the
children-completed operation applied. A second delivery of the same completed
child set (task state change and terminal step move) finds the operation applied
and does not queue a second prompt.

## Verification boundary

`TestProcessOnChildrenCompleted_QueuesAutoStartWhileParentIsRunning` covers a
running parent receiving two deliveries of the same completion: no direct
prompt, one queued workflow message, the operation applied, and one prompt after
the queue drains.
