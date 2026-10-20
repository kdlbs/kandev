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
| `AC-TASKS-SUBTASK-COMPLETION-TRIGGER-001.1` (busy parent failure mode and scenario) | Queue when busy, Step handoff across attempts, Turn ending while queueing, Operation settlement |

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
   as before. If that launch is refused before the prompt reaches the agent
   because the session is running a turn (`isPromptRejectedAsBusy`: agent
   already running, prompt in progress, or session not promptable), the prompt
   is still owed, so the callback starts over: it queues the prompt if the
   session is now mid-turn, or launches again if that turn has already ended.
   It makes at most `maxBusyAutoStartAttempts` (3) launch attempts, and checks
   for a running turn once more after the last busy rejection. A failure
   after the agent accepted the prompt, such as a stream disconnect or an
   uncertain delivery, is returned unchanged, so a prompt that may already have
   been delivered is never queued or sent again.

## Step handoff across attempts

A step handoff carry token is claimed once per step entry.
`StartSessionForWorkflowStep` claims it before `promptTask` can refuse the
prompt as busy. The callback therefore creates one `stepHandoffOnce` per
action and passes it in the context, scoped to the task and step
(`withStepHandoffOnce`). `StartSessionForWorkflowStep` claims through it
(`claimStepHandoffTextForLaunch`), and the queue path receives the same once, so
text claimed by a refused launch rides on the retry or on the queued prompt.
Without a scoped once, the launch claims directly as before.

## Turn ending while queueing

The ready handler that ends a turn settles it and drains the queue under the
session guard. The callback reads the session state, then queues, without that
guard, so the turn can end in between: its drain then finds an empty queue.
After queueing, the callback re-reads the session. When the session is no
longer mid-turn, it runs the guarded queue drain
(`drainQueuedMessageForPromptableSession`), which delivers the prompt once; a
concurrent drain claims the same message at most once. The callback skips this
drain when its caller already holds the session guard, since that caller owns
the turn settlement and its drain.

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
the queue drains. `TestProcessOnChildrenCompleted_DeliversWakeWhenTurnEndsWhileQueueing`
ends the parent's turn between the session read and the queue insert and checks
that the prompt is delivered once and leaves the queue. The
`TestAutoStartAgentCallback_*` tests cover a busy rejection followed by an ended
turn (launched again), by a started turn (queued), a non-busy launch error
(returned, nothing queued), and a handoff claimed by a refused launch (carried
on the queued prompt).
