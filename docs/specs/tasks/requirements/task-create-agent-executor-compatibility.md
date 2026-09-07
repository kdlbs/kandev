---
status: active
system: tasks
created: 2026-09-04
owners:
  - kandev
---
# Task Create Agent Compatibility Recovery Requirements

## Overview

The task-create dialog offers an agent profile and an executor profile. A
remote executor profile (Docker, Sprites, Kubernetes) can run only the agents
whose credentials are configured on that executor profile. When the user
changes the executor after an agent is already selected, the dialog must keep
the user able to start the task with a compatible agent, and it must describe
the real obstacle when it cannot.

The task system owns this contract because it owns task creation and launch
behavior. The credential rule that decides which agent is usable on which
executor belongs to the executor and agent systems; this requirement covers
only how the task-create dialog reacts to that rule.

## Terminology

- **Compatible agent profile:** An enabled agent profile whose agent needs no
  executor credentials, or whose agent has credentials configured on the
  selected executor profile.
- **Workflow-locked agent:** The agent profile that the selected workflow pins
  for new tasks. The user cannot change it inside the dialog.
- **Unavailable selection:** A selected profile that is disabled or blocked
  because dynamic agent routing is off, while another compatible profile exists.
- **Preference order:** The existing pre-selection order for the agent profile:
  the last-used profile, then the workspace default, then the first compatible
  profile.

## Requirements

### REQ-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001: Agent selection recovery when the executor changes

**Intent:** A user who switches the executor keeps a usable agent selected
whenever one exists, and otherwise sees which agent, workflow, and executor
combination blocks the launch. The dialog never claims that no compatible agent
exists while one does.

**User story:** As a user creating a task, I want the dialog to keep a usable
agent selected when I switch to a remote executor, so that I can start the task
without reopening the dialog or guessing which credential is missing.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.1:** When an executor profile
  is selected and at least one agent profile is compatible with it, the dialog
  shall show the agent selector with the compatible profiles as its options.
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.2:** When the selected agent
  profile is not compatible with the selected executor profile, the workflow
  does not lock the agent, and at least one compatible profile exists, the
  dialog shall replace the selection with a compatible profile chosen by the
  preference order. Until the replacement applies, an unavailable selection
  shall show a truthful pending state and keep the start action disabled.
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.3:** When the selected agent
  profile is compatible with a newly selected executor profile, the dialog
  shall keep that selection.
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.4:** When no agent profile is
  compatible with the selected executor profile, the dialog shall show a
  message stating that no compatible agent profile exists for that executor
  profile, with a link to that executor profile's credential settings, and
  shall keep the start action disabled.
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.5:** When the workflow locks an
  enabled agent profile that is not compatible with the selected executor
  profile, the dialog shall show a message that names the workflow, the agent
  profile, and the executor profile, with the same credential link, and shall
  keep the start action disabled. This message takes precedence over the
  message in 001.4 when no other profile is compatible either, because the
  user cannot change the locked agent. A workflow that locks a disabled
  profile keeps the behavior in
  [Disable an Agent Profile](../../agents/requirements/profile-disable.md).
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.6:** Except for the disabled
  workflow-locked profile behavior in 001.5, the dialog shall not state that no
  compatible agent profile exists while at least one compatible profile exists.
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.7:** When the start action is
  disabled because of agent compatibility, its explanation shall match the
  shown state: the no-compatible message for the state in 001.4, a message
  naming the agent profile and executor profile for the state in 001.5, or an
  unavailable-selection message while an automatic replacement is pending.
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.8:** An automatic replacement
  under 001.2 shall not change the user's stored last-used agent profile. Only a
  manual selection or a successful task creation records that preference.
- **AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.9:** Desktop and phone
  viewports shall show the same messages, link, and recovery behavior, and the
  message shall wrap inside the dialog without horizontal document overflow.

## Out of scope

- Changing which agents count as compatible with an executor profile. The
  credential rule stays with the executor and agent systems.
- Marking incompatible executor profiles inside the executor selector.
- The additional-session, handoff, and quick-chat dialogs, which have their own
  selection contracts.
- Changing the executor default policy in
  [Task Create Executor Default](task-create-executor-default.md).
