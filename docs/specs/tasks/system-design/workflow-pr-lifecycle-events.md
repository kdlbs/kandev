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
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.2` | Evaluation; Idempotency and replay |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.3` | Evaluation; Idempotency and replay |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.4` | Session selection; Retries |
| `AC-TASKS-WORKFLOW-PR-EVENTS-001.5` | Evaluation; Idempotency and replay |
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

`Service.handleTaskPRUpdated` calls `handleTaskPRWorkflowEvent` after starting
CI automation. `processTaskPRWorkflowEvent` evaluates synchronously and stops
at the first gate that does not pass:

1. Map the PR state: `merged` to `on_pr_merged`, `closed` to `on_pr_closed`,
   anything else ignored. Require a task ID, owner, repository, and a positive
   PR number, matching the `github_pr_merged` automation subscriber.
2. Load the task. Skip archived and ephemeral tasks and tasks without a workflow
   step. Load the current step and skip when it declares no actions for the
   trigger.
3. List the task's attached PRs (`ListTaskPRs`, which excludes detached rows).
   Skip when the reported PR is not among them, or when its stored row no
   longer reports the event's state and merge or close time (compared to the
   second), so a delayed or retried snapshot of a reopened PR does not fire.
4. `on_pr_merged` requires every linked PR to be merged; its outcome time is
   the latest `merged_at`. `on_pr_closed` uses the reported PR and its
   `closed_at`. A missing time counts as now.
5. Step arrival: read the newest `task_step_transitions` row. When it is the
   entry into the current step, skip outcomes older than its `occurred_at`. A
   missing or older-schema row means the arrival is unknown and does not skip.
6. Select a session, take the per-operation lock, check the operation ledger,
   and call `Engine.HandleTrigger` with `EvaluateOnly` and
   `DeferOperationMark`, as `processOnChildrenCompleted` does.

## Idempotency and replay

When the outcome timestamp is known, the operation id is deterministic:

- merged: `on_pr_merged:<task>:<sha256 of the sorted owner/repo#number keys of
  all linked PRs>`;
- closed: `on_pr_closed:<task>:<owner/repo#number>:<closed_at>`, so a reopened
  and closed again PR produces a new id.

For a missing close timestamp, the explicit observed-now fallback also supplies
the close key's timestamp. Such incomplete observations cannot identify a
distinct close reliably; they do not gain the known-timestamp deduplication
guarantee. The normal GitHub REST, CLI, and batched GraphQL reads request and
persist close timestamps.

The id is marked applied after a non-transition evaluation, after a committed
transition, and after a compare-and-swap miss. The operation ledger is
in-memory, so it only deduplicates redelivery within one process. Durable
protection comes from step 5: the poller republishes a merged or closed PR
whenever other tracked fields change, and after a restart such a refresh can
fire only while the task is still on the step it occupied when the outcome
happened. GitHub timestamps and the local ledger clock are compared directly;
clock skew of a few seconds around a step entry can misclassify an outcome.

## Session selection

Engine state and step history are keyed by session. The handler uses the
active session (`CREATED`, `STARTING`, `RUNNING`, `WAITING_FOR_INPUT`) when one
exists, otherwise the newest non-terminal session (the primary first). Such a
live session gets the full lifecycle. When every session is terminal, the
newest one (the primary first) is used only to record the move, through the
session-independent guarded lifecycle (`applyGuardedTransitionLifecycle`): the
compare-and-swap commit, history, task completion, and engine-owned entry
actions run, and no session is revived, so session-bound entry actions such as
`auto_start_agent` do not run. Agent auto-start is also skipped elsewhere for
tasks whose PRs are terminal. No session is created; a task without sessions
is skipped with a debug log.

A failed active-session read may still use a healthy session-list fallback.
Without a usable fallback, actual read failures propagate to bounded retry.
The repository's missing-active-session result is expected when only terminal
sessions exist; it does not prevent that fallback or turn a genuinely empty
task into a storage failure.

## Transition and history

With a live session the transition goes through
`applyEngineTransitionWithCommitMode` with `transitionLifecycleWithOnEnter`, the
same lifecycle as `on_children_completed`, but with a compare-and-swap commit:
`applyTransitionIfAtStepRaw` writes the move only while the task is still on
the evaluated source step and its observed arrival ledger identity, inside the
admission transaction. The task-row lock covers both comparisons. Departing
and returning to the same step supersedes an earlier evaluation, just as moving
to another step does. The arrival fence is scoped to that task and source step,
so destination entry work retains its own admission rules. A superseded PR
event reports no transition. The commit context carries the step-entry allocation
(`transitionContext`), then the handler publishes `task.updated`, clears the
session review status, and pulls feeder work, as `ApplyTransition` does.

The history mapping adds `StepTransitionTriggerPRMerged` (`on_pr_merged`) and
`StepTransitionTriggerPRClosed` (`on_pr_closed`). The row is written with a nil
actor id, so it is a system move. The task-step transition ledger records the
move as `engine_transition` with a system actor, like `on_children_completed`.
As with `on_children_completed`, a live session that is still `RUNNING` when
the transition commits is set to `WAITING_FOR_INPUT`.

## Retries

The poller publishes an event only when PR data changes. When evaluation or the
commit fails with an error (not a skip or a compare-and-swap miss),
`scheduleTaskPRWorkflowRetry` re-runs the same PR snapshot after 5 s, 30 s, and
2 min on the CI automation worker lifecycle, so service shutdown cancels it.
The terminal-session lifecycle also propagates a failed CAS write; only a
successful no-match CAS is an intentional skip. A storage failure is not marked
as an applied operation.

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
