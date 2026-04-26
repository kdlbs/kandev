---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Autonomous Agent Management

## Why

Kandev users today manually trigger every task execution, monitor each agent individually, and shepherd work through the kanban board one task at a time. Users managing multiple repositories and dozens of tasks spend more time orchestrating agents than reviewing their output. There is no way for agents to work independently across tasks, delegate work to other agents, run recurring jobs, or track spending -- all of which are table-stakes for autonomous multi-agent workflows.

Orchestrate adds an autonomy layer on top of kandev's existing task system. A coordinator agent manages a fleet of worker agents, picks up tasks, delegates subtasks, tracks costs, and reports progress. The user decides when to let agents run autonomously and when to drill into a task for low-level details (git changes, file tree, PR status). The existing kanban board and task detail pages remain unchanged.

## What

### New top-level page

- A new route at `/orchestrate` is accessible from a top-level navigation link on the kandev homepage.
- The `/orchestrate` page has its own full-replacement sidebar (replaces the default sidebar when on `/orchestrate/*` routes).
- The sidebar replicates a dedicated navigation structure:
  - **Workspace switcher**: at the top of the sidebar, showing the current workspace name. Dropdown to switch between workspaces (company/workspace selector dropdown).
  - **Top actions**: New Issue, Dashboard, Inbox
  - **Work**: Issues, Routines
  - **Projects**: expandable project list with `+` to create
  - **Agents**: expandable agent list with `+` to create. Each entry shows a status dot and channel indicators (Telegram, Slack icons) if the agent has configured channels.
  - **Company**: Org, Skills, Costs, Activity, Settings

### Sub-pages

| Route | Purpose |
|-------|---------|
| `/orchestrate` | Dashboard: agent status cards, run activity chart (last 14 days), agents enabled count with status breakdown, recent activity feed |
| `/orchestrate/inbox` | Pending approvals, budget alerts, agent errors, items requiring human review |
| `/orchestrate/issues` | Issues list with hierarchical tree view, view modes (list/board), toolbar (search, filters, sort, group, column picker, nesting toggle) |
| `/orchestrate/issues/[id]` | Task detail - simple mode (default): breadcrumb, description, properties panel, chat/activity tabs, sub-issues. Toggle to advanced mode. |
| `/orchestrate/issues/[id]?mode=advanced` | Task detail - advanced mode: kandev dockview layout within orchestrate chrome (chat, terminal, plan, files, changes). Auto-launches ACP session (idle until user sends message). |
| `/orchestrate/routines` | Routine definitions, run history, enable/disable toggles |
| `/orchestrate/projects` | Project list with task counts, budget usage, status |
| `/orchestrate/projects/[id]` | Single project detail with task list, agents, budget |
| `/orchestrate/agents` | Agent instance cards: name, role, status, skills, budget, current task |
| `/orchestrate/agents/[id]` | Agent detail with tabs: Overview, Skills, Runs, Memory, Channels |
| `/orchestrate/company/skills` | Skill catalog CRUD |
| `/orchestrate/company/costs` | Cost explorer with breakdowns by agent/project/model/time |
| `/orchestrate/company/org` | Org chart: visual tree of agent hierarchy (`reports_to` relationships). Interactive node cards showing icon, name, role, adapter type, status dot. Zoom/pan/fit controls. Click a node to open agent detail. |
| `/orchestrate/company/activity` | Full audit log with filtering |
| `/orchestrate/company/settings` | Global orchestrate configuration: approval defaults, budget defaults, config source repo, import/export |

### Issues list (`/orchestrate/issues`)

- Hierarchical tree view showing parent/child task relationships with collapsible nesting.
- View modes:
  - **List** (default): rows with status icon, identifier (KAN-1), title, timestamp. Nesting toggle for parent/child tree.
  - **Board**: kanban columns grouped by status.
- Toolbar: `[+ New Issue] [Search]  |  [List/Board toggle] [Nesting] [Columns] [Filters] [Sort] [Group]`
- Filters: status, priority, assignee, project, labels.
- Sort: status, priority, title, created, updated (asc/desc).
- Group by: status, priority, assignee, project, parent, none.
- Column picker: status, identifier, assignee, project, labels, updated.
- Click a row to open the task detail page.

### Task detail - simple mode (`/orchestrate/issues/[id]`)

The default view when opening a task from the issues list. Issue-tracker-style layout:

- **Header**: breadcrumb (Issues > Parent > Task), identifier + status icon + project badge, copy/menu buttons.
- **Main content area** (left):
  - Title (editable).
  - Description (rendered markdown, editable).
  - Action buttons: + New Sub-Issue, Upload attachment, + New document.
  - **Tabs**: Chat | Activity.
    - Chat: comment thread showing agent run transcripts (with collapsible tool call details) and user/agent comments. Input box for posting comments.
    - Activity: timeline of status changes, assignments, approvals.
  - Sub-issues section: same list/board toolbar as the main issues page, scoped to children of this task.
- **Properties panel** (right sidebar, collapsible):
  - Status (dropdown), Priority (dropdown), Labels (multi-select).
  - Assignee (agent or user), Project, Parent (link to parent task).
  - Blocked by (multi-select), Blocking (read-only).
  - Sub-issues (list with + Add sub-issue).
  - Reviewers (multi-select: agents/users), Approvers (multi-select: agents/users).
  - Created by, Started, Completed, Created, Updated timestamps.
- **Toggle to advanced mode**: button/link that switches to the dockview layout.

### Task detail - advanced mode (`/orchestrate/issues/[id]?mode=advanced`)

For users who want to micro-manage a task with full kandev tooling:

- Layout within orchestrate chrome (sidebar and topbar remain):
  ```
  | orchestrate sidebar | orchestrate topbar                                              |
                        | dockview tabs (chat, terminal, plan, etc.) | right sidebar (files, changes) |
  ```
- This is a fixed dockview layout (no layout presets, no left task list sidebar).
- Reuses existing kandev dockview components: chat panel, terminal, plan panel, file tree, changes panel.
- **Auto-launches ACP session**: on entering advanced mode, the agent's ACP session is started/resumed (no prompt sent, no tokens consumed). The agent is ready and idle until the user sends a message.
- Both session types coexist on the same task: one-shot heartbeat runs from the scheduler + interactive sessions from advanced mode.
- When the user leaves advanced mode (toggles back to simple or navigates away), the session stays open and can be resumed later.
- Toggle back to simple mode via button/link.

### New issue dialog

- Modal triggered by "+ New Issue" button (from issues list or sidebar).
- Fields:
  - Title (auto-expanding textarea).
  - Quick selector row: "For [Assignee] in [Project]" with overflow menu (three dots) to add Reviewer and Approver.
  - Description (markdown editor).
  - Bottom bar chips: Status (default: Todo), Priority, Upload, more options.
  - Footer: Discard Draft | Create Issue button.
- Draft auto-saved to localStorage.
- When creating from a parent task context, shows a "sub-issue of KAN-X" badge.

### Relationship to existing features

- Orchestrate tasks ARE kandev tasks. The existing `Task` model is extended with new fields (`assignee_agent_instance_id`, `origin`, `project_id`). No separate task table.
- Clicking a task in `/orchestrate/issues` opens the orchestrate task detail page (simple mode). Advanced mode provides the full kandev dockview experience within orchestrate chrome.
- The existing kanban board at `/` continues to work. Users can use kanban without Orchestrate. Orchestrate-managed tasks appear on the kanban board like any other task.
- Existing task sessions, turns, messages, executors, and worktrees are reused. Orchestrate creates sessions through the same orchestrator pipeline.

### Frontend architecture

- A new Zustand slice `orchestrate` in `lib/state/slices/orchestrate/` holds agent instances, projects, routines, approvals, activity log, cost summaries, and wakeup queue status.
- The slice follows the existing pattern: SSR fetch -> hydrate store -> components read store -> hooks subscribe via WS.
- WS subscriptions use the existing gateway with new event types for orchestrate-specific events.

## Scenarios

- **GIVEN** a user on the kandev homepage, **WHEN** they click the "Orchestrate" link in the top navigation, **THEN** they see the Orchestrate dashboard with agent status cards, run activity chart, and recent activity feed. The sidebar shows the Orchestrate navigation instead of the default sidebar.

- **GIVEN** a user on `/orchestrate/issues`, **WHEN** they click a task row, **THEN** they see the task detail in simple mode: description, properties panel, chat/activity tabs, sub-issues section.

- **GIVEN** a user viewing a task in simple mode, **WHEN** they click "Advanced Mode", **THEN** the layout switches to the kandev dockview (chat, terminal, plan, files, changes) within the orchestrate sidebar and topbar. The ACP session is auto-started/resumed (idle, no tokens consumed until the user sends a message).

- **GIVEN** a user in advanced mode, **WHEN** they toggle back to simple mode, **THEN** the dockview layout is replaced with the simple view. The ACP session stays open for later resumption.

- **GIVEN** a user on `/orchestrate`, **WHEN** they click the kandev logo or a "Back to Board" link, **THEN** they return to `/` with the default sidebar restored.

- **GIVEN** a task created by Orchestrate (origin=agent or origin=routine), **WHEN** the user opens the kanban board, **THEN** the task appears in the appropriate workflow step alongside manually-created tasks.

- **GIVEN** a user clicking "+ New Issue", **WHEN** the dialog opens, **THEN** they see title, "For [Assignee] in [Project]", description editor, and a three-dot menu to add Reviewer and Approver participants.

## Out of scope

- Multi-user permissions and role-based access control within Orchestrate.
- Cross-workspace orchestration (agent instances are scoped to one workspace).
- Mobile/responsive layout for the Orchestrate pages (desktop-first).
- Migration of existing tasks into Orchestrate-managed tasks (users opt in per task).

## Related specs

- [orchestrate-agents](../orchestrate-agents/spec.md) -- agent instances, hierarchy, permissions
- [orchestrate-skills](../orchestrate-skills/spec.md) -- skill registry and agent skills
- [orchestrate-scheduler](../orchestrate-scheduler/spec.md) -- wakeup queue and heartbeat scheduler
- [orchestrate-costs](../orchestrate-costs/spec.md) -- cost tracking and budget management
- [orchestrate-routines](../orchestrate-routines/spec.md) -- recurring scheduled tasks
- [orchestrate-inbox](../orchestrate-inbox/spec.md) -- inbox, approvals, activity log
- [orchestrate-projects](../orchestrate-projects/spec.md) -- projects
- [orchestrate-assistant](../orchestrate-assistant/spec.md) -- personal assistant, channels, agent memory
- [orchestrate-config](../orchestrate-config/spec.md) -- configuration portability and repository sync
