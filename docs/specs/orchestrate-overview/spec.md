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
- The sidebar replicates Paperclip's navigation structure:
  - **Top actions**: New Issue, Dashboard, Inbox
  - **Work**: Issues, Routines, Goals
  - **Projects**: expandable project list with `+` to create
  - **Agents**: expandable agent list with `+` to create. Each entry shows a status dot and channel indicators (Telegram, Slack icons) if the agent has configured channels.
  - **Company**: Org, Skills, Costs, Activity, Settings

### Sub-pages

| Route | Purpose |
|-------|---------|
| `/orchestrate` | Dashboard: agent status cards, run activity chart (last 14 days), agents enabled count with status breakdown, recent activity feed |
| `/orchestrate/inbox` | Pending approvals, budget alerts, agent errors, items requiring human review |
| `/orchestrate/issues` | All orchestrate-managed tasks (assigned to agent instances). Click-through navigates to existing `/t/[taskId]` for detail |
| `/orchestrate/routines` | Routine definitions, run history, enable/disable toggles |
| `/orchestrate/goals` | Goal hierarchy with linked projects |
| `/orchestrate/projects` | Project list with task counts, budget usage, status |
| `/orchestrate/projects/[id]` | Single project detail with task list, agents, budget |
| `/orchestrate/agents` | Agent instance cards: name, role, status, skills, budget, current task |
| `/orchestrate/agents/[id]` | Agent detail with tabs: Overview, Skills, Runs, Memory, Channels |
| `/orchestrate/company/skills` | Skill catalog CRUD |
| `/orchestrate/company/costs` | Cost explorer with breakdowns by agent/project/model/time |
| `/orchestrate/company/activity` | Full audit log with filtering |
| `/orchestrate/company/settings` | Global orchestrate configuration |

### Relationship to existing features

- Orchestrate tasks ARE kandev tasks. The existing `Task` model is extended with new fields (`assignee_agent_instance_id`, `origin`, `project_id`). No separate task table.
- Clicking a task in `/orchestrate/issues` opens the existing `/t/[taskId]` page with its full session detail (messages, git, PR, file tree).
- The existing kanban board at `/` continues to work. Users can use kanban without Orchestrate. Orchestrate-managed tasks appear on the kanban board like any other task.
- Existing task sessions, turns, messages, executors, and worktrees are reused. Orchestrate creates sessions through the same orchestrator pipeline.

### Frontend architecture

- A new Zustand slice `orchestrate` in `lib/state/slices/orchestrate/` holds agent instances, projects, goals, routines, approvals, activity log, cost summaries, and wakeup queue status.
- The slice follows the existing pattern: SSR fetch -> hydrate store -> components read store -> hooks subscribe via WS.
- WS subscriptions use the existing gateway with new event types for orchestrate-specific events.

## Scenarios

- **GIVEN** a user on the kandev homepage, **WHEN** they click the "Orchestrate" link in the top navigation, **THEN** they see the Orchestrate dashboard with agent status cards, run activity chart, and recent activity feed. The sidebar shows the Orchestrate navigation instead of the default sidebar.

- **GIVEN** a user on `/orchestrate/issues`, **WHEN** they click a task row, **THEN** they navigate to `/t/[taskId]` and see the full task detail (sessions, messages, git, file tree) -- the same view as tasks opened from the kanban board.

- **GIVEN** a user on `/orchestrate`, **WHEN** they click the kandev logo or a "Back to Board" link, **THEN** they return to `/` with the default sidebar restored.

- **GIVEN** a task created by Orchestrate (origin=agent or origin=routine), **WHEN** the user opens the kanban board, **THEN** the task appears in the appropriate workflow step alongside manually-created tasks.

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
- [orchestrate-projects](../orchestrate-projects/spec.md) -- projects and goals
- [orchestrate-assistant](../orchestrate-assistant/spec.md) -- personal assistant, channels, agent memory
- [orchestrate-config](../orchestrate-config/spec.md) -- configuration portability and repository sync
