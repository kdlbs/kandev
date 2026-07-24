---
title: "Tasks and Workflows"
description: "Create scoped tasks, configure workflow behavior, use plans, and manage the task lifecycle."
---

# Tasks and Workflows

A task is the unit of work. A workflow is the ordered process that task follows. Keeping those two concerns separate lets the same repository use different processes for a quick fix, a planned feature, or a human-reviewed change.

## Understand the model

| Concept | What it controls |
|---|---|
| Workspace | The scope containing repositories, workflows, tasks, integrations, and workspace defaults. |
| Workflow | An ordered set of steps plus the rules that run when a task or agent turn reaches an event. |
| Workflow step | The task's current process position, such as Backlog, Work, Review, or Done. |
| Task | The title, prompt, workflow position, repository attachments, sessions, and one shared plan. |
| Task repository | A repository, base branch, and optional checkout branch attached to a task. A task can have more than one. |
| Session | One agent conversation attached to a task. Several sessions can share the same task environment. |
| Plan | The task's single editable Markdown plan, with version history. Consecutive writes can be coalesced into one revision. |

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

1. Enter a title.
2. Select the workspace and workflow when Kandev cannot infer them. A regular non-ephemeral task must belong to a workflow.
3. Select a source:

   | Source | Use it for | Important behavior |
   |---|---|---|
   | **Repo** | A configured, discovered, or new local repository | Select a base branch for each repository row. For a single-row new task, **Create new repository** initializes an empty `main` repository in a parent folder you choose. Add more rows for a multi-repository task. |
   | **Remote** | A remote repository | Search configured GitHub, GitLab, or Azure DevOps repositories, or paste a supported URL; then select the branch. Clone and fetch require valid credentials. |
   | **None** | Planning, research, or work outside Git | Use a scratch workspace or an optional folder on the Kandev host. Git worktree execution and repository-aware Changes, branch, and pull-request features are unavailable. |

4. Select a compatible executor profile and agent profile. A workflow default agent profile locks the task-level agent selector. Executor and agent compatibility is validated before launch.
5. Enter the initial description. In the **New Task** dialog, an empty description changes the primary action to **Start Plan Mode**; the other dialog actions require a description. Agent-facing task MCP has different empty-description rules. A nonempty description exposes the standard split actions.
6. Choose the applicable action:

   - **Start Plan Mode** is the primary empty-description action and creates the task through the plan-mode path.
   - **Start task** requires a nonempty description, creates the task, and starts its agent.
   - **Start task in plan mode** requires a nonempty description and starts the agent with plan mode enabled. This path starts in the first positional workflow step, even if another step is marked **Start step**.
   - **Create without starting agent** requires a nonempty description. A structured ACP profile prepares the session/workspace without starting an agent turn. Passthrough/TUI is an exception: the backend launches it immediately so its native PTY exists.

   On mobile, the two non-primary actions are separate buttons labeled **Plan mode** and **Create only**; they have the same plan-mode and create-without-agent behavior.

Kandev remembers draft or recently used repository, branch, executor, and profile choices. Review the restored values before submitting, especially after changing workspace.

Creating a repository is available only in an unlocked, single-repository **New Task** form. Kandev rejects an existing target path, creates no initial files or commit, registers the repository in the workspace, and switches the task to a direct **Local** executor profile. If no direct Local profile is available, repository creation stays disabled. Add more repository rows only after selecting existing repositories; empty multi-repository worktrees are not supported.

### Choose the profile for tasks created by agents

Open **Settings → General → Task Actions → Profile for Tasks Created by Agents** to choose which agent profile Kandev assigns when an agent calls a Kandev MCP tool that creates a task without choosing `agent_profile_id`. The profile determines the agent, model, and setup used when the new task starts:

- **Current task profile** is useful when follow-up work needs the same model and agent setup as the task creating it. It preserves compatibility with existing behavior and is selected by default. Kandev first inherits the parent or calling task profile, then checks the workflow step or workflow default, and finally checks the target workspace's **Default Agent Profile**. This option can unintentionally reuse a more expensive profile.
- **Workspace default profile** is useful when you want agent-created tasks to use your standard workspace model and cost policy. It skips the parent and calling task profiles. Kandev still uses the workflow step or workflow default first, then the **Default Agent Profile** from the workspace that will own the new task.

Select an option, then choose **Save changes**. The only affected Kandev MCP tool is `create_task_kandev`; the preference covers both new tasks and subtasks when the call omits `agent_profile_id`. It does not affect `spawn_session_kandev`, because that tool adds a session to the current task instead of creating a task. It also does not affect tasks you create in the UI. The preference applies across workspaces, but **Workspace default profile** resolves the default from each new task's target workspace. An explicit `agent_profile_id` in the tool call always overrides the saved preference. If **Workspace default profile** is selected and neither the workflow nor the target workspace supplies a profile, task creation fails without creating the task, including when `start_agent=false`.

### Multiple repositories

A task can include several local or remote repository rows. Multi-repository task creation currently requires the **git-worktree** executor; the dialog leaves incompatible executors visible but disables them with `Multi-repo tasks only support the git-worktree executor.` Each remote needs credentials that can clone and fetch its selected base branch.

Changes and review are scoped by repository. State the expected deliverable, base branch, and pull-request target for every attachment. See [Coordinate work](coordination.md) for adding branches after creation and splitting multi-repository work.

### Attachments and local-change consent

The task prompt supports image, audio, and resource attachments. The backend accepts at most 10 attachments and rejects an encoded item or encoded batch larger than 10 MB. The picker also applies a 10 MB raw-file and 20 MB raw-total guard, so encoding overhead can make the backend limit stricter than the picker limit.

Creating a fresh local branch is available only with the local executor. If the checkout is dirty, Kandev lists the affected paths and requires explicit consent before discarding those local changes. If another path becomes dirty after the warning, creation fails with a conflict and asks for consent again. Save or commit work before approving this operation.

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

## Find and organize tasks

On desktop and tablet, the header switches between **Kanban**, **Pipeline**, and **List**. Kanban and Pipeline show the same workflow steps in different layouts. Phones offer **Kanban** and **List** only; a saved desktop Pipeline preference is kept but shown as Kanban on the phone.

- Search matches tasks without changing their state.
- The display menu filters by **Workflow** and **Repository** and can enable **Open preview on click**.
- **List** can group by **State**, **Workflow**, **Repository**, or **None**.
- **List** can sort by updated time, created time, or title in either direction.
- **Show archived** reveals archived tasks in List.
- List page sizes are 10, 25, or 50; the default is 25.
- Parent tasks and direct subtasks are indented as a tree.
- A subtask's action menu can detach it into a top-level task. Detaching preserves its workflow position and descendants; an inherited workspace remains shared with the former parent.

On phones, Kanban focuses one workflow and one step at a time. The board navigator always names both; open it to choose either level, or use the previous/next controls and horizontal swipe to move between steps. Choosing a workflow makes it the active workflow for board actions and task creation. Tap a card to open that task directly. Its **More options** menu opens as a touch-sized bottom surface; **Move to** changes the task's workflow or step. **Edit** can still rename a task after work starts, while its original prompt remains locked.

Regular Kanban does not currently expose label editing or label filters. Do not design a supported Kanban process around labels.

## Configure a workflow

Open **Settings → Workspaces → _workspace_ → Workflows**, then open a workflow card. A workflow has a name, an optional **Default Agent Profile**, and ordered steps. When the workflow has a default profile, users cannot choose another profile in the task-creation dialog.

You can add, reorder, edit, and delete steps. Deleting a step that still contains tasks opens a migration flow instead of silently stranding them. A GitHub-synchronized workflow is read-only in Kandev; change its source file in the synchronized repository.

### Configure each step

New steps allow manual moves by default. **Show in command panel** also defaults on. WIP is unlimited and auto-archive is off until configured.

| Setting | Effect |
|---|---|
| **Start step** | Makes this the normal starting step. Only one step per workflow should be selected. If none is selected, Kandev falls back to the first positional step. |
| Agent profile | Overrides the workflow/task profile when entering this step. A different profile creates a new session with fresh conversation context. |
| **Auto-start agent** | Starts an agent whenever a task enters the step. |
| **Plan mode** | Enables plan mode when the task enters the step. |
| **Reset agent context** | Starts with fresh conversation context on entry. It is disabled when the step has a profile override because the profile switch already creates a fresh session. |
| **Allow manual move** | Allows dragging a task into this step. Treat it as workflow UX, not as a security or approval boundary. |
| **Show in command panel** | Includes tasks in this step in the default, empty-search **Cmd+K** task list. Typed task search currently searches every step and can also return archived tasks, regardless of this setting. |
| **Auto-archive** | Archives inactive tasks after the configured number of hours. Enabling it starts at 24 hours; the minimum is 1. |
| **WIP limit** | Maximum active, non-archived, non-ephemeral tasks accepted by the step. `0` means unlimited. A move into a full step is rejected; reordering within the same step does not consume another slot. |
| **Pull from** | Optional feeder step. When capacity opens, Kandev pulls eligible work from that step. It requires a nonzero WIP limit. |

Auto-archive is checked on a five-minute background interval and uses the task's last update time. Any task update postpones eligibility, so the archive is not guaranteed at the exact configured minute. Auto-archive affects the task itself, not its children.

Pull configuration rejects self-references, cycles, and cross-workflow feeders. Pulling runs when a task vacates the limited step and fills each newly open slot. Candidates are ordered by board position, then priority (`critical`, `high`, `medium`, `low`, `none`), creation time, and ID. A candidate whose move fails—for example because its session is running or starting—is skipped for that pull pass.

### Configure events and transitions

| Event | Available transition |
|---|---|
| **On Turn Start** | Do nothing, move next, move previous, or move to a selected step when the user sends a message. |
| **On Turn Complete** | **Do nothing (wait for user)**, move next, move previous, or move to a selected step after the agent turn. |
| **When Child Tasks Complete** | Do nothing, move next, move previous, or move to a selected step after every active direct child reaches `COMPLETED`, `FAILED`, or `CANCELLED`, provided the parent has an active session. |

The child-completion event ignores archived and ephemeral children, does not inspect grandchildren, and does nothing when the parent has no children. It also requires a parent session in `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT`; a parent with no session, or only an `IDLE`, `COMPLETED`, `FAILED`, or `CANCELLED` session, does not transition.

Generic comment, blocker-resolution, approval, heartbeat, budget, and error triggers, plus participant quorum, belong to the in-progress Office workflow surface. They are not configurable regular-Kanban step events.

When **On Turn Complete** moves a task, **Wait for agent completion signal** is available. With it enabled, a bare turn end leaves the task waiting; the agent must call `step_complete_kandev`. The call requires a summary and can include a handoff or blockers. It is idempotent within the step, runs asynchronously, and a user message sent before the transition is applied cancels that pending signal. Without the option, turn end counts as completion.

Plan mode can be disabled when the turn completes and/or when the task exits the step. A step prompt is Markdown and can include `{{task_prompt}}` to insert the original task description.

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

| Capability | Regular Kanban | Office |
|---|---|---|
| One versioned task plan | Available | Available in Office-specific surfaces where enabled |
| Multiple named task documents | Not exposed | In-progress Office capability |
| Task label editor and label filters | Not exposed | In-progress Office capability |
| Blocked-by / blocking property editor | Not exposed | In-progress Office capability |

Stored related-task data can include blocker relationships, but regular Kanban has no blocker editor or blocker filter. Use workflow gates, direct-child completion, and explicit messages for supported Kanban coordination. Do not treat Office's named documents, labels, or blocker UI as a stable public contract yet.

## Archive, unarchive, and delete

Archive records the task as archived and removes it from active views immediately. Runtime stopping and physical cleanup then run in the background with a 60-second timeout. Cleanup is best-effort: a stop or deletion failure is logged and does not undo the archive, and Kandev preserves a runtime or environment when a nonterminal session cannot be stopped. Shared inherited environments and borrowed worktrees are also preserved while another active task still uses them.

| Executor | Archive cleanup |
|---|---|
| Local | Attempts to stop the agent runtime; leaves the local folder, files, and branch untouched. |
| Git worktree | Attempts to remove the Kandev-owned worktree and its local task branch. It does not delete the remote branch, and shared or borrowed worktrees can remain until their last active user is gone. |
| Local Docker | Attempts to stop and remove the container; the host repository remains. |
| Remote Docker | Runtime create and stop are not implemented. This executor is in progress and cannot currently start a task, so it has no supported archive-cleanup flow. |
| Sprites | Attempts to destroy the sandbox; if cleanup succeeds, uncommitted sandbox work is lost. |
| SSH | Attempts to stop the remote session runtime, but the remote task directory remains. Audit and remove retained task directories manually after confirming that no session needs them. |

The archive confirmation is enabled by default at **Settings → General → Task Actions → Archive Confirmation** under **Confirm before archiving tasks**. If a parent has children, **Also archive _N_ subtasks** is unchecked by default; without it, the children remain active. Task MCP archive/delete operations affect only the selected task and do not offer the cascade checkbox. MCP delete also does not reparent direct children the way the UI's non-cascade delete does; use the UI rather than task MCP to delete a parent that still has children.

To restore a task, open **List**, enable **Show archived**, and choose unarchive. If the parent was archived with its children, the cascade-owned children are restored with it. For worktree tasks, Kandev probes the newest historical worktree branch for each repository. If that branch still exists locally or on `origin`, Kandev restores it as the checkout branch so the next session can pick it up. Recovery is best-effort and does not rewrite ambiguous multi-row attachments for the same repository. If the branch is missing, the unarchive toast warns that the next session starts fresh from the base branch; work that existed only on the deleted local branch is unrecoverable. Removed worktree directories, containers, and sandboxes are materialized again on a later launch rather than resumed in place.

Delete is permanent. If **Also delete _N_ subtasks** is left unchecked, direct children become root tasks. If selected, descendants are deleted. The operation cannot be undone, and executor cleanup follows the same asynchronous, best-effort rules as archive.

## Troubleshooting

- **No workflow is available:** open the workspace's **Workflows** page. Newly added workspaces have none by default.
- **No agent starts:** the empty-description **Start Plan Mode** path does not use the normal start-agent submission. To begin an agent immediately, enter a description and use **Start task** or **Start task in plan mode**; also confirm the selected profiles are healthy and compatible.
- **Task starts in the wrong step:** normal creation uses **Start step** with first-step fallback; **Start task in plan mode** deliberately uses the first positional step.
- **A task moves unexpectedly:** inspect **On Turn Start**, **On Turn Complete**, child completion, entry actions, and the destination step's entry actions.
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
