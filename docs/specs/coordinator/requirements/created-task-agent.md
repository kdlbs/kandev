---
id: coordinator-created-task-agent
title: Agent for created tasks
status: draft
system: coordinator
owners:
  - kandev
created: 2026-10-01
last_updated: 2026-10-01
---

# Agent for created tasks Requirements

## Overview

A coordinator proposes tasks and a manager approves them, or an automatic
approval creates them. A created task starts only if an agent profile resolves
for it. The step, the workflow and the workspace may each name one; a workspace
with none of them left every approved task unable to start (live finding F-07).
Each coordinator therefore carries an "Agent for created tasks" setting that
the approval path adds to the task only when nothing else names an agent. This
document owns that setting. The approval behaviour it feeds is
[`AC-COORDINATOR-PROPOSALS-002.13`](proposals.md#req-coordinator-proposals-002-approving)
and `002.16` to `002.18`, and the card line is `005.11` with the
exceptions of `001.11`.

Decided by the owner on 2026-10-01, reversing decision D1 of phase 1. It is
part of `features.coordinator` and is not behind a later phase flag.

## Terminology

- **Agent for created tasks:** the coordinator's agent profile and executor
  profile that the approval path adds to a task it creates, only when the
  step, the workflow and the workspace default name no agent.
- **The coordinator's own pair:** the agent profile and executor profile the
  coordinator's conversation runs on
  ([coordinators](coordinators.md#req-coordinator-coordinators-002-add-edit-and-delete-coordinators)).
- Other terms are defined in the [system README](../README.md#terms).

## Requirements

### REQ-COORDINATOR-CREATED-TASK-AGENT-001: Agent for created tasks

**Intent:** A task a manager approves, or an automatic approval creates, can
start, because the coordinator says which agent and executor it runs with when
the board, the workflow and the workspace do not say.

**User story:** As a workspace manager, I want to choose the agent and executor
for the tasks my coordinator creates, so that an approved task is never a card
that cannot start.

It exists in the phase-1 add form and Identity section as well as in the
guided setup.

#### Acceptance criteria

- **AC-COORDINATOR-CREATED-TASK-AGENT-001.1:** Each coordinator shall have a setting
  named "Agent for created tasks" made of an agent profile and an executor
  profile, stored with the coordinator and returned by the create, list and
  read routes as `task_agent_profile_id` and `task_executor_profile_id`.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.2:** When a manager creates a coordinator,
  by either create route, without a non-empty value for either field, or edits
  either field to an empty value or to null, the system shall refuse the
  request with a 400 error naming that field and store nothing. A field absent
  from an edit shall stay unchanged.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.3:** When a manager sets Agent for created
  tasks, the system shall apply the checks of
  `AC-COORDINATOR-COORDINATORS-002.4` and `002.5` to the new pair: the agent
  profile must exist in the coordinator's workspace (a profile of another
  workspace does not exist for this purpose) and not be CLI passthrough, and
  the executor profile must exist (executor profiles are not workspace-scoped);
  the 400 error shall name `task_agent_profile_id` or `task_executor_profile_id`.
  An edit shall validate a field only when the request carries it with a value
  that differs from the stored one: an absent field and a field sent back
  unchanged shall not be validated, so a stored pair that has since gone
  missing never blocks an edit of another field. When both fields of one
  request are invalid in the same way (both empty or null, or both failing
  the checks), the error shall name `task_agent_profile_id`; a field that is
  empty or null is reported before a field that fails the checks, whichever
  field each is.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.4:** When the system starts with
  coordinators stored before this setting existed, it shall set each such
  coordinator's Agent for created tasks to its own agent profile and executor
  profile, so that no stored coordinator has an empty value, and shall leave
  every non-empty value unchanged. Starting again shall change nothing.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.5:** The add form of
  `AC-COORDINATOR-COORDINATORS-004.3`, the Identity section and the guided
  setup shall each show Agent for created tasks as an agent profile field and
  an executor field with the help text "Tasks you approve start with this
  agent unless their board, workflow or workspace already sets one.", and
  Identity shall show it read-only for a reader.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.6:** While a manager is adding a
  coordinator, whether in the add form or the guided setup, the Agent for
  created tasks agent profile shall pre-fill with the workspace's default
  agent profile when that profile exists and is not CLI passthrough, else with
  the coordinator's own agent profile once chosen; the executor shall pre-fill
  with the coordinator's own executor profile once chosen (the workspace
  holds a default executor, not an executor profile). A pre-filled field shall
  follow later changes to the coordinator's own pair until the manager edits
  that field; a field the manager edited shall keep the manager's value. A
  field counts as edited from the manager's first change to it until the form
  is closed or the setup is left; going Back or Change within the same setup
  keeps it. While the agent profile list has not loaded, the agent field shall
  stay empty, and it shall pre-fill when the list arrives unless it was
  edited. The same holds while the workspace default has not loaded. An
  existing coordinator's page shall show its stored values and
  pre-fill nothing.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.7:** The guided setup's Review shall show
  an "Agent for created tasks" row, owned by Identity, whose value is the agent
  profile name and the executor name joined by ", ", and **Change** on it shall
  return to Who runs it with the values kept. When the setup request is refused
  for either field, the page shall return to Who runs it and show the error
  beside that field (`AC-COORDINATOR-COORDINATORS-008.6`).
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.8:** When a coordinator's
  `task_agent_profile_status` is `missing` or `passthrough`, or its
  `task_executor_profile_status` is `missing`, its Identity section shall say
  so under that field and ask for another, in the same terms as
  `AC-COORDINATOR-COORDINATORS-005.1` (agent `missing`: the profile was
  removed; agent `passthrough`: the profile uses CLI passthrough, which created
  tasks cannot use here; executor `missing`: the executor was removed). These
  statuses shall not affect the copilot or the conversation route: a
  coordinator whose task pair is not `ok` shall still converse.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.9:** When a manager saves a change to
  Agent for created tasks and to no other conversation-affecting field
  (`AC-COORDINATOR-COORDINATORS-002.7`, `002.10`), the system shall keep the
  coordinator's conversation and its configuration revision unchanged. When
  that save changes a stored value, the system shall publish
  `coordinator.updated` after the save commits, as the only event of that
  request (a request that also changes the autonomy publishes the autonomy
  event alone), so open proposal cards
  refetch their "Runs with" line (`AC-COORDINATOR-PROPOSALS-005.11`); a save
  that changes no stored value shall publish nothing.

- **AC-COORDINATOR-CREATED-TASK-AGENT-001.10:** When
  `AC-COORDINATOR-PROPOSALS-002.16` refuses an approval, the proposal's error
  shall be exactly one of: "Agent for created tasks is not usable: the agent
  profile was removed." (agent `missing`), "Agent for created tasks is not
  usable: the agent profile uses CLI passthrough, which created tasks cannot
  use here." (agent `passthrough`), or "Agent for created tasks is not usable:
  the executor was removed." (executor `missing`), choosing the agent's when
  both apply. When the coordinator or its workspace no longer exists, no row
  remains to carry a text and the request shall return 404.
- **AC-COORDINATOR-CREATED-TASK-AGENT-001.11:** The "Runs with" line of
  `AC-COORDINATOR-PROPOSALS-005.11` shall show on a `pending` or `failed`
  proposal card of the create-task kind, and shall be omitted when a read the
  card's own row needs fails (the step, the workflow's default, the workspace
  default, or the coordinator when the chain is empty or the pair is to be
  added) or when the step or the workspace no longer exists; a failed read
  shall not fail the proposal list or read, and a row that needs no failed
  read shall still show its line. When only the agent profile's name cannot be
  read or is unknown, the line shall show the profile id.

## Out of scope

- A choice of agent per proposal, per kind or per board: one pair per
  coordinator.
- Changing when an agent starts. Approval starts an agent only when the phase-2
  start-agent policy allows it and an automatic approval starts none; the owner
  keeps that a separate decision.
- Adding an executor profile when the step, workflow or workspace resolves an
  agent profile but no executor. The task then resolves its executor the way any
  task of its workflow does (the workspace default executor).
- Changing the agent of a task that already exists, and any live preview of the
  resolved agent while a proposal is being edited.
