---
status: active
system: tasks
created: 2026-10-18
owners:
  - kandev
---

# Workflow PR Lifecycle Events Requirements

## Overview

A workflow step can react to the task's linked GitHub pull request being merged
or closed, without an agent turn. Today a merge reaches a task only through an
agent: the per-PR "prompt on merged" automation messages the task's agent, which
must then call `step_complete_kandev`, and the "Pull request merged" automation
starts a separate agent run that archives the task. When the agent session is
busy, parked, or the message is not delivered, the task stays in its review or
acceptance step although the work is merged. Anything that waits for the task
to leave that step stalls, for example a Linear issue watch with
`max_inflight: 1`, which starts the next issue only after the previous task is
completed.

The "Pull request merged" automation deliberately keeps archiving an agent
action ([AC-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001.7](../../office/requirements/automations-pr-merged-trigger.md)).
This capability does not change that automation. It adds a deterministic
workflow transition so a merged task can enter a Done step whose
**Complete task on entry** setting completes it.

## Terminology

- **Linked PR:** A GitHub pull request attached to the task through
  `github_task_prs` and not detached. Multi-repository tasks can have several.
- **PR poller:** The GitHub integration that publishes `github.task_pr.updated`
  when a linked PR's stored data changes.
- **PR event step:** A workflow step that declares `on_pr_merged` or
  `on_pr_closed` actions.

## Requirements

### REQ-TASKS-WORKFLOW-PR-EVENTS-001: Workflow step events for linked PR merge and close

**Intent:** Let a workflow move a task when its deliverable PR is merged or
closed, independent of agent turns and message delivery.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-PR-EVENTS-001.1:** A workflow step shall accept, persist,
  return, export, import, and sync `on_pr_merged` and `on_pr_closed` in its
  `events` object, with the same generic action set and `move_to_step` target
  remapping as `on_children_completed`. Deleting a step shall remove
  `move_to_step` actions that target it from these events.
- **AC-TASKS-WORKFLOW-PR-EVENTS-001.2:** When the PR poller reports a linked PR
  in the merged state and every linked PR of the task is merged, the task's
  current step's `on_pr_merged` actions shall run once for that set of merged
  PRs. While any linked PR is not merged, `on_pr_merged` shall not run.
- **AC-TASKS-WORKFLOW-PR-EVENTS-001.3:** When the PR poller reports a linked PR
  closed without merging, the task's current step's `on_pr_closed` actions shall
  run once for that close.
- **AC-TASKS-WORKFLOW-PR-EVENTS-001.4:** The event shall run without an agent
  turn and without a queued message, including when the task's agent session is
  waiting for input, idle, completed, or has queued messages. When every session
  of the task is terminal, the move shall still apply without reviving a
  session; session-bound entry actions of the destination do not run. A task
  with no session at all shall be skipped and logged. A failed evaluation or
  commit shall be retried a bounded number of times.
- **AC-TASKS-WORKFLOW-PR-EVENTS-001.5:** A PR reported for a task whose current
  step does not declare the event, a PR that is not linked to the task, an
  archived or ephemeral task, and a PR state other than merged or closed shall
  not change the task. The event shall fire only when the merge (the latest
  merge of the linked set) or close happened at or after the task entered its
  current step, so an outcome from an earlier step is never replayed by a later
  poller refresh.
- **AC-TASKS-WORKFLOW-PR-EVENTS-001.6:** A transition caused by these events
  shall apply the normal transition lifecycle (`on_exit`, persisted step change,
  `on_enter`, and task completion when the target step completes tasks on
  entry). The move shall commit only while the task is still on the evaluated
  step; a newer move wins. Its step-transition history row shall use trigger `on_pr_merged` or
  `on_pr_closed` with no human actor identifier.
- **AC-TASKS-WORKFLOW-PR-EVENTS-001.7:** The workflow step editor shall offer
  **When PR Merged** and **When PR Closed** transition selectors with Do nothing,
  Move to next step, Move to previous step, and Move to specific step, next to
  **When Child Tasks Complete**.
- **AC-TASKS-WORKFLOW-PR-EVENTS-001.8:** Steps that declare neither event, the
  per-PR lifecycle prompts, and the "Pull request merged" automation shall
  behave as before.

## Out of scope

- GitLab merge requests and other repository providers.
- A native archive or complete action for the "Pull request merged"
  automation.
- Evaluating PR state when a task enters a step; only poller-reported changes
  run the events.
- Exactly-once delivery: redelivery is deduplicated in process; across restarts
  the step-arrival rule prevents replays of outcomes from earlier steps.
- Per-repository or per-branch filters on the events.

## Implementation plans

- [Workflow PR lifecycle events](../../../plans/workflow-pr-lifecycle-events/plan.md)
