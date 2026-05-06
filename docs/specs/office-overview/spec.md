---
status: draft
created: 2026-04-25
owner: cfl
---

# Office: Autonomous Agent Management

## Why

Kandev users today manually trigger every task execution, monitor each agent individually, and shepherd work through the kanban board one task at a time. Users managing multiple repositories and dozens of tasks spend more time orchestrating agents than reviewing their output. There is no way for agents to work independently across tasks, delegate work to other agents, run recurring jobs, or track spending -- all of which are table-stakes for autonomous multi-agent workflows.

Office adds an autonomy layer on top of kandev's existing task system. A coordinator agent manages a fleet of worker agents, picks up tasks, delegates subtasks, tracks costs, and reports progress. The user decides when to let agents run autonomously and when to drill into a task for low-level details (git changes, file tree, PR status). The existing kanban board and task detail pages remain unchanged.

## What

### New top-level page

- A new route at `/office` is accessible from a top-level navigation link on the kandev homepage.
- The `/office` page has its own full-replacement sidebar (replaces the default sidebar when on `/office/*` routes).
- The sidebar replicates a dedicated navigation structure:
  - **Workspace switcher**: at the top of the sidebar, showing the current workspace name. Dropdown to switch between workspaces (company/workspace selector dropdown).
  - **Top actions**: New Task, Dashboard, Inbox
  - **Work**: Tasks, Routines
  - **Projects**: expandable project list with `+` to create
  - **Agents**: expandable agent list with `+` to create. Each entry shows a status dot and channel indicators (Telegram, Slack icons) if the agent has configured channels.
  - **Company**: Org, Skills, Costs, Activity, Settings

### Sub-pages

| Route | Purpose |
|-------|---------|
| `/office` | Dashboard: agent status cards, run activity chart (last 14 days), agents enabled count with status breakdown, recent activity feed |
| `/office/inbox` | Pending approvals, budget alerts, agent errors, items requiring human review |
| `/office/tasks` | Tasks list with hierarchical tree view, view modes (list/board), toolbar (search, filters, sort, group, column picker, nesting toggle) |
| `/office/tasks/[id]` | Task detail - simple mode (default): breadcrumb, description, properties panel, chat/activity tabs, sub-tasks. Toggle to advanced mode. |
| `/office/tasks/[id]?mode=advanced` | Task detail - advanced mode: kandev dockview layout within office chrome (chat, terminal, plan, files, changes). Auto-launches ACP session (idle until user sends message). |
| `/office/routines` | Routine definitions, run history, enable/disable toggles |
| `/office/projects` | Project list with task counts, budget usage, status |
| `/office/projects/[id]` | Single project detail with task list, agents, budget |
| `/office/agents` | Agent instance cards: name, role, status, skills, budget, current task |
| `/office/agents/[id]` | Agent detail with tabs: Overview, Skills, Runs, Memory, Channels |
| `/office/company/skills` | Skill catalog CRUD |
| `/office/company/costs` | Cost explorer with breakdowns by agent/project/model/time |
| `/office/workspace/org` | Org chart: visual tree of agent hierarchy (`reports_to` relationships). Interactive node cards showing icon, name, role, adapter type, status dot. Zoom/pan/fit controls. L-shaped SVG edge connectors (vertical->horizontal->vertical). Click a node to open agent detail. Server-side PNG export for sharing. |
| `/office/company/activity` | Full audit log with filtering |
| `/office/company/settings` | Global office configuration: approval defaults, budget defaults, config source repo, import/export |

### Tasks list (`/office/tasks`)

- Hierarchical tree view showing parent/child task relationships with collapsible nesting.
- View modes:
  - **List** (default): rows with status icon, identifier (KAN-1), title, timestamp. Nesting toggle for parent/child tree.
  - **Board**: kanban columns grouped by status.
- Toolbar: `[+ New Task] [Search]  |  [List/Board toggle] [Nesting] [Columns] [Filters] [Sort] [Group]`
- Filters: status, priority, assignee, project, labels.
- Sort: status, priority, title, created, updated (asc/desc).
- Group by: status, priority, assignee, project, parent, none.
- Column picker: status, identifier, assignee, project, labels, updated.
- Click a row to open the task detail page.
- **Server-side search**: the search input queries the backend with full-text search on title, description, and identifier. SQLite FTS5 index for fast matching on large task lists. Client-side filtering is a fallback for quick filtering within loaded results.

### Task detail - simple mode (`/office/tasks/[id]`)

The default view when opening a task from the tasks list. Task-tracker-style layout:

- **Header**: breadcrumb (Tasks > Parent > Task), identifier + status icon + project badge, copy/menu buttons.
- **Main content area** (left):
  - Title (editable).
  - Description (rendered markdown, editable).
  - Action buttons: + New Sub-Task, Upload attachment, + New document.
  - **Tabs**: Chat | Activity.
    - Chat: comment thread showing agent run transcripts (with collapsible tool call details) and user/agent comments. Input box for posting comments.
    - Activity: timeline of status changes, assignments, approvals.
  - Sub-tasks section: same list/board toolbar as the main tasks page, scoped to children of this task.
- **Properties panel** (right sidebar, collapsible):
  - Status (dropdown), Priority (dropdown), Labels (multi-select).
  - Assignee (agent or user), Project, Parent (link to parent task).
  - Blocked by (multi-select), Blocking (read-only).
  - Sub-tasks (list with + Add sub-task).
  - Reviewers (multi-select: agents/users), Approvers (multi-select: agents/users).
  - Created by, Started, Completed, Created, Updated timestamps.
- **Toggle to advanced mode**: button/link that switches to the dockview layout.

### Task detail - advanced mode (`/office/tasks/[id]?mode=advanced`)

For users who want to micro-manage a task with full kandev tooling:

- Layout within office chrome (sidebar and topbar remain):
  ```
  | office sidebar | office topbar                                              |
                        | dockview tabs (chat, terminal, plan, etc.) | right sidebar (files, changes) |
  ```
- This is a fixed dockview layout (no layout presets, no left task list sidebar).
- Reuses existing kandev dockview components: chat panel, terminal, plan panel, file tree, changes panel.
- **Auto-launches ACP session**: on entering advanced mode, the agent's ACP session is started/resumed (no prompt sent, no tokens consumed). The agent is ready and idle until the user sends a message.
- Both session types coexist on the same task: one-shot heartbeat runs from the scheduler + interactive sessions from advanced mode. The "one-shot" granularity is a single agent turn, not the whole conversation — see [office-task-session-lifecycle](../office-task-session-lifecycle/spec.md) for the per-(task, agent) session model where each turn fires up the executor, runs to completion, and tears down (RUNNING → IDLE), preserving conversation state for the next wakeup via `session/load`.
- When the user leaves advanced mode (toggles back to simple or navigates away), the session stays open and can be resumed later.
- Toggle back to simple mode via button/link.

### New task dialog

- Modal triggered by "+ New Task" button (from tasks list or sidebar).
- Fields:
  - Title (auto-expanding textarea).
  - Quick selector row: "For [Assignee] in [Project]" with overflow menu (three dots) to add Reviewer and Approver.
  - Description (markdown editor).
  - Bottom bar chips: Status (default: Todo), Priority, Upload, more options.
  - Footer: Discard Draft | Create Task button.
- Draft auto-saved to localStorage.
- When creating from a parent task context, shows a "sub-task of KAN-X" badge.

### Relationship to existing features

- Office tasks ARE kandev tasks. The existing `Task` model is extended with new fields. No separate task table.
- Clicking a task in `/office/tasks` opens the office task detail page (simple mode). Advanced mode provides the full kandev dockview experience within office chrome.
- The existing kanban board at `/` continues to work. Users can use kanban without Office.
- Existing task sessions, turns, messages, executors, and worktrees are reused. Office creates sessions through the same orchestrator pipeline.

### Configuration storage

- The **database** is the source of truth for all office config (agents, skills, projects, routines, workspace settings) and runtime state.
- The **filesystem** (`~/.kandev/workspaces/<name>/`) is an optional sync target for git versioning, sharing, and backup. Users control import/export via the settings Sync UI.
- See [office-config](../office-config/spec.md) for the full sync model.

### Task model extensions

Office tasks extend the existing `Task` model with these changes:

**System office workflow:**
- Instead of making `workflow_id` nullable (which breaks the kanban board, task detail stepper, move operations, and dozens of queries), office tasks use a **system-created "Office" workflow** per workspace.
- The workflow is auto-created when office is enabled, with steps matching the office status lifecycle:
  - Backlog (position 0)
  - Todo (position 1, is_start_step)
  - In Progress (position 2)
  - In Review (position 3)
  - Blocked (position 4)
  - Done (position 5)
  - Cancelled (position 6)
- Office tasks get `workflow_id` = this system workflow and `workflow_step_id` = the step matching their current status.
- When office changes a task's status (e.g. agent completes -> in_review), it moves the task to the corresponding workflow step. This means the existing workflow engine, task detail stepper, and `/t/[taskId]` page all work unchanged.
- The Office workflow is **hidden from the homepage kanban board** by default. Office tasks are managed from `/office/tasks`, not the kanban. The kanban's workflow selector excludes workflows marked as `office_workflow_id` on any workspace.
- The Office workflow is visible in the settings workflow page as a system workflow (read-only). Users can view steps and customize colors, but cannot delete, rename steps, add/remove steps, or add step events.
- The Office workflow's step events are configured for office behavior (no on_enter auto_start_agent -- the scheduler handles that).

**Manual status changes:**
- Users change office task status from the `/office/tasks` detail page or via the office API. Since office status IS the workflow step, status changes trigger `task.moved` events.
- This lets users manually intervene (unblock a stuck task, manually complete, reject a review).
- The office event subscribers listen for `task.moved` events and fire the appropriate side effects:
  - Move to In Progress: if the task has an assignee agent, queue a `task_assigned` wakeup so the agent starts working.
  - Move to Done: check blocker dependencies, resolve them, fire `task_blockers_resolved` wakeups for newly-unblocked tasks. Check if parent's children are all terminal, fire `task_children_completed`.
  - Move to In Review: if the task has an execution_policy with reviewers, wake the reviewer agents.
  - Move from In Review to In Progress: treat as rejection, wake the assignee agent with context.
  - Move to Cancelled: same blocker/parent resolution as Done (cancelled is terminal).
- Side effects only fire for office tasks (those with `assignee_agent_instance_id` set). Non-office tasks moved on other workflows are unaffected.

**Task identifiers:**
- Each workspace has a `task_sequence` counter (integer, auto-incrementing).
- Each workspace has a `task_prefix` (string, default "KAN").
- On task creation, the task gets a human-readable `identifier` field: `{prefix}-{sequence}` (e.g. "KAN-1", "KAN-42").
- The identifier is immutable once assigned. The sequence is workspace-scoped.
- Only office tasks (those with a project or non-manual origin) get identifiers. Existing kanban tasks have a null identifier and continue to display by title/UUID as before. No backfill needed.

**Labels:**
- Tasks have a `labels` JSON array field (list of label strings, e.g. `["security", "frontend", "urgent"]`).
- Labels are free-form strings. No separate label registry table for now.

**Blockers:**
- A new `task_blockers` junction table stores `(task_id, blocker_task_id)` pairs.
- A task is `blocked` when it has unresolved blockers (any blocker not in `done`/`cancelled` state).
- Circular dependency detection on insert.

**Comments:**
- The existing session message system is used for agent-user communication within a session.
- Office adds a `task_comments` table for asynchronous comments (outside sessions): agent-to-agent, user notes, channel messages.
- Comment fields: `id`, `task_id`, `author_type` (user/agent), `author_id`, `body` (text), `source` (user/agent/channel), `reply_channel_id` (for channel relay), `created_at`.
- Comments trigger `task_comment` wakeups for the assigned agent.

**Other new fields on tasks:**
- `assignee_agent_instance_id` (nullable) -- which office agent owns this task.
- `origin` (enum: manual/agent_created/routine) -- how the task was created.
- `project_id` (nullable) -- which project this task belongs to.
- `requires_approval` (boolean, default false) -- shorthand for "add user as approver."
- `execution_policy` (JSON, nullable) -- multi-stage review/approval config.
- `execution_state` (JSON, nullable) -- current stage progress.

### Agent API authentication

- When the scheduler launches an agent session, it mints a per-run JWT (short-lived, scoped to the agent instance and task).
- The JWT is injected as `KANDEV_API_KEY` environment variable in the agent subprocess.
- Agents use this JWT as a bearer token when calling office API endpoints (memory, task updates, comments).
- The JWT encodes: `agent_instance_id`, `task_id`, `workspace_id`, `session_id`, `exp` (expiry).
- API endpoints validate the JWT and scope access: an agent can only access its own memory, its assigned tasks, and workspace-level read endpoints (skills, project list).
- The JWT generation reuses the existing per-run auth mechanism in the lifecycle manager.

### Frontend architecture

- A new Zustand slice `office` in `lib/state/slices/office/` holds agent instances, projects, routines, approvals, activity log, cost summaries, and wakeup queue status.
- The slice follows the existing pattern: SSR fetch -> hydrate store -> components read store -> hooks subscribe via WS.
- WS subscriptions use the existing gateway with new event types for office-specific events.

## Scenarios

- **GIVEN** a user on the kandev homepage, **WHEN** they click the "Office" link in the top navigation, **THEN** they see the Office dashboard with agent status cards, run activity chart, and recent activity feed. The sidebar shows the Office navigation instead of the default sidebar.

- **GIVEN** a user on `/office/tasks`, **WHEN** they click a task row, **THEN** they see the task detail in simple mode: description, properties panel, chat/activity tabs, sub-tasks section.

- **GIVEN** a user viewing a task in simple mode, **WHEN** they click "Advanced Mode", **THEN** the layout switches to the kandev dockview (chat, terminal, plan, files, changes) within the office sidebar and topbar. The ACP session is auto-started/resumed (idle, no tokens consumed until the user sends a message).

- **GIVEN** a user in advanced mode, **WHEN** they toggle back to simple mode, **THEN** the dockview layout is replaced with the simple view. The ACP session stays open for later resumption.

- **GIVEN** a user on `/office`, **WHEN** they click the kandev logo or a "Back to Board" link, **THEN** they return to `/` with the default sidebar restored.

- **GIVEN** a task created by Office (origin=agent or origin=routine), **WHEN** the user opens the homepage kanban board, **THEN** the task does not appear — office tasks are managed from `/office/tasks`, not the kanban. The kanban's workflow selector does not list office workflows.

- **GIVEN** a user clicking "+ New Task", **WHEN** the dialog opens, **THEN** they see title, "For [Assignee] in [Project]", description editor, and a three-dot menu to add Reviewer and Approver participants.

## Out of scope

- Multi-user permissions and role-based access control within Office.
- Cross-workspace orchestration (agent instances are scoped to one workspace).
- Mobile/responsive layout for the Office pages (desktop-first).
- Migration of existing tasks into Office-managed tasks (users opt in per task).

## Related specs

- [office-agents](../office-agents/spec.md) -- agent instances, hierarchy, permissions
- [office-skills](../office-skills/spec.md) -- skill registry and agent skills
- [office-scheduler](../office-scheduler/spec.md) -- wakeup queue and heartbeat scheduler
- [office-costs](../office-costs/spec.md) -- cost tracking and budget management
- [office-routines](../office-routines/spec.md) -- recurring scheduled tasks
- [office-inbox](../office-inbox/spec.md) -- inbox, approvals, activity log
- [office-projects](../office-projects/spec.md) -- projects
- [office-assistant](../office-assistant/spec.md) -- personal assistant, channels, agent memory
- [office-config](../office-config/spec.md) -- configuration portability and repository sync
