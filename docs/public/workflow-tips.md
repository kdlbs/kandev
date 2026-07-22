---
title: "Workflow Tips"
description: "Choose, configure, and troubleshoot Kandev task workflows."
---

# Workflows

A workflow is an ordered set of steps for tasks in one workspace. A workflow can be a plain board, or its step events can start an agent, change agent mode, and move a task after a user or agent turn.

Configure Kanban workflows in **Settings → Workspaces → select a workspace → Workflows**. You need a workspace first; steps which start agents also need a healthy agent profile and a usable executor profile. Use a template when its prompts fit your process. Use **Custom** when you only need columns or want to build the automation yourself.

Workflow prompts run with the selected executor's filesystem, credentials, and network access. Do not place tokens in prompt text, and give the agent only the access that workflow needs. Workflow settings use Kandev's current local backend trust boundary; protect any network-exposed installation as described in [Run as a Service](run-as-a-service.md).

> Workflow definitions can be copied between workspaces with [Workflow Import / Export](workflow-import-export.md), or reconciled from a repository with [Workflow Sync](workflow-sync.md).

## Built-in Kanban templates

The template prompts are product behavior, not merely sample text. Review them before using a template in a repository with strict Git, test, or deployment rules. Kandev currently presents these five Kanban templates.

### Kanban

**Backlog → In Progress → Review → Done**

- A normal new task starts in **In Progress**. Its agent starts automatically.
- A user message in **Backlog** moves the task to **In Progress** before the message is delivered.
- Completion of an agent turn in either **Backlog** or **In Progress** moves the task to **Review**. Backlog also has the user-turn-start transition above, so which route runs depends on how work is started there.
- A user message in **Review** moves the task back to **In Progress**. A message in **Done** also reopens it in **In Progress**.

Choose this for short implementation work with a simple run-and-review loop.

### Plan & Build

**Todo → Plan → Implementation → Done**

- A normal new task starts in **Plan**. Kandev enables plan mode and starts the agent with a prompt that asks it to save a task plan and wait for review.
- Leaving **Plan** disables plan mode.
- Entering **Implementation** starts the agent with a prompt that retrieves the saved plan and implements it.
- The template does **not** define automatic transitions between these steps. Move the task to Implementation and Done when ready.

Choose this when a human should approve or edit a plan before implementation.

### Architecture

**Ideas → Planning → Review → Approved**

- A normal new task starts in **Planning**. The agent starts in plan mode and is instructed to produce design, not code.
- **Review** enables plan mode. A user message there moves the task back to Planning for another design turn.
- Other transitions are manual; **Approved** does not launch implementation.

Choose this for designs, RFCs, and technical decisions that will be implemented elsewhere.

### Feature Dev

**Todo → Spec → Work → Review → QA → PR → CI Fixup → Done**

- A normal new task starts in **Spec**, in plan mode.
- Entering **Work**, **Review**, **QA**, **PR**, or **CI Fixup** starts the step prompt automatically. Review also resets agent context first.
- The prompts cover planning, TDD-oriented implementation, diff review, QA, draft-PR creation, and CI repair respectively. Their success depends on repository tools and credentials such as the test runner, Git remote access, `gh`, and CI access.
- The template does **not** auto-advance between phases. Move the task after checking the current phase's result.

Choose this for a deliberate multi-pass delivery process. It is excessive for small chores.

### PR Review

**Waiting → Review → Done**

- A normal new task starts in **Waiting**. Sending a message moves it to Review.
- Entering **Review** starts an agent. The current prompt expects a GitHub PR number or URL, an authenticated `gh` CLI, and a usable `origin` remote. It reviews only added or modified diff lines and reports **BLOCKER** and **SUGGESTION** findings.
- Moving to Done is manual. The template does not publish review comments by itself.

Choose this for a local first-pass review. For repository-provider watch automation, use the relevant integration instead.

## Build a custom workflow

Choose **Add Workflow**, give it a name, select **Custom**, and save it. Expand each step to edit its behavior. Reorder steps by dragging them; transition actions that say “next” or “previous” follow the saved position order.

Workflow-level settings include the name and default agent profile. A step can override that profile; switching profiles creates a different session with fresh context. A step also has these controls:

| Control | Behavior |
|---------|----------|
| Name and color | Board label and presentation. Color is stored as a CSS utility class. |
| Prompt | Step-specific agent prompt. `{{task_prompt}}` inserts the task description. |
| Start step | Preferred initial step. The editor keeps at most one. If none is set, task creation falls back to the first step by position. |
| Auto-start agent | Adds `auto_start_agent` to `on_enter`. It still needs a valid agent and executor configuration. |
| Plan mode | Adds `enable_plan_mode` on entry. Add the matching disable behavior on completion or exit when later steps should edit files. |
| Reset agent context | Starts the step with fresh conversation context. It is redundant when the step changes agent profile. |
| Allow manual move | Allows board drag/drop into the step. It is a product-UI rule, not a security boundary for API clients. |
| Show in command panel | Includes tasks in this step in the command panel. |
| Auto-archive | Archives eligible tasks after the configured number of hours. `0` disables it; the background sweep runs every five minutes and uses task `updated_at`, so timing is approximate. |
| Wait for agent completion signal | With an `on_turn_complete` transition, waits for the agent to call `step_complete_kandev`. A halt without the signal leaves the task on the current step; retry or reconnect the agent, or move the task through the normal workflow UI. Without this setting, a normal turn end counts as completion. Default is off. |
| WIP limit | Maximum active, non-archived, non-ephemeral tasks in the step. `0` means unlimited. A full target rejects manual and automated moves. |
| Pull from | Optional feeder step. When a WIP-limited step is vacated, Kandev moves candidates from the feeder until capacity is full. Self-references, cross-workflow references, and pull cycles are rejected. |

Pull candidates are selected by board position, then priority, creation time, and ID. A candidate that cannot be moved is skipped. Pulling only runs when the receiving step has both a positive WIP limit and a feeder.

## Events and actions

The standard Kanban editor exposes these events:

| Event | When it runs | Editor actions |
|-------|--------------|----------------|
| `on_enter` | A task enters a step through normal step-entry processing. | Enable plan mode, auto-start agent, reset context. |
| `on_turn_start` | A user sends a message. The transition happens before that message is delivered. | Move next, previous, or to a selected step. |
| `on_turn_complete` | An agent turn finishes, unless a clarification is still pending or explicit completion is required but absent. | Move next, previous, or to a selected step; disable plan mode. |
| `on_exit` | A task leaves a step. | Disable plan mode. |

The portable format also recognizes `set_session_mode`, `clear_decisions`, `queue_run`, and `queue_run_for_each_participant` in `on_enter`; these are advanced/runtime-dependent actions and most are not offered by the Kanban editor. Office event triggers have a broader model, but do not round-trip through Kanban import/export. See the exact boundary in [Workflow Import / Export](workflow-import-export.md).

Keep one transition action per event. A “next” action on the last step or “previous” on the first has nowhere to go and leaves the task in place. WIP rejection, a missing target step, a failed agent launch, or missing credentials can also prevent the intended progression; inspect the task/session error and backend logs before changing the workflow.

## Safe authoring pattern

1. Start with manual transitions and verify prompts in a disposable task.
2. Add `auto_start_agent` only to steps that always have an effective agent profile.
3. Add turn-complete transitions after the prompt has an unambiguous stop condition.
4. Enable the explicit completion signal for agents that can discover and call `step_complete_kandev`; otherwise the step can wait indefinitely. If the tool is not already visible, the agent should search the active tool catalog for its canonical name.
5. Add WIP limits before pull rules, then test a full target and a vacated slot.
6. Export the workflow before a large edit. Workflow deletion is permanent; when it contains tasks, the UI asks you to migrate them or archive them.

## Repository instructions and multiple repositories

Step prompts are combined with the selected agent and the checked-out repository. Keep repository-specific instructions such as `AGENTS.md`, `CLAUDE.md`, skills, test commands, and MCP configuration in that repository. Kandev does not make one repository's agent rules automatically authoritative for another.

A task may contain several repositories, but a workflow step is not bound to one repository. The agent session receives the task workspace and its repository set. Prompts should name the intended repository when the phase is repository-specific, and Git operations must be scoped per repository. See [Git Operations](git-operations.md).

## Troubleshooting

- **Task starts in the wrong column:** confirm exactly one Start step, save the workflow, and check whether the creator supplied an explicit `workflow_step_id`.
- **Agent does not start:** verify the effective workflow/step agent profile, its health, executor profile, repository access, and the `auto_start_agent` entry action.
- **Task stays after a turn:** check for an absent transition, a pending clarification, the explicit-completion toggle, a full WIP target, or an invalid target left by an older definition.
- **Task cannot be dragged:** the destination may disallow manual moves, be at its WIP limit, or the task may have a starting/running session.
- **Auto-archive looks late:** the sweep cadence is five minutes and task updates extend the age check.
- **Synced workflow is read-only:** edit its repository definition and run Sync now, or remove the sync configuration to release all synced workflows as editable manual workflows.

Related guides: [Workflow Import / Export](workflow-import-export.md), [Workflow Sync](workflow-sync.md), [Git Operations](git-operations.md), and [Executors](executors.md).
