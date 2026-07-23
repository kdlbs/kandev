---
title: "Coordinate Work"
description: "Coordinate parallel sessions, subtasks, repositories, branches, messages, and human gates."
---

# Coordinate Work

Kandev can coordinate several agents without turning every unit of work into a separate task. Choose the smallest boundary that gives the work the isolation, workflow state, and review surface it needs.

## Choose a coordination boundary

| Need                                                                 | Use                                       | Filesystem relationship                    | Independent workflow state |
| -------------------------------------------------------------------- | ----------------------------------------- | ------------------------------------------ | -------------------------- |
| Another agent on the same files and goal                             | Additional named session                  | Shares the task environment                | No                         |
| A child deliverable that must see current uncommitted files          | Subtask with **Inherit parent workspace** | Shares the parent's materialized workspace | Yes                        |
| A child deliverable needing isolated files or a different repository | Subtask with **Create new workspace**     | Gets its own materialized environment      | Yes                        |
| One deliverable spanning repositories                                | Multi-repository task                     | Separate worktree per attachment           | No; one task workflow      |

Shared files make handoff cheap but allow concurrent edits to collide. Separate environments isolate file state, but only the executor determines whether host processes and credentials are also isolated.

## Run parallel sessions in one task

Use an additional session when agents need the same task, repository attachments, and materialized environment.

<DocsVideo
  webm="./media/feature-guides/parallel-named-sessions.webm"
  mp4="./media/feature-guides/parallel-named-sessions.mp4"
  poster="./media/feature-guides/parallel-named-sessions.webp"
  title="Coordinate parallel named sessions"
  caption="Two named sessions run in parallel while their responsibilities remain easy to distinguish."
/>

1. Open the task workbench.
2. Choose **Add panel (+) → New Agent**, or run **New Agent** from the command panel.
3. Select a compatible agent profile. Missing credentials link to the relevant **Settings → Executors** or **Settings → Agents** configuration.
4. Choose context:
   - **Blank** starts without copied conversation context.
   - **Copy initial prompt** copies the task's initial prompt.
   - **Summarize session** asks the configured utility agent to summarize the current session. It is unavailable without a utility agent.

5. Enter the required prompt and select **Start Agent**.

The dialog shows the environment, branch, and executor the session will share. Two sessions can edit the same files concurrently; assign files or phases explicitly.

Right-click a session tab to **Rename…**, **Set as Primary**, stop, resume, delete, share, use **Handoff** to another profile, or **Close Others**, when that action is available for the session state. Names are trimmed and limited to 120 characters. **Handoff** creates another session; it does not move the task or transfer its workflow state.

### Spawn a session from an agent

Task agents can call `spawn_session_kandev` with a prompt and optional `name`, `agent_profile_id`, and `task_id`.

- With no task ID, it adds a session to the current task.
- A target task must be in the same workspace as the spawning task.
- Profile selection prefers an explicit profile, then the spawner's profile for the same task, then the target task's primary-session profile.
- The spawned session receives hidden attribution context identifying the sender and reply route.
- A failure to apply the optional name does not fail an otherwise successful launch.

Use the returned task ID, session ID, and state for subsequent targeted messages.

## Create subtasks

A subtask is appropriate when the child needs its own title, sessions, workflow position, history, or pull request.

Open it from either:

- the current task action beside **New Task** in the sidebar (**New subtask of current task**); or
- the task workbench **Task** split menu → **Subtask**.

The dialog proposes `_Parent title_ / Subtask _N_`. Title and prompt are required, and **Create Subtask** starts the agent immediately. The subtask inherits the parent's workflow. Its profile starts from the active parent session's profile and can be changed, but the dialog does not filter profiles for executor compatibility.

Choose a workspace mode:

| Mode                         | Default and inheritance                                                                                                                                                                    | Use it when                                                               |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| **Inherit parent workspace** | Default when the parent has an active worktree. Reuses the parent's executor, repositories, worktrees, branch, and current uncommitted files. Repository and executor controls are hidden. | The child is a focused collaborator on the same file state.               |
| **Create new workspace**     | Default when the parent has no active worktree branch. Lets you choose another configured/discovered repository, folder, remote URL, branch, and executor.                                 | The child needs isolation, a different branch, or a different repository. |

The context choices are **Blank**, **Copy initial prompt**, and, when a utility agent is configured, **Summarize session**. Context supplies background; it does not create a shared conversation. Attachments and prompt enhancement are also available.

The subtask dialog currently does not enforce agent/executor credential compatibility. For an isolated multi-repository subtask, choose **Worktree**, **Local Docker**, **SSH**, or **Sprites**; Local/Local PC creation remains gated and Remote Docker is not implemented. For every subtask, choose an agent profile configured on the inherited or selected executor; otherwise creation can succeed while agent launch or repository materialization fails.

Regular Kanban allows one subtask level: a root task can have children, but a child cannot have another child. Split further work into sibling subtasks, additional sessions, or a separate top-level task. Arbitrary-depth trees belong to the in-progress Office surface.

### Detach a subtask

Open the subtask's action menu from its sidebar entry or Kanban card, choose **Detach from parent**, and confirm. The subtask becomes a top-level task without changing its workflow position, blockers, sessions, or descendants. The Office parent picker's **No parent** choice performs the same operation.

Detaching changes task hierarchy only. An inherited workspace remains shared with the former parent, and the confirmation dialog calls this out explicitly. Create a task with **Create new workspace** when the work needs isolated files or a separate branch.

### Create a subtask from an agent

Call `create_task_kandev` with `parent_id: "self"`. `workspace_mode` defaults to `inherit_parent`; set it to `new_workspace` for isolated materialization.

- `start_agent` defaults to `true`, so a description is required unless it is set to `false`.
- The tool inherits the parent workspace, workflow, profile, executor, repositories, and base branches unless overridden.
- Inherited repository attachments deliberately do not copy an explicit checkout branch.
- An explicit same-repository child uses the inherited base branch. An explicit cross-repository child defaults to that repository's default branch unless `base_branch` is supplied.
- Every created task must resolve an agent profile, even with `start_agent: false`.
- Profile precedence is explicit profile, parent/current/source task metadata or primary-session profile, destination-step profile override, workflow default, then workspace default. With no explicit workflow step, the destination is the workflow's start step.
- If no executor or executor profile is explicit or inherited, task MCP uses the built-in **git-worktree** executor. It does not consult the workspace's **Default Executor** for this fallback.
- The one-level Kanban depth rule still applies.
- An ephemeral Quick Chat task cannot be a parent; omit `parent_id` and create a top-level task instead.

For predictable top-level creation, pass `repository_url`, `repository_id`, or `local_path`, as the tool contract requests. In task mode, the backend currently inherits repositories from the calling source task when no locator is supplied; if that source is repository-free, it can create a repository-free task despite the stricter tool description. The regular UI's **None** source remains the explicit route for intentional repository-free work.

## Coordinate with targeted messages

`message_task_kandev` sends a message to another task's primary session by default. Supply a session ID to target a particular sibling session on the current task.

| Target state                | Result                                    |
| --------------------------- | ----------------------------------------- |
| Running or starting         | Queues the message FIFO for a later turn. |
| Waiting, idle, or completed | Starts a new turn immediately.            |
| Created but not started     | Starts the session with the message.      |
| Failed or cancelled         | Returns an error.                         |

The default delivery mode is queued. Each session accepts 10 queued messages by default; operators can change it with `KANDEV_QUEUE_MAX_PER_SESSION`, and a value of `0` or less disables the limit. Only one queued message drains per agent turn. When the cap is reached, the sender receives a structured `queue_full` error and should retry after a target turn completes.

Choose the control by intent:

| Intent                                                  | Operation                                                        | Result                                                                                                                                       |
| ------------------------------------------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Send information that can wait                          | `message_task_kandev` with queued delivery or no `delivery_mode` | The current turn continues and the message waits FIFO.                                                                                       |
| Stop the current approach and give replacement work now | `message_task_kandev` with `delivery_mode: "interrupt"`          | The direct parent requests immediate cancellation and redispatch. If that cannot proceed safely, the response reports the message as queued. |
| Halt all current work with no replacement prompt        | `stop_task_kandev`                                               | The direct parent requests logical cancellation and graceful teardown without creating or dispatching a message.                             |

Only a direct parent may interrupt its child. Halt-only stop is stricter: it accepts only a same-workspace direct child, while self, siblings, ancestors other than the parent, deeper descendants, unrelated tasks, and cross-workspace callers are rejected. Use interrupt for stop-and-steer work. Reserve stop for halt-only intent.

### Stop a direct child's work

`stop_task_kandev` accepts the full ID of one direct child and has no session-specific option. Kandev inspects that child's active-session candidates and requests a graceful stop for every execution still observed as live, including non-primary sibling sessions. It does not recurse into descendants.

For each accepted execution, Kandev persists the session as `CANCELLED` before scheduling runtime teardown. A `status: "stopped"` response confirms that logical state and scheduled teardown; cleanup continues asynchronously and the process may not have exited yet. When no live execution is accepted, the call succeeds idempotently with `status: "not_running"` and changes no task or session state.

After at least one accepted stop, Kandev attempts to move a regular, unarchived, non-Office task from `IN_PROGRESS` or `SCHEDULING` to `REVIEW`, provided no session remains working. Office, archived, and already non-active tasks keep their state, and a failed secondary `REVIEW` reconciliation does not undo accepted session stops.

Stopping preserves the task record, worktrees, environments, commits, descendants, and existing queued messages. It sends no prompt, creates no replacement turn, and does not create a durable pause: a later user or workflow action can start the task again.

Additional messaging boundaries:

- A task cannot message its own primary session through the default route, and a session cannot message itself.
- Normal targeted messages can cross workspaces when the sender has the exact task ID. Session spawning cannot.
- Sender metadata and content become part of the target conversation. Do not send secrets.
- Use bounded requests with the repository, branch, expected result, and reply target instead of treating messages as shared memory.

Use `get_task_conversation_kandev` to read a primary or explicit session conversation. It supports limits, before/after cursors, ascending or descending order, and message-type filters. Use `list_related_tasks_kandev` for the current or another same-workspace task to list its parent, direct children, siblings, stored blocker relationships, and associated GitHub pull requests.

Replies close the loop: the receiving agent calls `message_task_kandev` back with the originating task's ID, turning a one-way notification into a genuine bidirectional conversation. This enables multi-turn negotiations between agents — for example, agreeing on an API contract before both sides implement. See [Agent Communication](agent-communication.md) for delivery semantics, discovery patterns, and a worked negotiation example.

## Wait for child tasks

On the parent's current workflow step, configure **When Child Tasks Complete** to move next, previous, or to a selected step. It runs once when every active direct child is `COMPLETED`, `FAILED`, or `CANCELLED` and the parent has an active session.

It ignores archived and ephemeral children, ignores grandchildren, and does not run if the parent has no children. The qualifying parent-session states are `CREATED`, `STARTING`, `RUNNING`, and `WAITING_FOR_INPUT`; no session, `IDLE`, or a terminal session prevents the transition. Terminal child state means work stopped; it does not mean every child succeeded. Put a human Review step after the transition when failures, diffs, or pull requests must be inspected.

For a human gate, use **On Turn Complete → Do nothing (wait for user)** and require a person to review before moving the task or sending the next instruction. `step_complete_kandev` proves only that the agent emitted the configured completion signal.

## Work across repositories and branches

### Create a multi-repository task

In **New Task**, add more **Repo** or **Remote** rows and select a base branch for each. Multi-repository creation supports **Worktree**, **Local Docker**, **SSH**, and **Sprites**. Local/Local PC creation remains gated while its initial-launch path cannot project sibling repositories, and Remote Docker is not implemented. Kandev scopes Changes, review, and pull-request surfaces by repository.

Before starting, document:

- which repository owns each deliverable;
- the base and pull-request target for each attachment;
- which remote credential can fetch and push each repository;
- the merge order for dependent changes; and
- the test command required in each repository.

### Add sources after creation

For an idle, non-archived repository-backed task, use **Files → Workspace actions → Add sources**. Its **Local** mode shares task creation's saved/discovered local-repository selector and offers local folders when the executor supports them; its **Remote** mode uses provider-backed repositories or pasted repository URLs. Add multiple mixed sources together; validation, persistence, and materialization are atomic. Desktop uses a dialog and phones use a full-height drawer.

Repository sources work on **Worktree**, **Local/Local PC**, **Local Docker**, **SSH**, and **Sprites**. Folder sources are live host grants and work only on **Worktree** and **Local/Local PC**; they are unavailable to Docker and remote runtimes. Remote Docker remains unimplemented. Local Git sources need a cloneable origin on Docker, SSH, and Sprites; Worktree and Local/Local PC can use host repositories directly.

Use a base branch for every repository attachment. Worktree, Docker, SSH, and Sprites can materialize an optional existing checkout branch for repository sources. Local/Local PC never changes the user-owned repository checkout.

Kandev rejects the entire request if a source is invalid, duplicated, inaccessible, or cannot be materialized. A failure removes new source records and Kandev-owned materialization; existing task contents remain intact. Attachments persist across reload, relaunch, and reset. A missing persisted folder is surfaced during a new or reset environment rather than silently omitted.

Task agents can call `add_workspace_sources_kandev` with the same mixed batch; `task_id` defaults to the current task. `add_branch_to_task_kandev` remains available for its legacy one-repository/branch input. Neither operation can run during an active turn or tool call.

Use `update_repository_base_branch_kandev` with a task-repository ID to change the comparison base. The database update is authoritative. Resetting cached session bases, refreshing Changes, base commit, ahead/behind counts, and cumulative diff in a live tracker are best-effort side effects; a failure is logged without rolling back the new base, and the persisted value is rebuilt on the next session launch. The tool does not rewrite commits, switch the checkout, or change an existing pull request's target branch.

Separate worktrees isolate file state. They do not by themselves isolate host credentials, ports, background processes, or other machine resources.

## Keep durable coordination state

Regular tasks have one versioned plan. Use the parent plan for the overall breakdown, then put each child's precise scope and file ownership in that child's description. A subtask does not share a second live copy of the parent plan.

Before handing work to another session or task, record:

- completed work and remaining scope;
- repository, checkout branch, and relevant commit;
- tests run and exact failures;
- pull-request state and required reviewers;
- blockers and expected input; and
- files another writer must not overwrite.

Commit before switching isolated branches when the repository process requires it. Conversation summaries are useful context, but plans, repository files, commits, and pull requests are the durable sources of truth.

## Coordinator-led and human-led patterns

A supported coordinator-led Kanban flow is:

1. Inspect the workspace, workflow, repository attachments, available profiles, and related tasks.
2. Record one parent plan with deliverables and ownership.
3. Create parallel sessions for shared-file work and subtasks for independent workflow state.
4. Use isolated subtask workspaces for concurrent writers or separate branches.
5. Send bounded targeted messages and monitor conversations and pull requests.
6. Let the direct-child completion event move the parent to Review.
7. Have a person inspect changes and merge results in the documented order.

The coordinator has only the tools, filesystem access, credentials, and permissions exposed by its agent and executor profiles. It cannot bypass branch protection, provider approval rules, or human workflow gates.

A human-led flow uses the same primitives but keeps **Do nothing** transitions at planning, review, or release steps. Humans can create each task, approve agent-proposed subtasks, restrict credentials to narrow profiles, and decide what merges or ships.

### Office dependencies and deeper coordination

> [!EXPERIMENTAL]
> Office is feature-flagged, disabled in the production profile by default, and still in progress. Its dependency editor, persistent coordinators, deeper task trees, quorum rules, and routines are not stable regular-Kanban features.

Regular Kanban does not currently expose a dependency editor or blocker filter. Stored blocker data can appear through related-task MCP, but it is not a regular-board planning surface. For supported ordering, use parent/child structure, **When Child Tasks Complete**, explicit messages, workflow gates, and pull-request review.

When Office is enabled, it prototypes **Blocked by** and **Blocking** properties with same-workspace, self-reference, and cycle validation. Treat these as an evaluation surface rather than a production coordination contract.

## Troubleshooting

- **New Agent has no usable profile:** configure credentials under **Settings → Agents** and a compatible executor under **Settings → Executors**.
- **Summarize session is missing:** configure a utility agent; otherwise use **Blank** or **Copy initial prompt**.
- **Parallel agents overwrite work:** stop one writer, assign disjoint files, or create an isolated subtask workspace.
- **Subtask cannot be created:** confirm title, prompt, compatible profile, workflow, and the one-level Kanban depth limit.
- **Subtask creates but its agent or repositories fail to start:** the dialog does not enforce agent/executor compatibility. Confirm the agent is configured on that executor; multi-repository creation supports Worktree, Local Docker, SSH, and Sprites.
- **Inherited subtask sees unexpected changes:** it intentionally shares the parent's materialized files and branch.
- **Message remains queued:** the target is busy and only one queued message drains per turn. Check for `queue_full` before retrying.
- **Interrupt is rejected:** only the target's direct parent may use interrupt delivery.
- **Stop is rejected:** `stop_task_kandev` is task-mode only and accepts only a same-workspace direct child of the caller.
- **Stop reports `stopped` but a process is still visible:** the response confirms logical cancellation and scheduled graceful teardown, not process exit.
- **Agent cannot spawn on another task:** `spawn_session_kandev` is same-workspace only; create a task or use a normal targeted message instead.
- **Parent does not advance after children finish:** the parent needs an active session in `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT` in addition to terminal direct children.
- **A multi-repository executor is disabled:** Local/Local PC creation is still gated, or Remote Docker is not implemented. Choose Worktree, Local Docker, SSH, or Sprites.
- **Add sources is rejected:** wait until the task has no active turn or tool call, check every source's locator and branch, and remove duplicate repository/branch pairs or folder paths. A Docker or remote task cannot attach a folder.
- **Changed base did not refresh immediately:** the database value is saved even when live tracker refresh fails; the next session launch rebuilds the comparison state.
- **Changed base did not retarget the PR:** the base-update tool changes Kandev's comparison context, not Git history or provider PR metadata.

Related: [Tasks and workflows](tasks-and-workflows.md), [Sessions and review](sessions-and-review.md), [Automation and MCP](automation-and-mcp.md), [Agents and profiles](agents-and-profiles.md), and [Executors](executors.md).
