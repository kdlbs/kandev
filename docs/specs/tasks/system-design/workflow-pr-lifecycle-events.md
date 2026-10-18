---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PR-EVENTS-001
---

# Workflow PR Lifecycle Events System Design

## Purpose and boundaries

The orchestrator owns the reaction to linked PR state changes. The GitHub PR
poller already publishes `github.task_pr.updated` (`events.GitHubTaskPRUpdated`)
after it stores changed PR data. The orchestrator already subscribes to that
event for CI and lifecycle-prompt automation. This design adds one more
consumer in the same handler and reuses the `on_children_completed` transition
path. No new table, route, WebSocket action, or MCP tool is introduced.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.1` | Step event model |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.2` | Evaluation; Idempotency |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.3` | Evaluation; Idempotency |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.4` | Session selection |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.5` | Evaluation |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.6` | Transition and history |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.7` | Step editor |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.8` | Compatibility |

## Step event model

`models.StepEvents` gains `OnPRMerged` and `OnPRClosed`, both
`[]GenericAction` with JSON and YAML keys `on_pr_merged` and `on_pr_closed`.
Every place that enumerates the generic triggers includes them:

- `engine.TriggerOnPRMerged` / `engine.TriggerOnPRClosed` and `CompileStep`;
- `CollectStepEventReferences` and `RemapStepEvents` (step-write reference
  validation and workflow copy);
- portable export validation and `ConvertPositionToStepID`;
- `ClearStepReferences` on step deletion;
- task DTOs, the settings catalog event schema, the plugin admin step merge, and
  the built-in template loader.

Payloads are `engine.OnPRMergedPayload{PRs []LinkedPR}` and
`engine.OnPRClosedPayload{PR LinkedPR}`.

## Evaluation

`Service.handleTaskPRUpdated` calls `processTaskPRWorkflowEvent` after starting
CI automation. Evaluation is synchronous and stops at the first failed gate:

1. Map the PR state: `merged` to `on_pr_merged`, `closed` to `on_pr_closed`,
   anything else ignored. Require a task ID, owner, repository, and a positive
   PR number, matching the `github_pr_merged` automation subscriber.
2. Load the task. Skip archived and ephemeral tasks and tasks without a workflow
   step. Load the current step and skip when it declares no actions for the
   trigger. This is where a merge on another step is ignored.
3. List the task's attached PRs (`ListTaskPRs`, which excludes detached rows).
   Skip when the reported PR is not among them. The reported PR's state replaces
   the stored row's state for the decision.
4. `on_pr_merged` requires every linked PR to be merged. `on_pr_closed` uses the
   reported PR only.
5. Select a session, take the per-operation lock, check the operation ledger,
   and call `Engine.HandleTrigger` with `EvaluateOnly` and
   `DeferOperationMark`, as `processOnChildrenCompleted` does.

## Idempotency

The operation id is deterministic:

- merged: `on_pr_merged:<task>:<sha256 of the sorted owner/repo#number keys of
  all linked PRs>`. A merged PR cannot be reopened, so one id per merged set is
  enough.
- closed: `on_pr_closed:<task>:<owner/repo#number>:<closed_at>`. A reopened and
  closed again PR produces a new id.

The id is marked applied after a non-transition evaluation and after a
committed transition, so poller redelivery of an unchanged PR does not repeat
the actions. The engine ledger survives restarts; bus events missed while the
backend is down are not replayed.

## Session selection

Engine state and step history are keyed by session. The handler uses the
active session (`CREATED`, `STARTING`, `RUNNING`, `WAITING_FOR_INPUT`) when one
exists. Otherwise it uses the primary session, then the most recent session in
any state, so an idle, parked, or completed agent does not block the move. No
session is created. A task without sessions is skipped with a debug log. No
message is queued and no agent turn is started by the handler itself; the
target step's own `on_enter` actions still run.

## Transition and history

A transition goes through `applyEngineTransitionWithMode` with
`transitionLifecycleWithOnEnter`, the same lifecycle as `on_children_completed`:
target validation, credential preflight, `on_exit`, committed step change,
history row, data patch, terminal-step completion, then asynchronous
`on_enter`. The history mapping adds `StepTransitionTriggerPRMerged`
(`on_pr_merged`) and `StepTransitionTriggerPRClosed` (`on_pr_closed`). The row
is written through `recordAutoStepTransition` with a nil actor id, so it is a
system move.

As with `on_children_completed`, a session that is still `RUNNING` when the
transition commits is set to `WAITING_FOR_INPUT`.

## Step editor

`EventTransitionSelect` generalizes the former children-completed selector over
an event key. The editor renders it for `on_children_completed`,
`on_pr_merged`, and `on_pr_closed` in the transitions grid. Test ids keep the
`<step>-children-completed-*` form and add `<step>-pr-merged-*` and
`<step>-pr-closed-*`. Copy lives in the `workflows` namespace in all locales.

## Compatibility

Steps without the new keys compile to empty action lists, so the handler stops
at gate 2. The lifecycle-prompt automation and the `github_pr_merged`
automation subscriber are separate consumers of the same bus event and are not
modified. GitLab merge requests use a different event and are not handled.
