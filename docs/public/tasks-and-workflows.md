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
3. Open the workspace's **Repositories** page and add existing local repositories the workspace needs. You can also initialize a new empty repository while creating a task. Remote URLs are not registered on this page; enter them through **New Task → Remote**.
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
   | **Repo**   | A configured, discovered, or new local repository | Select a base branch for each repository row. For a single-row new task, **Create new repository** initializes an empty `main` repository in a parent folder you choose. Add more rows for a multi-repository task.                                                                                                                                                                                                  |
   | **Remote** | A remote repository                               | Search configured GitHub, GitLab, or Azure DevOps repositories, or paste a supported URL. A pasted URL stays editable until you press Enter; then select the branch. Anonymous, credential-free reads include public GitHub repository branches, pull requests, and issues, plus public `gitlab.com` branch discovery. Private resources and authenticated browse/write features require valid provider credentials. |
   | **None**   | Planning, research, or work outside Git           | Use a scratch workspace or an optional folder on the Kandev host. Git worktree execution and repository-aware Changes, branch, and pull-request features are unavailable.                                                                                                                                                                                                                                            |

4. Select a compatible executor profile and agent profile. A workflow default agent profile locks the task-level agent selector. Executor and agent compatibility is validated before launch.
5. Enter the initial description. In the **New Task** dialog, an empty description changes the primary action to **Start Plan Mode**; the other dialog actions require a description. Agent-facing task MCP has different empty-description rules. When agent-generated task titles are enabled, every task and subtask action requires a nonempty prompt; the empty-description Plan Mode exception is disabled. A nonempty description exposes the standard split actions.
6. Choose the applicable action:
   - **Start Plan Mode** is the primary empty-description action and creates the task through the plan-mode path.
   - **Start task** requires a nonempty description, creates the task, and starts its agent.
   - **Start task in plan mode** requires a nonempty description and starts the agent with plan mode enabled. This path starts in the first positional workflow step, even if another step is marked **Start step**.
   - **Create without starting agent** requires a nonempty description. A structured ACP profile prepares the session/workspace without starting an agent turn. Passthrough/TUI is an exception: the backend launches it immediately so its native PTY exists.

   On mobile, the two non-primary actions are separate buttons labeled **Plan mode** and **Create only**; they have the same plan-mode and create-without-agent behavior.

Kandev remembers draft or recently used repository, branch, executor, and profile choices. Review the restored values before submitting, especially after changing workspace.

Creating a repository is available only in an unlocked, single-repository **New Task** form. Kandev rejects an existing target path, creates no initial files or commit, registers the repository in the workspace, and switches the task to a direct **Local** executor profile. If no direct Local profile is available, repository creation stays disabled. Add more repository rows only after selecting existing repositories; empty multi-repository worktrees are not supported.

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

Open **Settings → General → Task Actions → Profile for Tasks Created by Agents** to choose which agent profile Kandev assigns when an agent calls a Kandev MCP tool that creates a task without choosing `agent_profile_id`. The profile determines the agent, model, and setup used when the new task starts:

- **Current task profile** is useful when follow-up work needs the same model and agent setup as the task creating it. It preserves compatibility with existing behavior and is selected by default. Kandev first inherits the parent or calling task profile, then checks the workflow step or workflow default, and finally checks the target workspace's **Default Agent Profile**. This option can unintentionally reuse a more expensive profile.
- **Workspace default profile** is useful when you want agent-created tasks to use your standard workspace model and cost policy. It skips the parent and calling task profiles. Kandev still uses the workflow step or workflow default first, then the **Default Agent Profile** from the workspace that will own the new task.

Select an option, then choose **Save changes**. The only affected Kandev MCP tool is `create_task_kandev`; the preference covers both new tasks and subtasks when the call omits `agent_profile_id`. It does not affect `spawn_session_kandev`, because that tool adds a session to the current task instead of creating a task. It also does not affect tasks you create in the UI. The preference applies across workspaces, but **Workspace default profile** resolves the default from each new task's target workspace. An explicit `agent_profile_id` in the tool call always overrides the saved preference. The one case where an explicit `agent_profile_id` does not run is a task that lands on a workflow step: the step's launch profile (its pinned profile, or the workflow default when unpinned) is what the agent starts with, and the created task records that profile. A task lands on a step whenever its workflow has steps, using the start step when `workflow_step_id` is omitted. If **Workspace default profile** is selected and neither the workflow nor the target workspace supplies a profile, task creation fails without creating the task, including when `start_agent=false`.

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
history above your prompt, even though the prompt itself is out of view —
use **Scroll to last prompt** to jump back to it instead. Expand the bar for
longer prompts, or use its scroll action to return to the full prompt; the
expanded view is capped at 40% of the transcript panel's height so it stays
proportionate whether the panel is a full-screen view or a small embedded
split. The anchored bar is desktop-only; phones use the scroll-to-last-prompt
action instead. Both scroll actions keep the transcript at your requested
position even if the agent streams new replies while the scroll is still in
progress.

### Multiple repositories

A task can include several local or remote repository rows. Multi-repository creation supports **Worktree**, **Local Docker**, **SSH**, and **Sprites**. Local/Local PC creation remains unavailable until its initial-launch path can materialize sibling repositories, and Remote Docker is not implemented. Public GitHub and GitLab repositories can be cloned and fetched anonymously. Private repositories and authenticated browse/write features need credentials that can access the selected base branch.

If Kandev cannot resolve a pasted remote URL or its branch, the repository row keeps the URL and shows the provider error. Use **Retry** after correcting the URL or when a transient provider failure has cleared.

Changes and review are scoped by repository. State the expected deliverable, base branch, and pull-request target for every attachment. See [Coordinate work](coordination.md) for adding branches after creation and splitting multi-repository work.

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
Local Docker, SSH, and Sprites attach repository siblings to the current remote workspace and rescan
without restarting the agent or changing its CWD.

Folders are live host paths and are available only to **Local/Local PC** and **Worktree** tasks. Repository sources are supported for **Worktree**, **Local/Local PC**, **Local Docker**, **SSH**, and **Sprites**. Local Git rows need a cloneable origin on Docker, SSH, and Sprites; Worktree and Local/Local PC can use the host repository directly. See [Executors](executors.md#workspace-sources) and [Coordinate work](coordination.md#add-sources-after-creation) for runtime limits and recovery behavior.

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

## Find and organize tasks

On desktop and tablet, the header switches between **Kanban**, **Pipeline**, and **List**. Kanban and Pipeline show the same workflow steps in different layouts. Kandev remembers the last selected view in that browser on the current device. Phones offer **Kanban** and **List** only; a saved desktop Pipeline preference is kept but shown as Kanban on the phone.

Under **Settings → General → Appearance → Startup Page**, choose **Task overview** (the default) or **Last visited task**. The latter resumes the most recently opened task in the current workspace on that device when Kandev starts or you open bare Home. It does not change an explicit task or workflow link. Home navigation and a task's Back action always return to the task overview; when there is no matching local recent task, Kandev opens the overview instead.

- Search matches tasks without changing their state.
- The display menu filters by **Workflow** and **Repository** and can enable **Open preview on click**.
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
| **Start step**            | Makes this the normal starting step. Only one step per workflow should be selected. If none is selected, Kandev falls back to the first positional step.                                         |
| Agent profile             | Overrides the workflow/task profile when entering this step. A different profile creates a new session with fresh conversation context. The fixed profile override and original-session options are mutually exclusive. |
| **Override original session options** | Keeps the original conversation tab while applying model and ACP configuration rules for the task's starting agent family. The options editor appears below WIP settings only when this is checked. |
| **Auto-start agent**      | Starts an agent whenever a task enters the step.                                                                                                                                                 |
| **Plan mode**             | Enables plan mode when the task enters the step.                                                                                                                                                 |
| **Reset agent context**   | Starts with fresh conversation context on entry. It is disabled when the step has a profile override because the profile switch already creates a fresh session.                                 |
| **Allow manual move**     | Allows dragging a task into this step. Treat it as workflow UX, not as a security or approval boundary.                                                                                          |
| **Show in command panel** | Includes tasks in this step in the default, empty-search **Cmd+K** task list. Typed task search currently searches every step and can also return archived tasks, regardless of this setting.    |
| **Auto-archive**          | Archives inactive tasks after the configured number of hours. Enabling it starts at 24 hours; the minimum is 1.                                                                                  |
| **WIP limit**             | Maximum admitted active, non-archived, non-ephemeral tasks in the step. `0` means unlimited. Overflow remains visible as queued cards; manual moves into a full step are still rejected. |
| **Pull from**             | Optional one-hop feeder step. When capacity opens or eligible work arrives in the feeder, Kandev promotes queued work from the destination first, then the feeder. A full feeder rejects new overflow creation. |

The WIP check also applies when a task is created. It runs for an explicit
`workflow_step_id` and for the workflow's resolved start step, and the
admission check is atomic. When a limited step is full, the task is still
created and visible: it is queued in that step when no feeder is configured,
or placed in the configured feeder and tagged for the destination. Queued
tasks do not start sessions or consume destination WIP until promoted. If the
configured feeder is also full, creation returns a conflict. Ephemeral tasks
are not counted.

Integration watchers use the same admission rule. For example, a GitHub review
watch targeting a `Review` step with a limit of two admits at most two newly
observed pull requests at a time. Pull requests that lose the capacity race
remain eligible for a later poll; Kandev releases their temporary watch
reservation and does not start an agent for them.

Auto-archive is checked on a five-minute background interval and uses the task's last update time. Any task update postpones eligibility, so the archive is not guaranteed at the exact configured minute. Archiving, deleting, or moving an admitted task opens capacity and promotes the oldest queued card. Auto-archive affects the task itself, not its children.

Pull configuration rejects self-references, cycles, and cross-workflow feeders. Pulling runs when a task vacates the limited step and when eligible work is created in its feeder, filling each available slot. Candidates are ordered by board position, then priority (`critical`, `high`, `medium`, `low`, `none`), queue time, creation time, and ID. A candidate whose move fails—for example because its session is running or starting—is skipped for that pull pass.

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
| Blocked-by / blocking property editor | Not exposed    | In-progress Office capability                       |

Stored related-task data can include blocker relationships, but regular Kanban has no blocker editor or blocker filter. Use workflow gates, direct-child completion, and explicit messages for supported Kanban coordination. Do not treat Office's named documents, labels, or blocker UI as a stable public contract yet.

## Archive, unarchive, and delete

Archive records the task as archived and removes it from active views immediately. Runtime stopping and physical cleanup then run in the background with a 60-second timeout. Cleanup is best-effort: a stop or deletion failure is logged and does not undo the archive, and Kandev preserves a runtime or environment when a nonterminal session cannot be stopped. Shared inherited environments and borrowed worktrees are also preserved while another active task still uses them.

| Executor      | Archive cleanup                                                                                                                                                                                 |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Local         | Attempts to stop the agent runtime; leaves the local folder, files, and branch untouched.                                                                                                       |
| Git worktree  | Attempts to remove the Kandev-owned worktree and its local task branch. It does not delete the remote branch, and shared or borrowed worktrees can remain until their last active user is gone. |
| Local Docker  | Attempts to stop and remove the container; the host repository remains.                                                                                                                         |
| Remote Docker | Runtime create and stop are not implemented. This executor is in progress and cannot currently start a task, so it has no supported archive-cleanup flow.                                       |
| Sprites       | Attempts to destroy the sandbox; if cleanup succeeds, uncommitted sandbox work is lost.                                                                                                         |
| SSH           | Attempts to stop the remote session runtime, but the remote task directory remains. Audit and remove retained task directories manually after confirming that no session needs them.            |

The archive confirmation is enabled by default at **Settings → General → Task Actions → Archive Confirmation** under **Confirm before archiving tasks**. If a parent has children, **Also archive _N_ subtasks** is unchecked by default; without it, the children remain active. Task MCP archive/delete operations affect only the selected task and do not offer the cascade checkbox. MCP delete also does not reparent direct children the way the UI's non-cascade delete does; use the UI rather than task MCP to delete a parent that still has children.

To restore a task, open **List**, enable **Show archived**, and choose unarchive. If the parent was archived with its children, the cascade-owned children are restored with it. For worktree tasks, Kandev probes the newest historical worktree branch for each repository. If that branch still exists locally or on `origin`, Kandev restores it as the checkout branch so the next session can pick it up. Recovery is best-effort and does not rewrite ambiguous multi-row attachments for the same repository. If the branch is missing, the unarchive toast warns that the next session starts fresh from the base branch; work that existed only on the deleted local branch is unrecoverable. Removed worktree directories, containers, and sandboxes are materialized again on a later launch rather than resumed in place.

Delete is permanent. If **Also delete _N_ subtasks** is left unchecked, direct children become root tasks. If selected, descendants are deleted. The operation cannot be undone, and executor cleanup follows the same asynchronous, best-effort rules as archive.

When a task still has a `RUNNING` agent, the confirmation dialog adds a
still-working warning: proceeding discards work that is in progress. Delete
always shows this warning; archive shows it only when the archive confirmation
is enabled. Best-effort detached-work accounting does not independently keep a
settled task in the still-working state.

## Troubleshooting

- **No workflow is available:** open the workspace's **Workflows** page. Newly added workspaces have none by default.
- **No agent starts:** the empty-description **Start Plan Mode** path does not use the normal start-agent submission. To begin an agent immediately, enter a description and use **Start task** or **Start task in plan mode**; also confirm the selected profiles are healthy and compatible.
- **Task starts in the wrong step:** normal creation uses **Start step** with first-step fallback; **Start task in plan mode** deliberately uses the first positional step.
- **A task moves unexpectedly:** inspect **On Turn Start**, **On Turn Complete**, child completion, entry actions, and the destination step's entry actions.
- **A task stays after a cancel:** check for a pending clarification, the cancelled-turn completion policy, an absent or blocked transition, a queued WIP card, or an invalid target left by an older definition.
- **Move rejected:** check the target WIP limit and whether the task is already counted there.
- **Pull does nothing:** configure a nonzero WIP limit, remove cycles, and confirm feeder candidates are not running or starting.
- **Child completion does not move the parent:** confirm every active direct child is terminal and the parent still has a session in `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT`.
- **Completion signal appears ignored:** it is asynchronous; also check whether a user message canceled it or whether the task already left the step.
- **Remote source cannot clone or fetch:** verify provider credentials and access to every repository and base branch.
- **Attachment is rejected below the picker limit:** encoded size is subject to the backend's stricter 10 MB item/batch checks.
- **Resources remain after archive or delete:** physical cleanup is asynchronous and best-effort. Check for an active task sharing the environment, a failed runtime stop, and server cleanup logs before removing anything manually.
- **An unarchived worktree starts fresh:** the prior branch no longer existed locally or on `origin`; any work that was never pushed or otherwise saved cannot be recovered by Kandev.
- **A synchronized workflow is read-only:** edit the workflow file in its GitHub source and let sync apply the change.

Related: [Coordinate work](coordination.md), [Sessions and review](sessions-and-review.md), [Agents and profiles](agents-and-profiles.md), and [Automation and MCP](automation-and-mcp.md).
