---
title: "Tasks and Workflows"
description: "Create scoped tasks, configure workflow behavior, use plans, and manage the task lifecycle."
---

# Tasks and Workflows

A task is the work to deliver. A workflow is the sequence of steps it follows. Use a task for the outcome and a workflow for the review process.

## Quick path

1. Add a repository to a workspace.
2. Create a task with a clear outcome, a compatible agent, and an executor.
3. Start the agent, review its changes, and move the task through the human gate.

## Understand the model

| Concept         | What it controls                                                                                                       |
| --------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Workspace       | The scope containing repositories, workflows, tasks, integrations, and workspace defaults.                             |
| Workflow        | An ordered set of steps plus the rules that run when a task or agent turn reaches an event.                            |
| Workflow step   | The task's current process position, such as Backlog, Work, Review, or Done.                                           |
| Task            | The title, prompt, workflow position, repository attachments, sessions, and one shared plan.                           |
| Task repository | A repository, base branch, and optional checkout branch attached to a task. A task can have more than one.             |
| Session         | One agent conversation attached to a task. Several sessions can share the same task environment.                       |
| Plan            | The task's single editable Markdown plan, with version history. Consecutive writes can be coalesced into one revision. |

Workflow position and runtime state are different. Moving a card changes its workflow step; it does not prove that an agent ran, code was committed, review passed, or a pull request merged.

## Prepare a workspace

A new workspace created from **Settings → Workspaces** automatically receives a **Kanban** workflow
with the built-in Kanban steps, so it can accept tasks immediately.

1. Open **Settings → Workspaces** and select **Add Workspace**.
2. Enter the required workspace name.
3. Open the workspace's **Repositories** page and add existing local repositories the workspace needs. You can also initialize a new empty repository while creating a task. Remote URLs are not registered on this page; enter them through **New Task → Remote**. The same page's **Repository sets** section groups repositories you routinely use together, so one action fills the task form with all of them; see [Repository sets](#repository-sets).
4. Open its **Workflows** page to review the default **Kanban** workflow. Create, import, or synchronize another workflow when the workspace needs a different process.
5. On **Workspace Settings**, optionally choose a **Default Executor** and **Default Agent Profile**. Both default to **No default** unless configured.

The initial database bootstrap can include a **Default Workspace** and a **Development** workflow.
Later user-created workspaces receive **Kanban** instead; they do not inherit other workflows or
settings from the default workspace.

## Create a task

Use **New Task** in the sidebar. In an open task, the **Task** split button also opens task creation.

<DocsVideo
  webm="./media/feature-guides/task-create.webm"
  mp4="./media/feature-guides/task-create.mp4"
  poster="./media/feature-guides/task-create.webp"
  title="Create a task"
  caption="A focused task is entered while its repository, agent profile, worktree isolation, and start mode remain visible for review."
/>

1. When the title field is shown, enter a concise title of up to 60 characters. Titles prefilled from
   a remote pull request, issue, or merge request are shortened with an ellipsis when needed; the
   detailed context belongs in the description. If **Settings → General → Task Actions → Agent-generated
   task titles** is enabled, the New Task dialog hides this field, requires a nonempty prompt, and uses
   the prompt's first six words as a provisional title while the first eligible agent session chooses
   the final title. The empty-description Plan Mode exception applies only when this setting is disabled.
2. Select the workspace and workflow when Kandev cannot infer them. A regular non-ephemeral task must belong to a workflow.
3. Select a source:

   | Source     | Use it for                                        | Important behavior                                                                                                                                                                                                                                                                                                                                                                                                   |
   | ---------- | ------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
   | **Repo**   | A configured, discovered, or new local repository | Select a named branch policy or a raw base branch for each repository row. A policy creates a fresh branch from its saved base and uses its branch template. For a single-row new task, **Create new repository** initializes an empty `main` repository with one empty initial commit in a parent folder you choose. Add more rows for a multi-repository task.                                                                                                                                                                                                  |
   | **Remote** | A remote repository                               | Search configured GitHub, GitLab, or Azure DevOps repositories, or paste a supported URL. A pasted URL stays editable until you press Enter; then select the branch. Anonymous, credential-free reads include public GitHub repository branches, pull requests, and issues, plus public `gitlab.com` branch discovery. Private resources and authenticated browse/write features require valid provider credentials. |
   | **None**   | Planning, research, or work outside Git           | Use a scratch workspace or an optional folder on the Kandev host. Git worktree execution and repository-aware Changes, branch, and pull-request features are unavailable.                                                                                                                                                                                                                                            |

4. Select a compatible executor profile and agent profile. A workflow default agent profile locks the task-level agent selector. Executor and agent compatibility is validated before launch.
5. Enter the initial description. In the **New Task** dialog, an empty description changes the primary action to **Start Plan Mode**; the other dialog actions require a description. Agent-facing task MCP has different empty-description rules. When agent-generated task titles are enabled, every task and subtask action requires a nonempty prompt; the empty-description Plan Mode exception is disabled. A nonempty description exposes the standard split actions.
6. Choose the applicable action:
   - **Start Plan Mode** is the primary empty-description action and creates the task through the plan-mode path.
   - **Start task** requires a nonempty description, creates the task, and starts its agent. This path starts in the first positional step whose entry actions include **Auto-start agent**, falling back to **Start step** when the workflow automates no step.
   - **Start task in plan mode** requires a nonempty description and starts the agent with plan mode enabled. This path starts in the first positional workflow step, even if another step is marked **Start step**.
   - **Create without starting agent** requires a nonempty description and starts in **Start step**. A structured ACP profile prepares the session/workspace without starting an agent turn. Passthrough/TUI is an exception: the backend launches it immediately so its native PTY exists.

   On mobile, the two non-primary actions are separate buttons labeled **Plan mode** and **Create only**; they have the same plan-mode and create-without-agent behavior.

### Branch policies

Manage named branch policies in **Settings → Workspaces → _workspace_ → Repositories**. A policy
stores a base branch, a branch-name template, and a pull-request target for one repository. The
task picker shows policies before raw branches. A selected policy starts a fresh branch. A raw branch
continues to open the existing branch. Each policy row has an information icon for its saved values.
The base branch is the starting point. The pull-request target is the merge destination.

When you create a task, Kandev saves the selected policy values on the task repository. Later edits
or deletion of the policy do not change the task. Kandev's pull-request dialog uses the saved target
by default. You can change it before creation. Kandev also adds the saved target to the agent's task
context. The instruction tells the agent to pass the target explicitly to its provider CLI.
Policies are not offered in **Quick Chat**, **Remote**, **Add Sources**, or **Add Branch** flows.

Kandev remembers draft or recently used repository, branch, executor, and profile choices. Review the restored values before submitting, especially after changing workspace.

When the selected profile is dynamic, the task keeps one logical profile and one
session tab while Kandev chooses a concrete candidate in the configured order.
Provider errors before a result may move execution to the next configured
candidate. Kandev does not switch candidates after an ambiguous started turn.
If the route has no eligible candidate, wait for the current turn to settle and
use the session's **Retry current agent** or **Try next agent** recovery action.

Creating a repository is available only in an unlocked, single-repository **New Task** form. Kandev rejects an existing target path, creates one empty initial commit but no project files, registers the repository in the workspace, and switches the task to a direct **Local** executor profile. If no direct Local profile is available, repository creation stays disabled. Add more repository rows only after selecting existing repositories; empty multi-repository worktrees are not supported.

### Work with an empty remote repository

An existing local checkout or a repository selected from **Remote** can point to a remote with no refs. Kandev creates a local empty baseline so a normal **Worktree** task can start. The baseline contains no README, license, `.gitignore`, or other project files.

Task launch, resume, and worktree recovery do not write to the remote. When the work is ready, use the existing **Changes** action to **Push** or **Create pull request**. Kandev publishes the selected base branch first, then the task branch, with the task runtime's Git credentials. Read or clone access alone is not enough to publish.

If another person or tool initializes the remote before the first publication, Kandev stops without overwriting that history. Reconcile the remote and local task branch, then retry. If the base branch was published but the task branch failed, the task branch remains local and **Push** can be retried. On phones, use the same actions from the touch-sized **Changes** menu.

> **Local changes:** creating a fresh local branch can discard dirty files only after explicit consent. Save or commit important work before approving it.

<details>
<summary>Advanced task creation: agent-created tasks, long transcripts, multiple sources, and attachments</summary>

### Let the agent name new tasks

Open **Settings → General → Task Actions → Agent-generated task titles** and choose **Save changes**.
The setting is enabled by default; an explicitly saved **off** value remains off. When enabled, new task
and subtask dialogs use the prompt as the source of the title: the prompt must contain text, and Kandev
immediately displays its first six normalized words as a provisional title. The first eligible task-mode
session to launch atomically claims the handoff, receives the `set_task_title_kandev` MCP tool, and is
instructed to call it before doing any other work. Ask for a short title phrase targeting about six words
in sentence case rather than a sentence or progress update. Later sessions do not receive the instruction
or tool, even if the owner fails before renaming the task. If the agent never renames the task, the
provisional title remains usable and can still be edited by a person.

The setting affects only new task/subtask creation. Existing task edits keep the title field, and
sessions for tasks created while the setting was disabled receive neither this instruction nor the
tool. Config and Office sessions never receive the title tool.

### Choose the profile for tasks created by agents

Open **Settings → General → Task Actions → Profile for Tasks Created by Agents** to choose which agent profile Kandev assigns when an agent calls `create_task_kandev` without choosing `agent_profile_id`. The preference covers new tasks and subtasks, and it also controls the effective model, mode, and dynamic options used by the first session:

- **Creating session profile** is useful when follow-up work needs the same live setup. For a session-bound task-mode call, Kandev uses the verified creating session's profile and its effective model, mode, and dynamic options, including changes made during that session. A workflow launch profile wins first. When no workflow profile wins, the creating session profile is used. This option can reuse a more expensive setup.
- **Workspace default profile** is useful when you want agent-created tasks to follow a consistent workspace cost policy. It skips the creating session and source, parent, or current task profiles. Kandev uses the workflow launch profile first, then the **Default Agent Profile** from the workspace that will own the new task. It does not copy the creating session's model, mode, or dynamic options. If neither source supplies a profile, task creation fails.

Select an option, then choose **Save changes**. Workflow-selected profiles always win when the new task lands on a workflow step. Away from a workflow step, an explicit `agent_profile_id` wins and prevents creator-session runtime inheritance. The only affected Kandev MCP tool is `create_task_kandev`. `spawn_session_kandev` adds a session to the current task, so it does not use this preference. Tasks you create in the UI are not affected.

External MCP calls have no verified creating session. With **Creating session profile**, those calls keep the compatibility fallback to the parent task when one exists, then workflow and target-workspace defaults. The preference applies across workspaces, but **Workspace default profile** resolves the default from each new task's target workspace. A resolved profile and runtime seed are stored even when `start_agent=false`, so a later manual start uses the same decision.

### Navigate long chat transcripts

When your latest prompt has fully left the transcript viewport, **Scroll to
last prompt** appears beside the Chat share control. Select it to return to
that prompt; it hides again after any part of the prompt is back in view. Its
arrow points the direction the transcript will actually scroll: upward once
you've scrolled further down past your prompt, or downward if you've scrolled
back up above it while browsing earlier history. **Scroll to start of
transcript** appears when the first prompt is no longer fully visible. You can
show or hide each action independently in **Settings → General → Task
Actions → Transcript Navigation**.

For a compact reminder while you read later replies, enable **Show anchored
prompt bar** in the same settings section. On desktop, it pins a shortened
copy of your latest prompt below the session tabs once you've scrolled past
it further down the transcript. It stays hidden while you're browsing earlier
  history above your prompt, even though the prompt itself is out of view.
use **Scroll to last prompt** to jump back to it instead. Expand the bar for
longer prompts, or use its scroll action to return to the full prompt; the
expanded view is capped at 40% of the transcript panel's height so it stays
proportionate whether the panel is a full-screen view or a small embedded
split. The anchored bar is desktop-only; phones use the scroll-to-last-prompt
action instead. Both scroll actions keep the transcript at your requested
position even if the agent streams new replies while the scroll is still in
progress.

### Multiple repositories

A task can include several local or remote repository rows. Multi-repository creation supports **Worktree**, **Local Docker**, **Kubernetes**, **SSH**, and **Sprites**. Local/Local PC creation remains unavailable until its initial-launch path can materialize sibling repositories, and Remote Docker is not implemented. Public GitHub and GitLab repositories can be cloned and fetched anonymously. Private repositories and authenticated browse/write features need credentials that can access the selected base branch.

If Kandev cannot resolve a pasted remote URL or its branch, the repository row keeps the URL and shows the provider error. Use **Retry** after correcting the URL or when a transient provider failure has cleared.

Changes and review are scoped by repository. State the expected deliverable, base branch, and pull-request target for every attachment. See [Coordinate work](coordination.md) for adding branches after creation and splitting multi-repository work.

</details>

### Repository sets

<details>
<summary>Repository set details</summary>

A **repository set** is a named, reusable group of a workspace's repositories: define **full-stack**
once, then fill the repository picker with all of its repositories in a single action every time that
combination of repositories is the one you need.

A set holds repositories only. Branches stay a per-task decision, so applying a set leaves each row's
branch to the picker's normal defaulting, and you review and adjust branches exactly as when adding
rows by hand.

Define a set in either place:

- **Settings → Workspaces → _workspace_ → Repositories**, in the **Repository sets** section: create,
  rename, edit which repositories belong, reorder them, and delete.
- **New Task → Sets → Save as set**, which captures the repositories currently selected in the form
  without disturbing the task you are creating.

Apply one from the **Sets** control beside **add repository** in **New Task** and **New subtask**.
Applying a set adds one row per repository, in the set's order. It is additive and repeatable:

- a repository already in the form is skipped, so applying the same set twice changes nothing and two
  overlapping sets give you the union;
- rows you already configured are never discarded or reordered;
- a repository that has since been removed from the workspace is skipped, and the dialog says how
  many were skipped.

Applying a set only fills the form. Nothing is saved until you create the task, so the repositories
the task ends up with are whatever the form holds when you submit.

Sets are also available over the API for scripted setup:

```text
GET    /api/v1/workspaces/:id/repository-sets
POST   /api/v1/workspaces/:id/repository-sets   {"name","description","repository_ids"}
GET    /api/v1/repository-sets/:id
PATCH  /api/v1/repository-sets/:id              any of name, description, repository_ids
DELETE /api/v1/repository-sets/:id
```

`repository_ids` is ordered and is the order a set fills the picker. A supplied `repository_ids`
replaces the whole membership list, which is also how you reorder one; omit the field to leave
membership untouched. The same five operations exist as `repository_set.list|create|get|update|delete`
WebSocket actions, and `repository_set.created|updated|deleted` notifications keep every open client
current. See [WebSocket API](websocket-api.md).

Sets are workspace-scoped and shared: everyone who can see the workspace sees and can apply its sets.
A set name is unique within its workspace, compared case-insensitively. Deleting a set removes the
grouping only, never a repository; deleting a repository removes it from every set and leaves the sets
themselves in place. Sets are not offered in **Remote** or **None** source mode. On an executor that
cannot run a multi-repository task the control still works; the executor picker marks that profile
unavailable once several repositories are selected, exactly as when you add the rows by hand.

</details>

### Add sources to an existing task

<details>
<summary>Adding sources details</summary>

For a non-archived, repository-backed task, open the **Files** panel and choose **Workspace actions → Add Repositories to workspace**. Use **Add repository** to choose a workspace repository, an existing local Git checkout, or a provider-backed/pasted remote URL. The workspace option shares task creation's saved/discovered selector, refresh, and create-repository actions. Use **Add folder** for an arbitrary local folder when the executor supports it. Add one or more rows in a single submission. Repository rows choose a base branch once; the flow does not ask for a second checkout branch. Local/Local PC uses the user-owned repository's current checkout and never switches it. The whole mixed batch succeeds or fails together.

The task must be idle: Kandev disables the action while a turn or tool call is active, and rejects a race without changing the task. Desktop opens a dialog; phones open the same flow in a full-height drawer. On success, repositories appear in Files and repository-aware Changes, branch, editor, and pull-request surfaces; folders are Files-only.

Before submission, the dialog or drawer summarizes the effect on the workspace, session context,
and running processes. **Cancel** or closing the surface sends no request and changes nothing. A
submitted batch remains all-or-nothing.

If adding a source promotes a Worktree or Local/Local PC workspace from one repository directory to
the task root, Kandev restarts the idle agent in the new root. Existing files, Git changes, task
state, messages, plan, attached sources, model, and mode remain. Native cross-directory resume is
retained where supported; otherwise Kandev starts a fresh provider session and supplies recorded
conversation context with the next prompt. Provider-private context not recorded by Kandev may not
carry over. The intentional restart is not shown as a previous agent error.

The host rebind stops open task terminals, dev servers, the task editor server, and other
agentctl-managed workspace processes, so save unsaved work and restart those processes afterward.
Local Docker, Kubernetes, SSH, and Sprites attach repository siblings to the current remote workspace and rescan
without restarting the agent or changing its CWD.

Folders are live host paths and are available only to **Local/Local PC** and **Worktree** tasks. Repository sources are supported for **Worktree**, **Local/Local PC**, **Local Docker**, **Kubernetes**, **SSH**, and **Sprites**. Local Git rows need a cloneable origin on Docker, Kubernetes, SSH, and Sprites; Worktree and Local/Local PC can use the host repository directly. See [Executors](executors.md#workspace-sources) and [Coordinate work](coordination.md#add-sources-after-creation) for runtime limits and recovery behavior.

### Attachments and local-change consent

The task prompt supports image, audio, and resource attachments. Kandev accepts at most 10 files per submission, with a 100 MiB raw limit per file and a 100 MiB raw aggregate limit. Files are uploaded over authenticated HTTP before the task or message is submitted, so the task-create JSON and WebSocket frames carry attachment descriptors rather than base64 file contents. An upload that is still in progress or has failed must finish or be retried before the prompt can be sent. Removing a staged attachment discards its private upload; unclaimed uploads expire automatically after 24 hours. This prompt-attachment limit does not change the separate 10 MB task-document upload contract.

Creating a fresh local branch is available only with the local executor. If the checkout is dirty, Kandev lists the affected paths and requires explicit consent before discarding those local changes. If another path becomes dirty after the warning, creation fails with a conflict and asks for consent again. Save or commit work before approving this operation.

</details>

## Start a task

A task created with **Create without starting agent** opens in a prepared workbench. Review its repository, branch, executor, profile, and initial prompt, then select **Start agent**. The run stays in the task conversation, where environment preparation, tool calls, permission requests, and the final response remain inspectable.

<DocsVideo
  webm="./media/feature-guides/task-start-agent.webm"
  mp4="./media/feature-guides/task-start-agent.mp4"
  poster="./media/feature-guides/task-start-agent.webp"
  title="Start an agent on a prepared task"
  caption="A prepared task starts its selected agent in the workbench and reaches a completed response."
/>

If the selected profile is unhealthy or incompatible with the executor, fix that configuration before launch. Starting an agent is separate from moving the task through its workflow; entry actions and turn-complete transitions can move or restart work afterward.

By default, a running session keeps the coarse **Generating** state and queues
another message even if Kandev detects background work. Operators can opt into
the high-risk **Claude background prompt handoff** feature toggle for controlled
testing. With that experiment enabled, a Claude Code session shows **Working in
background** after its foreground yields while a recognized async subagent,
`run_in_background` shell, or Monitor remains active. A follow-up is then sent
immediately and the child may continue streaming. Other providers and
foreground-generating Claude turns retain the coarse queueing behavior.

### Prevent auto-start on open

Under **Settings → General → Task actions**, the **Prevent auto-start on open**
preference is off by default. When enabled, opening a task never launches or
resumes its agent on its own; it shows the **Start agent** button instead. The
preference applies in two situations:

- **Opening a task in the final step of its workflow.** The task opens with a
  prepared session and the agent stays stopped until you select **Start agent**.
  Opening the same task with the preference off keeps the workflow step's
  normal auto-start behavior.
- **Opening a task whose agent was stopped by a Kandev restart.** The session
  is recovered and shown stopped instead of being resumed automatically. Select
  **Start agent** to resume it.

The preference only gates opening a task. Choosing **Start agent** (or a
workflow step transition) always starts the agent as usual, and a failed or
interrupted session still shows its recovery actions.

## Task dependencies

A task can declare that it **depends on** one or more other tasks. This is a
peer relationship and is separate from the parent/child subtask hierarchy: a
subtask says "B is part of A", a dependency says "B cannot start until A
finishes". The two can be combined freely, including a dependency between a
task and its own child.

Dependencies form a graph, not just a line. A task can wait on several
predecessors and can itself block several dependents. A link that would close a
cycle is rejected when you try to create it, and the offending path is shown so
you can see which link to drop.

### Declare dependencies

Dependencies are declared in the **New Task** dialog under **Depends on**, or
by an agent over MCP. There is deliberately no editor in the open task: a
dependency records how the work was planned, so the surfaces that display it
stay read-only. To change one after the fact, use the MCP tools or delete and
recreate the task.

### What blocked means

A task with at least one unfinished predecessor is **blocked**. Blocked tasks
show a badge on their Kanban card and a dependency chip in the status row above
the chat box, next to the pull request chip. The chip reports both directions,
the tasks this one waits on and the tasks waiting on it, and each entry links
to that task.

While a task is blocked, no automated path starts it. That covers workflow
**On Enter** auto-start, promotion out of a WIP queue, integration watchers,
and dependency resolution itself. You can still press **Start agent**
yourself; a manual start is an explicit override, not an error.

### Chains that run themselves

A task created with dependencies and an agent start request does not launch
immediately. It records the start as an intent, and Kandev launches it once
every predecessor has completed successfully. Setting that up along a path
produces a chain:

1. Create task A normally.
2. Create task B with **Depends on** set to A.
3. Create task C with **Depends on** set to B.

Starting A is the only manual step. When A completes, B starts. When B
completes, C starts. A is never restarted.

Auto-start grants eligibility, never a bypass. If a task's dependencies have
resolved but the target step is at its WIP limit, the task stays queued and
launches when the queue promotes it, exactly as any other queued task would.

### When a predecessor does not succeed

Only successful completion resolves a dependency. A predecessor that ends in
**Failed** or **Cancelled** leaves its dependents blocked, and the blocked
reason names the failed task rather than reporting a generic wait. The chain
stops there and waits for you. Kandev never retries a failed predecessor on its
own and never quietly drops the link.

Three things clear it, all of them deliberate: retry the predecessor until it
succeeds, remove the link over MCP, or start the dependent manually.

An **archived** predecessor is treated as unfinished, not as failed and not as
resolved, so archiving a task does not release the work waiting on it.
**Deleting** a task does remove its links in both directions, and any dependent
that was waiting only on it becomes unblocked. That dependent is not started:
deletion is not success.

## Find and organize tasks

On desktop and tablet, the header switches between **Kanban**, **Pipeline**, and **List**. Kanban and Pipeline show the same workflow steps in different layouts. Kandev remembers the last selected view in that browser on the current device. Phones offer **Kanban** and **List** only; a saved desktop Pipeline preference is kept but shown as Kanban on the phone.

Under **Settings → General → Appearance → Startup Page**, choose **Task overview** (the default) or **Last visited task**. The latter resumes the most recently opened task in the current workspace on that device when Kandev starts or you open bare Home. It does not change an explicit task or workflow link. Home navigation and a task's Back action always return to the task overview; when there is no matching local recent task, Kandev opens the overview instead.

The **TASKS** list in the left sidebar has two time-based sort choices. These choices are separate from the sort choices in the task **List** view.

| Sort choice | Meaning |
| --- | --- |
| **Updated** | The last task summary refresh. Background events, such as pull-request status changes, can change this time. |
| **Last activity** | The last real user or agent action. Opening or focusing a task and background provider polling do not change this time. |

Choose **Last activity** when you want to review tasks by the least recent user or agent interaction.

- Search matches tasks without changing their state.
- The display menu filters by **Workflow** and **Repository** and can enable **Open preview on click**. In Kanban/Pipeline, each workflow lane has a **Columns** menu to hide individual steps. Unticking a step hides its column and tasks on that board, scoped to its own workflow, until you re-tick it. The optional **Auto-hide empty columns** setting collapses unoccupied steps without changing those manual choices; auto-hidden empty steps return as move destinations while a task is being moved, while manually hidden steps remain unavailable for pointer and bulk moves. On phones, open the menu drawer to change columns for the focused workflow.
- In **List**, the display menu can enable **Show task details** to include available repository, description, pull-request, session, parent, review, and archive context in each row. This option is off by default and follows the user across devices.
- **List** can group by **State**, **Workflow**, **Repository**, or **None**.
- **List** can sort by updated time, created time, or title in either direction.
- **Show archived** reveals archived tasks in List.
- List page sizes are 10, 25, or 50; the default is 25.
- Parent tasks and direct subtasks are indented as a tree.
- A subtask's action menu can detach it into a top-level task. Detaching preserves its workflow position and descendants; an inherited workspace remains shared with the former parent.

On phones, Kanban focuses one workflow and one step at a time. The board navigator always names both; open it to choose either level, or use the previous/next controls and horizontal swipe to move between steps. Choosing a workflow makes it the active workflow for board actions and task creation. Tap a card to open that task directly. Its **More options** menu opens as a touch-sized bottom surface; **Move to** changes the task's workflow or step. **Edit** can still rename a task after work starts, while its original prompt remains locked.

Regular Kanban does not currently expose label editing or label filters. Do not design a supported Kanban process around labels.

<details>
<summary>Configure a workflow, its steps, automation, and human gates</summary>

## Configure a workflow

Open **Settings → Workspaces → _workspace_ → Workflows**, then open a workflow card. A workflow has a name, an optional **Default Agent Profile**, and ordered steps. When the workflow has a default profile, users cannot choose another profile in the task-creation dialog.

You can add, reorder, edit, and delete steps. Deleting a step that still contains tasks opens a migration flow instead of silently stranding them. A GitHub-synchronized workflow is read-only in Kandev; change its source file in the synchronized repository.

### Configure each step

New steps allow manual moves by default. **Show in command panel** also defaults on. WIP is unlimited and auto-archive is off until configured.

| Setting                   | Effect                                                                                                                                                                                           |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Start step**            | Where a task is created when no agent starts with it. Only one step per workflow should be selected. If none is selected, Kandev falls back to the first positional step. This setting places tasks; it never starts agents, which is **Auto-start agent** below. |
| Agent profile             | Overrides the workflow/task profile when entering this step. A different profile creates a new session with fresh conversation context. The fixed profile override and original-session options are mutually exclusive. |
| **Override original session options** | Keeps the original conversation tab while applying model and ACP configuration rules for the task's starting agent family. The options editor appears below WIP settings only when this is checked. |
| **Auto-start agent**      | Starts an agent whenever a task enters the step.                                                                                                                                                 |
| **Plan mode**             | Enables plan mode when the task enters the step.                                                                                                                                                 |
| **Reset agent context**   | Starts with fresh conversation context on entry. It is disabled when the step has a profile override because the profile switch already creates a fresh session.                                 |
| **Allow manual move**     | Allows dragging a task into this step. Treat it as workflow UX, not as a security or approval boundary.                                                                                          |
| **Show in command panel** | Includes tasks in this step in the default, empty-search **Cmd+K** task list. Typed task search currently searches every step and can also return archived tasks, regardless of this setting.    |
| **Auto-archive**          | Archives inactive tasks after the configured number of hours. Enabling it starts at 24 hours; the minimum is 1.                                                                                  |
| **WIP limit**             | Maximum admitted active, non-archived, non-ephemeral tasks in the step. `0` means unlimited. Overflow remains visible as queued cards; manual moves into a full step succeed and queue there. |
| **Pull from**             | Optional one-hop feeder step. When capacity opens or eligible work arrives in the feeder, Kandev promotes queued work from the destination first, then the feeder. Direct moves and automatic transitions queue in the destination without using the feeder. A full feeder rejects new overflow creation. |

When **Reset agent context** creates a fresh ACP session, Kandev preserves the
selected ACP model, permission mode, and provider options. It restores these
settings before the next automatic prompt. If the provider rejects a setting,
the restoration fails and Kandev does not send the destination step's automatic
prompt.

The WIP check also applies when a task is created. It runs for an explicit
`workflow_step_id` and for the workflow's resolved start step, and the
admission check is atomic. When a limited step is full, the task is still
created and visible: it is queued in that step when no feeder is configured,
or placed in the configured feeder and tagged for the destination. Queued
tasks do not start sessions or consume destination WIP until promoted. If you
manually move a task, or an automatic transition sends it to a full limited
step, it queues in that destination instead of using the feeder. The Kanban
column shows the admitted count and limit, then a **Queued** section. The task
sidebar shows a queue icon whose tooltip gives the task's position in that
destination queue. If the configured
feeder is also full, creation returns a conflict. Ephemeral tasks are not
counted.

Integration watchers use the same admission rule. For example, a GitHub review
watch targeting a `Review` step with a limit of two admits at most two newly
observed pull requests at a time. Pull requests that lose the capacity race
remain eligible for a later poll; Kandev releases their temporary watch
reservation and does not start an agent for them.

Auto-archive is checked on a five-minute background interval and uses the task's last update time. Any task update postpones eligibility, so the archive is not guaranteed at the exact configured minute. Archiving, deleting, or moving an admitted task opens capacity and promotes the oldest queued card. Auto-archive affects the task itself, not its children.

Pull configuration rejects self-references, cycles, and cross-workflow feeders. Pulling runs when a task vacates the limited step and when eligible work is created in its feeder, filling each available slot. Destination-queued tasks are promoted before feeder candidates. Candidates are ordered by board position, then priority (`critical`, `high`, `medium`, `low`, `none`), queue time, creation time, and ID. A candidate whose move fails, for example because its session is running or starting, is skipped for that pull pass.

### Configure events and transitions

| Event                         | Available transition                                                                                                                                                                       |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **On Turn Start**             | Do nothing, move next, move previous, or move to a selected step when the user sends a message.                                                                                            |
| **On Turn Complete**          | **Do nothing (wait for user)**, move next, move previous, or move to a selected step after the agent turn.                                                                                 |
| **Cancelled turn completion** | When enabled, an explicit user cancellation also runs this step's normal `on_turn_complete` actions after the cancelled turn settles. It bypasses the `auto_advance_requires_signal` / `step_complete_kandev` gate for that cancellation, but a pending clarification still blocks the transition. It does not apply to silent clarification cancellation, peer interruptions, parent/task stops, provider errors, crashes, or runtime teardown. |
| **When Child Tasks Complete** | Do nothing, move next, move previous, or move to a selected step after every active direct child reaches `COMPLETED`, `FAILED`, or `CANCELLED`, provided the parent has an active session. |

The child-completion event ignores archived and ephemeral children, does not inspect grandchildren, and does nothing when the parent has no children. It also requires a parent session in `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT`; a parent with no session, or only an `IDLE`, `COMPLETED`, `FAILED`, or `CANCELLED` session, does not transition.

Generic comment, blocker-resolution, approval, heartbeat, budget, and error triggers, plus participant quorum, belong to the in-progress Office workflow surface. They are not configurable regular-Kanban step events.

When **On Turn Complete** moves a task, **Wait for agent completion signal** is available. With it enabled, a bare turn end leaves the task waiting; the agent must call `step_complete_kandev`. The call requires a summary and can include a handoff or blockers. It is idempotent within the step, runs asynchronously, and a user message sent before the transition is applied cancels that pending signal. Without the option, turn end counts as completion.

**Run completion actions when a turn is cancelled** is available beneath a configured turn-complete transition. It applies only when a user explicitly presses **Cancel** on the active turn. The normal completion pipeline still applies, including `on_exit`, the configured transition, and the destination step's `on_enter` actions; an `auto_start_agent` action there can start another turn immediately. An eligible explicit cancellation bypasses the `auto_advance_requires_signal` / `step_complete_kandev` gate, but a pending clarification still blocks the transition. The setting does not turn other interruption or failure paths into completion events. When the setting is off, an explicit cancel leaves the task in its current step and ready for input.

The built-in **Kanban** workflow enables this policy on **Backlog** and **In Progress** and leaves it disabled on its other steps. Custom steps and imported definitions default to disabled unless they set the field explicitly.

An auto-started task stays in its current step while the agent session boots and
while its first turn is running. A boot-ready event is not a turn completion.
For example, a review step with `on_enter: auto_start_agent` and
`on_turn_complete: move_to_next` moves to the next step only after the genuine
review turn completes, not during startup.

Plan mode can be disabled when the turn completes and/or when the task exits the step. A step prompt is Markdown and can include `{{task_prompt}}` to insert the original task description.

#### Override original session options

Check **Override original session options** when a workflow should keep one
conversation while changing its model settings between steps. For example, a
task can start with session model **5.6 Sol** and switch to **5.6 Luna** for an
implementation step. The options editor appears below WIP settings after the
checkbox is enabled; selecting a fixed **Agent profile** disables this option.

Add one rule per agent family; the rule is ignored when the task started with
another family. The family picker lists only families represented by configured
agent profiles, while existing persisted rules remain visible if capability
data later becomes unavailable. The editor uses the same model and ACP option
picker as the chat input, so provider-specific models and options are selected
from the agent's advertised capabilities.

The model and option list is resolved for the selected model. Providers can
therefore expose different options for different models, and the list can
change after a model selection. Kandev removes saved option values only after
a successful provider response; if discovery fails, the current draft remains
available and can be retried.

Each rule can **Set** a model and any selected options, **Keep** the settings
already active, or **Restore original** to reapply the immutable model and
option values captured when the original session finished initializing, after
profile settings were applied.
Rules are best-effort: a rejected field produces a warning, while successful
fields remain active and the step continues. The settings are applied before
an auto-start prompt and persist as the session's runtime overrides.

This behavior is mutually exclusive with the step's fixed **Agent profile**
override. A fixed profile intentionally creates a separate session; conditional
rules never activate or mutate that replacement tab. If an earlier rule may
carry changed values into a later step, the editor shows a warning with **Keep**,
**Restore**, and **Set new** choices. Read-only synced workflows display these
rules and warnings but cannot edit them.

### Build a human gate

For a Review or Approval step:

1. Set **On Turn Complete** to **Do nothing (wait for user)**.
2. Leave automatic movement into the next step disabled.
3. Have the reviewer inspect Changes, tests, and the conversation.
4. Move the task manually or send the next instruction only after approval.

`step_complete_kandev` is an agent-completion gate, not human approval. Profile permissions, repository credentials, and branch protection still apply.

### Avoid automation loops

An entry action can auto-start an agent, and turn completion can move the task into another step that auto-starts again. Trace the entire cycle before enabling it. WIP limits stop over-capacity moves but are not compute budgets. Keep a **Do nothing** transition wherever a person must decide whether work continues.

For examples and portability, see [Workflow tips](workflow-tips.md), [Workflow import and export](workflow-import-export.md), and [Workflow sync](workflow-sync.md).

</details>

## Use the task plan

Regular tasks have one shared Markdown plan, not a collection of named documents.

<DocsVideo
  webm="./media/feature-guides/plan-review-implement.webm"
  mp4="./media/feature-guides/plan-review-implement.mp4"
  poster="./media/feature-guides/plan-review-implement.webp"
  title="Review a plan before implementation"
  caption="A plan step receives human feedback before the approved plan moves into implementation."
/>

1. In the task workbench, select **Add panel (+) → Plan**.
2. Write the plan or let an agent write it through task MCP.
3. Edit it directly. The panel autosaves after 1.5 seconds.
4. Use plan history to preview a revision, compare it with the previous or current revision, or restore it. Restore creates a new revision; it does not erase history or coalesce with the preceding revision.
5. Select plan text to leave a comment. **Run** sends the selected feedback to the agent in plan mode.
6. Choose **Implement** for the current session or **Implement in fresh agent**. Kandev saves the current draft first and marks the plan as sent for implementation; the implement control is then disabled for that plan.

Agents use `create_task_plan_kandev`, `get_task_plan_kandev`, `update_task_plan_kandev`, and `delete_task_plan_kandev`. Human edits are therefore visible to the next agent that reads the plan. A plan records intent; verify that code and review still match it.

Revision history is not an immutable record of every autosave. Consecutive writes from the same author name and author kind coalesce into the latest revision for five minutes by default. Operators can set `KANDEV_PLAN_COALESCE_WINDOW_MS`; `0` disables coalescing, while an invalid or negative value falls back to five minutes.

## Office documents, labels, and blockers

> [!EXPERIMENTAL]
> Office is feature-flagged, disabled in the production profile by default, and still in progress. Its named documents, labels, and blocker controls are not stable regular-Kanban features.

| Capability                            | Regular Kanban | Office                                              |
| ------------------------------------- | -------------- | --------------------------------------------------- |
| One versioned task plan               | Available      | Available in Office-specific surfaces where enabled |
| Multiple named task documents         | Not exposed    | In-progress Office capability                       |
| Task label editor and label filters   | Not exposed    | In-progress Office capability                       |
| Blocked-by / blocking property editor | Set at task creation or over MCP; read-only afterwards | In-progress Office capability |

Regular Kanban reads and enforces blocker relationships (see [Task dependencies](#task-dependencies)) but has no blocker filter and no in-place editor: dependencies are declared when the task is created or over MCP. Office additionally exposes named documents, labels, and its own blocker property editor. Do not treat those Office surfaces as a stable public contract yet.

## Archive, unarchive, and delete

Archive records the task as archived and removes it from active views immediately. Runtime stopping and physical cleanup then run in the background with a 60-second timeout. Cleanup is best-effort: a stop or deletion failure is logged and does not undo the archive, and Kandev preserves a runtime or environment when a nonterminal session cannot be stopped. Shared inherited environments and borrowed worktrees are also preserved while another active task still uses them.

| Executor      | Archive cleanup                                                                                                                                                                                 |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Local         | Attempts to stop the agent runtime; leaves the local folder, files, and branch untouched.                                                                                                       |
| Git worktree  | Attempts to remove the Kandev-owned worktree directory. It keeps the local task branch and leaves any existing remote branch untouched. Shared or borrowed worktrees can remain until their last active user is gone. |
| Local Docker  | Attempts to stop and remove the container; the host repository remains.                                                                                                                         |
| Kubernetes    | Deletes only the recorded Pod and Kandev-managed PVC after exact UID and ownership checks. An existing claim is retained.                                                                       |
| Remote Docker | Runtime create and stop are not implemented. This executor is in progress and cannot currently start a task, so it has no supported archive-cleanup flow.                                       |
| Sprites       | Attempts to destroy the sandbox; if cleanup succeeds, uncommitted sandbox work is lost.                                                                                                         |
| SSH           | Attempts to stop the remote session runtime, but the remote task directory remains. Audit and remove retained task directories manually after confirming that no session needs them.            |

The archive confirmation is enabled by default at **Settings → General → Task Actions → Archive Confirmation** under **Confirm before archiving tasks**. If a parent has children, **Also archive _N_ subtasks** is unchecked by default; without it, the children remain active. Task MCP archive/delete operations affect only the selected task and do not offer the cascade checkbox. MCP delete also does not reparent direct children the way the UI's non-cascade delete does; use the UI rather than task MCP to delete a parent that still has children.

To restore a task, open **List**, enable **Show archived**, and choose unarchive. If the parent was archived with its children, the cascade-owned children are restored with it. For worktree tasks, archive keeps the environment identity and the local branch. The next session recreates the worktree directory from that branch. Recovery is best-effort and does not rewrite ambiguous multi-row attachments for the same repository. If an external action or an older Kandev version removed the branch, Kandev also checks `origin`. If no branch exists, the next session starts from the base branch. Removed worktree directories, containers, and sandboxes are materialized again on a later launch rather than resumed in place.

Delete is permanent. If **Also delete _N_ subtasks** is left unchecked, direct children become root tasks. If selected, descendants are deleted. The operation cannot be undone, and executor cleanup follows the same asynchronous, best-effort rules as archive.

When a task still has a `RUNNING` agent, the confirmation dialog adds a
still-working warning: proceeding discards work that is in progress. Delete
always shows this warning; archive shows it only when the archive confirmation
is enabled. Best-effort detached-work accounting does not independently keep a
settled task in the still-working state.

## Troubleshooting

- **No workflow is available:** open the workspace's **Workflows** page. Newly added workspaces have none by default.
- **No agent starts:** the empty-description **Start Plan Mode** path does not use the normal start-agent submission. To begin an agent immediately, enter a description and use **Start task** or **Start task in plan mode**; also confirm the selected profiles are healthy and compatible.
- **Task starts in the wrong step:** the destination depends on the action. **Create without starting agent** uses **Start step** with first-step fallback; **Start task** uses the first **Auto-start agent** step and falls back to **Start step**; **Start task in plan mode** deliberately uses the first positional step. An explicit `workflow_step_id` from the creator outranks all three.
- **A task moves unexpectedly:** inspect **On Turn Start**, **On Turn Complete**, child completion, entry actions, and the destination step's entry actions.
- **A task stays after a cancel:** check for a pending clarification, the cancelled-turn completion policy, an absent or blocked transition, a queued WIP card, or an invalid target left by an older definition.
- **Move rejected:** check the target WIP limit and whether the task is already counted there.
- **Pull does nothing:** configure a nonzero WIP limit, remove cycles, and confirm feeder candidates are not running or starting.
- **Child completion does not move the parent:** confirm every active direct child is terminal and the parent still has a session in `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT`.
- **Completion signal appears ignored:** it is asynchronous; also check whether a user message canceled it or whether the task already left the step.
- **Remote source cannot clone or fetch:** verify provider credentials and access to every repository and base branch.
- **Attachment is rejected below the picker limit:** encoded size is subject to the backend's stricter 10 MB item/batch checks.
- **Resources remain after archive or delete:** physical cleanup is asynchronous and best-effort. Check for an active task sharing the environment, a failed runtime stop, and server cleanup logs before removing anything manually.
- **An unarchived worktree starts fresh:** an external action or an older Kandev version removed the branch, and no matching branch exists on `origin`.
- **A synchronized workflow is read-only:** edit the workflow file in its GitHub source and let sync apply the change.

Related: [Coordinate work](coordination.md), [Sessions and review](sessions-and-review.md), [Agents and profiles](agents-and-profiles.md), and [Automation and MCP](automation-and-mcp.md).
