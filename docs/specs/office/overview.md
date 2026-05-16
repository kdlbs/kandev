---
status: draft
created: 2026-04-25
owner: cfl
---

# Office: Overview

## Why

Kandev users today manually trigger every task execution, monitor each agent individually, and shepherd work through the kanban board one task at a time. There is no way for agents to work independently across tasks, delegate work, run recurring jobs, or roll progress up across related initiatives - all table-stakes for autonomous multi-agent workflows. Office adds an autonomy layer on top of kandev's existing task system: a coordinator agent manages a fleet of workers, picks up tasks, delegates subtasks, tracks costs, and reports progress. Users decide when to let agents run autonomously and when to drill into a single task for low-level details.

This spec is the top-level entry point for Office. It covers the workspace model, projects, configuration storage and sync, and the first-run onboarding wizard. Other Office surfaces (agents, skills, scheduler, costs, routines, inbox, assistant) live in sibling specs under `docs/specs/office/`.

## What

### Top-level page and navigation

- A new route at `/office` is accessible from a top-level navigation link on the kandev homepage.
- The `/office/*` routes use a full-replacement sidebar (replaces the default sidebar).
- The sidebar layout:
  - **Workspace switcher** at the top - dropdown to switch between workspaces.
  - **Top actions**: New Task, Dashboard, Inbox.
  - **Work**: Tasks, Routines.
  - **Projects**: expandable project list with `+` to create.
  - **Agents**: expandable agent list with `+` to create. Each entry shows a status dot and channel indicators (Telegram, Slack icons) if configured.
  - **Company**: Org, Skills, Costs, Activity, Settings.

### Sub-pages

| Route | Purpose |
|-------|---------|
| `/office` | Dashboard: agent status cards, run activity chart (14d), enabled count with status breakdown, recent activity feed |
| `/office/inbox` | Pending approvals, budget alerts, agent errors, items requiring human review |
| `/office/tasks` | Tasks list with hierarchical tree view, list/board modes, toolbar (search, filters, sort, group, columns, nesting) |
| `/office/tasks/[id]` | Task detail in simple mode (default): description, properties panel, chat/activity tabs, sub-tasks |
| `/office/tasks/[id]?mode=advanced` | Task detail in advanced mode: kandev dockview (chat, terminal, plan, files, changes) inside office chrome; auto-launches an idle ACP session |
| `/office/routines` | Routine definitions, run history, enable/disable toggles |
| `/office/projects` | Project list with task counts, budget usage, status |
| `/office/projects/[id]` | Single project detail: task list, agents, budget |
| `/office/agents` | Agent instance cards: name, role, status, skills, budget, current task |
| `/office/agents/[id]` | Agent detail tabs: Overview, Skills, Runs, Memory, Channels |
| `/office/company/skills` | Skill catalog CRUD |
| `/office/company/costs` | Cost explorer with breakdowns by agent/project/model/time |
| `/office/workspace/org` | Org chart: visual tree of agent hierarchy with zoom/pan/fit and PNG export |
| `/office/company/activity` | Full audit log with filtering |
| `/office/company/settings` | Global office configuration: approval defaults, budget defaults, config source repo, import/export |
| `/office/setup` | First-run onboarding wizard / FS import prompt (see Onboarding) |

### Tasks list and detail

- **List view** (default): rows with status icon, identifier (e.g. KAN-1), title, timestamp; nesting toggle for parent/child tree.
- **Board view**: kanban columns grouped by status.
- Toolbar: `[+ New Task] [Search]  |  [List/Board] [Nesting] [Columns] [Filters] [Sort] [Group]`. Filters: status, priority, assignee, project, labels. Sort: status/priority/title/created/updated asc/desc. Group by: status/priority/assignee/project/parent/none. Column picker: status/identifier/assignee/project/labels/updated.
- **Server-side search** queries the backend with full-text search on title, description, and identifier (SQLite FTS5). Client-side filtering is a fallback for loaded results.
- **Simple mode** is the default task detail view: breadcrumb, identifier + status + project badge, editable title and markdown description, Chat | Activity tabs (chat shows agent run transcripts with collapsible tool-call detail), sub-tasks section with the same toolbar scoped to children, and a collapsible right-hand properties panel (status, priority, labels, assignee, project, parent, blocked-by/blocking, sub-tasks, reviewers, approvers, timestamps).
- **Advanced mode** swaps the main area for the kandev dockview (chat, terminal, plan, files, changes) inside the office sidebar and topbar. Entering advanced mode starts or resumes the agent's ACP session (idle - no tokens consumed until the user sends a message). Leaving advanced mode keeps the session open for later resumption. Both one-shot heartbeat runs and interactive advanced-mode sessions can coexist on the same task; the per-(task, agent) session model is described in [office-task-session-lifecycle](../office-task-session-lifecycle/spec.md).

### New task dialog

- Modal triggered by "+ New Task" (from the tasks list or sidebar).
- Fields: title (auto-expanding textarea), quick selector row "For [Assignee] in [Project]" with an overflow menu to add Reviewer and Approver, markdown description, bottom-bar chips (Status default Todo, Priority, Upload, more options), footer (Discard Draft | Create Task).
- Drafts auto-save to localStorage. When creating from a parent task context, a "sub-task of KAN-X" badge is shown.

### Relationship to existing kandev features

- Office tasks ARE kandev tasks. The existing `Task` model is extended with new fields; no separate task table.
- The existing kanban board at `/` continues to work and can be used without Office.
- Existing sessions, turns, messages, executors, and worktrees are reused. Office creates sessions through the same orchestrator pipeline.

### Projects

- A project is a container for related tasks, scoped to one or more repositories or filesystem folders.
- Tasks can be unprojectized (`project_id` null) and continue to behave as today on the kanban.
- The CEO and worker agents can assign tasks to projects on creation; users can move tasks between projects or remove them via the UI.
- **Project repositories** are either a git URL (GitHub, GitLab, Bitbucket, any remote) or a local filesystem path. Tasks can target one repo (agent gets a single worktree) or several (multi-worktree session via the existing `TaskSessionWorktree[]` support). Repos are configured at the project level and inherited by tasks; tasks may target a subset.
- **Project executor configuration** is an optional override that defines how agent sessions run for tasks in this project (executor type, image, resource limits, worktree strategy, network policy, environment, prepare scripts). If unset, the workspace default executor is used.
- **Views**:
  - `/office/projects` lists projects with name, status, color, repo count and names, task counts (total/in-progress/completed/blocked), budget utilization, lead agent and status, and a progress bar.
  - `/office/projects/[id]` shows description, status, repos, task list filtered to this project (same list/board UI as `/office/tasks`), budget breakdown, agent instances on this project, and clickable rows that open the task detail.
  - The sidebar Projects section shows active projects with color dots, task counts, and a `+` to create.
- **CEO integration**: the CEO's system prompt includes current project structure so it can assign tasks to projects, create new projects when work doesn't fit, and pick the right repo for each task.
- **Cross-project delegation pattern**: features spanning multiple projects flow through the agent hierarchy using existing primitives (hierarchy, subtasks, blockers, `requires_approval`). Example: CEO -> CTO -> Analyst (analysis subtask with approval) -> per-repo worker tasks chained by blockers -> QA on a multi-worktree session -> SRE ship. No special schema required.

### Configuration storage and sync

Office uses a **DB-first model with filesystem sync**. The database is the source of truth for all config and runtime state; the filesystem (`~/.kandev/`) is an optional sync target for git versioning, sharing, and backup. The user controls when changes flow in either direction via a Sync UI - there is no automatic reconciliation.

Why DB-first:
- Accidental file deletion, parse errors, and git conflicts cannot break a running system.
- Cloud-ready: shared PostgreSQL for team/SaaS works without filesystem coordination.
- Atomic operations (budget checks, wakeup claims, approval flows) stay transactional.
- No fsnotify / reload / cache-invalidation complexity.
- Agent profiles continue to live in the existing kandev DB tables, referenced by ID.

Filesystem layout:

```
~/.kandev/
├── workspaces/                          # user config, git-syncable
│   ├── default/                         # first workspace (slug)
│   │   ├── kandev.yml                   # workspace settings
│   │   ├── agents/<name>.yml
│   │   ├── skills/<slug>/SKILL.md       # plus any skill assets
│   │   ├── routines/<name>.yml
│   │   └── projects/<name>.yml
│   └── my-team/                         # repo-backed workspace
│       ├── .git/
│       └── ... (same structure)
├── system/                              # bundled with kandev binary, read-only
│   └── skills/<slug>/SKILL.md
└── runtime/                             # ephemeral, generated at session time
    └── <workspace-slug>/
        ├── instructions/<agentId>/{AGENTS.md, HEARTBEAT.md}
        └── skills/<slug>/SKILL.md
```

- **`workspaces/`**: user config, git-syncable. One directory per workspace, named by immutable slug.
- **`system/`**: bundled with kandev binary, refreshed on upgrade.
- **`runtime/`**: per-workspace ephemeral cache. Generated from DB before agent sessions; safe to delete - rebuilt on next session.

**Workspace slugs** are generated once at creation time and never change. The display `name` is editable freely without moving directories or breaking paths. Sanitization: lowercase, replace spaces/underscores with hyphens, strip non-alphanumeric (except hyphens), collapse consecutive hyphens, trim leading/trailing hyphens, max 50 chars. If empty after sanitization: `workspace-<shortId>`. Duplicates: append `-2`, `-3`, etc. The first workspace created during onboarding gets slug `default`.

**Dual workspace creation**: when Office creates a workspace, it writes `kandev.yml` to the filesystem and a DB row in the existing `workspaces` table. Both the kanban board and Office see the same workspace. Filesystem config is authoritative for office entities (agents, skills, projects, routines); the DB row is authoritative for kanban state (task sequence, default executor, workflow ID).

**Sync UI** in the settings page shows:
- **Incoming (filesystem -> DB)**: diff against current DB - new (green +), modified (yellow ~), deleted (red -). User clicks "Review & Apply", previews details, then confirms.
- **Outgoing (DB -> filesystem)**: entities in DB but missing/different on disk. "Export to FS" writes files; for repo-backed workspaces the user can then `git add && git commit && git push`.

**Git integration** for repo-backed workspaces: setup via `git clone <repo-url> ~/.kandev/workspaces/<name>/`; pulling new config -> Sync UI shows incoming diff -> user applies; conflicts are resolved in the terminal then imported via Sync UI.

**Skill injection**: skills for agent sessions are written into the agent's worktree (CWD) before each session. Skill content can come from the DB (inline skills created via UI), the filesystem (imported from GitHub/skills.sh), or bundled (shipped with the kandev binary). All routes inject into the agent-specific skill path under the worktree. See [office-skills](../office-skills/spec.md).

**Agent profiles** (model, CLI flags, MCP servers) stay in the existing kandev `agent_profiles` DB table. Office agent instances reference profiles by ID. Filesystem export format:

```yaml
# agents/ceo.yml
name: CEO
role: ceo
agent_profile_id: "prof_abc123"
desired_skills: [memory, delegation-playbook]
```

**Export/import bundle**: export writes DB config entities to `~/.kandev/workspaces/<name>/` as YAML/markdown files (also available as a zip download). Import reads YAML files, shows a diff against DB, and applies approved changes. The preview shows what will change before applying (created, updated, deleted).

### Onboarding

When a user opens `/office`, the backend checks both DB and filesystem state via `GET /api/v1/office/onboarding-state` (not localStorage). No default workspace is auto-created on startup - the filesystem is only populated by explicit user action.

| State | DB workspaces | FS workspaces | Action |
|-------|------|-----|--------|
| Fresh install | 0 | 0 | Redirect to `/office/setup` -> 4-step wizard |
| Shared config | 0 | >=1 | Redirect to `/office/setup` -> import prompt |
| Normal | >=1 | any | Show dashboard |

**Adding additional workspaces**: the workspace rail has an "Add workspace" (+) button that navigates to `/office/setup?mode=new`. The setup page skips the onboarding-complete redirect when `mode=new` is present and shows the wizard. The FS import prompt is also shown when `mode=new` if unimported FS workspaces exist. After creating the new workspace, the user is redirected to `/office` with it selected.

**Import prompt** (shared-config state): shows "Existing configuration found - Found N workspace(s) on the filesystem. Import settings to get started?", lists the workspace names, and offers `[Import & Continue]` (creates DB rows for FS workspaces, runs the config sync import, marks onboarding complete) or `[Start Fresh]` (skip import, proceed to wizard).

**Onboarding wizard** is a full-page (not modal) 4-step flow:

1. **Welcome + Workspace** - workspace name (default "Default Workspace") and task prefix (default "KAN", explained as "Tasks will be numbered KAN-1, KAN-2, etc.").
2. **Create CEO Agent** - agent name (default "CEO"), agent profile dropdown (Claude, Codex, etc.), executor preference (Local / Docker / Sprites) with descriptions, budget (default $0 = unlimited).
3. **First Task (optional)** - title (placeholder "Explore the codebase and create an engineering roadmap"), optional description prefilled with a helpful default. `[Back] [Skip] [Next]`.
4. **Review & Launch** - summary card of what will be created. `[Back] [Create & Launch]`.

**On "Create & Launch"** the following are created in a single transaction:
1. Office workspace: `kandev.yml` on filesystem + DB row in `workspaces` + system office workflow (7 steps).
2. CEO agent instance with `role=ceo`, full permissions, linked profile, bundled skills (kandev-protocol, memory).
3. Agent runtime row `status=idle` in `office_agent_runtime`.
4. "Onboarding" project (auto) for the initial task.
5. First task if provided: assigned to the CEO, in the Onboarding project, `status=todo`; a `task_assigned` wakeup is enqueued so the scheduler picks it up.
6. Onboarding state marked completed in the DB.

After creation the user is redirected to `/office`.

**Returning users**: backend checks `onboarding_state` per user; if completed and no `mode=new`, skip wizard and show dashboard; if not completed, the wizard is shown again on next visit.

**Shared workspaces**: onboarding state is per-workspace, not per-user. If one team member completes onboarding, others see the completed workspace; additional users joining an existing workspace skip onboarding.

## Data model

### Workspace

- `id` (UUID, PK), `name` (display name, editable), `slug` (filesystem name, immutable; sanitization rules above), `task_sequence` (int, auto-incrementing), `task_prefix` (string, default "KAN"), `office_workflow_id` (FK to the system Office workflow), `default_executor` and other existing kandev workspace fields.

### Task (extensions to the existing `Task` model)

- `identifier` (string, e.g. "KAN-42"). Format `{prefix}-{sequence}`. Immutable once assigned. Workspace-scoped. Only office tasks (those with a project or non-manual origin) get an identifier; existing kanban tasks have a null identifier and continue to display by title/UUID. No backfill.
- `labels` (JSON array of free-form strings, e.g. `["security", "frontend", "urgent"]`). No separate label registry.
- `assignee_agent_instance_id` (nullable) - which office agent owns this task.
- `origin` (enum: `manual` | `agent_created` | `routine`) - how the task was created.
- `project_id` (nullable) - which project this task belongs to. A task belongs to at most one project.
- `requires_approval` (boolean, default false) - shorthand for "add user as approver".
- `execution_policy` (JSON, nullable) - multi-stage review/approval config.
- `execution_state` (JSON, nullable) - current stage progress.
- `workflow_id` set to the workspace's system Office workflow; `workflow_step_id` matches the current status.

### Office workflow (system-created per workspace)

Auto-created when office is enabled, with steps matching the office status lifecycle: Backlog (0), Todo (1, `is_start_step`), In Progress (2), In Review (3), Blocked (4), Done (5), Cancelled (6). Hidden from the homepage kanban by default (the kanban's workflow selector excludes workflows referenced by `office_workflow_id` on any workspace). Visible in the settings workflow page as a read-only system workflow - users may view steps and customize colors but cannot delete, rename steps, add/remove steps, or add step events. Step events are configured for office behavior (no `on_enter auto_start_agent` - the scheduler handles that).

### Project

- `id` (PK), `workspace_id` (FK, scoped to workspace), `name` (human-readable label), `description`, `status` (`active` | `completed` | `on_hold` | `archived`), `lead_agent_instance_id` (nullable, typically CEO or team lead), `color` (UI display), `budget_cents` (nullable, project-level budget - see [office-costs](../office-costs/spec.md)), `repositories` (list of git URLs or local FS paths), `executor_config` (JSON, optional), `created_at`, `updated_at`.

### `task_blockers` (junction table)

- `(task_id, blocker_task_id)` pairs. A task is `blocked` when any blocker is not in `done`/`cancelled` state. Circular dependency detection on insert.

### `task_comments`

- `id`, `task_id`, `author_type` (`user` | `agent`), `author_id`, `body` (text), `source` (`user` | `agent` | `channel`), `reply_channel_id` (nullable, for channel relay), `created_at`. Used for asynchronous comments outside sessions (agent-to-agent, user notes, channel messages). Inserting a comment fires a `task_comment` wakeup for the assigned agent. The existing session message system is used for agent-user communication within a session.

### What stays in SQLite vs. filesystem

Runtime/transactional tables (DB-only, not exported): `office_agent_runtime`, `office_wakeup_queue`, `office_cost_events`, `office_budget_policies`, `office_routine_triggers`, `office_routine_runs`, `office_routines`, `office_approvals`, `office_activity_log`, `office_channels` (secrets, not exportable), `task_blockers`, `task_comments`.

Config entities synced to filesystem: `office_agent_instances`, `office_skills`, `office_projects`, workspace settings, routines.

## API surface

### Onboarding

```
GET /api/v1/office/onboarding-state
  -> { completed: bool,
       workspaceId?: string,
       ceoAgentId?: string,
       fsWorkspaces: [{ name: string }] }   # only unimported FS workspaces

POST /api/v1/office/onboarding/complete
  body: { workspaceName, taskPrefix, agentName, agentProfileId,
          executorPreference, taskTitle?, taskDescription? }
  -> { workspaceId, agentId, taskId?, projectId }

POST /api/v1/office/onboarding/import-fs
  -> { workspaceIds: string[], importedCount: int }
```

The `complete` endpoint performs all creation in a single transaction. When `taskTitle` is provided, it creates a task assigned to the CEO and enqueues a `task_assigned` wakeup. It can be called multiple times to create additional workspaces.

The `import-fs` endpoint creates DB workspace rows for each FS workspace that doesn't already exist in the DB, runs `ApplyIncoming` config sync for each, and marks onboarding complete.

The `fsWorkspaces` field only includes workspaces on disk that are not already imported, so the setup page can show "N unimported workspaces found" when adding a new workspace.

### Frontend architecture

A new Zustand slice `office` in `lib/state/slices/office/` holds agent instances, projects, routines, approvals, activity log, cost summaries, and wakeup queue status. The slice follows the existing pattern: SSR fetch -> hydrate store -> components read store -> hooks subscribe via WS. WS subscriptions use the existing gateway with new event types for office-specific events.

### Agent API authentication

When the scheduler launches an agent session, it mints a per-run JWT (short-lived, scoped to the agent instance and task), injected as `KANDEV_API_KEY` in the agent subprocess. Agents use this JWT as a bearer token when calling office API endpoints (memory, task updates, comments). The JWT encodes `agent_instance_id`, `task_id`, `workspace_id`, `session_id`, `exp`. API endpoints validate it and scope access: an agent can only access its own memory, its assigned tasks, and workspace-level read endpoints (skills, project list). JWT generation reuses the existing per-run auth mechanism in the lifecycle manager.

## State machine

**Manual status changes** drive office task lifecycle. Because office status IS the workflow step, changing status triggers a `task.moved` event, which the office event subscribers translate into side effects. Side effects only fire for office tasks (those with `assignee_agent_instance_id` set); non-office tasks on other workflows are unaffected.

| Transition | Side effect |
|---|---|
| -> In Progress | If task has an assignee agent, enqueue a `task_assigned` wakeup so the agent starts. |
| -> Done | Check blocker dependencies, resolve them, fire `task_blockers_resolved` wakeups for newly-unblocked tasks. If all of a parent's children are terminal, fire `task_children_completed`. |
| -> In Review | If `execution_policy` has reviewers, wake the reviewer agents. |
| In Review -> In Progress | Treat as rejection - wake the assignee agent with context. |
| -> Cancelled | Same blocker/parent resolution as Done (cancelled is terminal). |

The CEO and workers move tasks between states by calling the same API the UI uses; manual user intervention (unblock, mark complete, reject review) uses the same path.

## Permissions

- Onboarding state is per-workspace. The first user to complete onboarding for a workspace sets it up; other workspace members skip the wizard and see the existing setup.
- All users in a workspace can see and edit all projects in that workspace.
- Agent JWTs scope agent access to their own memory, their assigned tasks, and workspace-level read endpoints (skills, project list).
- Workflow-engine permissions and approvals are governed by `requires_approval` and `execution_policy` on the task; the user always controls Sync (import/export) actions.

## Failure modes

- **Filesystem parse error on import**: the Sync preview shows the parse error for the bad file; other files can still be imported. The DB is unaffected unless the user explicitly applies a change.
- **Accidental on-disk deletion of a config file**: the outgoing Sync diff shows the entity as "missing on disk". The DB is unaffected; the user can re-export.
- **Git pull conflict on a repo-backed workspace**: the user resolves in the terminal, then imports via the Sync UI.
- **No automatic filesystem reconciliation**: nothing on disk changes config without an explicit user action.
- **Onboarding state detection**: backend-driven via `GET /api/v1/office/onboarding-state`; localStorage cannot accidentally suppress the wizard.

## Persistence guarantees

- The **database** is the source of truth for all office config and runtime state. Restarting kandev preserves agents, skills, projects, routines, workspace settings, wakeup queue, cost events, approvals, activity log, blockers, comments, and onboarding completion.
- The **filesystem** under `~/.kandev/workspaces/<slug>/` is a portable copy; deleting it does not break a running system - the next export rebuilds it.
- `~/.kandev/runtime/<slug>/` is ephemeral. It can be deleted at any time and is rebuilt from the DB on the next agent session.
- `~/.kandev/system/` ships with the kandev binary and is refreshed on upgrade.
- Workspace slugs are immutable; renaming a workspace's display name does not move directories or break paths.
- Task identifiers are immutable once assigned; the workspace `task_sequence` only advances forward.
- ACP sessions opened in advanced mode persist across visits - leaving advanced mode keeps the session open for later resumption.

## Scenarios

- **GIVEN** a user on the kandev homepage, **WHEN** they click the "Office" link in the top navigation, **THEN** they see the Office dashboard with agent status cards, run activity chart, and recent activity feed. The sidebar shows the Office navigation instead of the default sidebar.

- **GIVEN** a user on `/office/tasks`, **WHEN** they click a task row, **THEN** they see the task detail in simple mode: description, properties panel, chat/activity tabs, sub-tasks section.

- **GIVEN** a user viewing a task in simple mode, **WHEN** they click "Advanced Mode", **THEN** the layout switches to the kandev dockview (chat, terminal, plan, files, changes) within the office sidebar and topbar. The ACP session is auto-started/resumed (idle, no tokens consumed until the user sends a message).

- **GIVEN** a user in advanced mode, **WHEN** they toggle back to simple mode, **THEN** the dockview layout is replaced with the simple view and the ACP session stays open for later resumption.

- **GIVEN** a task created by Office (origin=agent or origin=routine), **WHEN** the user opens the homepage kanban board, **THEN** the task does not appear; office tasks are managed from `/office/tasks`. The kanban's workflow selector does not list office workflows.

- **GIVEN** a user clicking "+ New Task", **WHEN** the dialog opens, **THEN** they see title, "For [Assignee] in [Project]", description editor, and a three-dot menu to add Reviewer and Approver participants.

- **GIVEN** a user on `/office/projects`, **WHEN** they click "+" and enter "API v2 Migration" with two repositories (github.com/team/backend, github.com/team/frontend) and a $50 budget, **THEN** the project appears in the list with status `active`, two repos listed, zero tasks, and a budget gauge.

- **GIVEN** a project with repos [backend, frontend], **WHEN** a user creates a task "Update auth endpoints" and selects the backend repo, **THEN** the task's agent session gets a worktree for the backend repo only.

- **GIVEN** a project with repos [backend, frontend], **WHEN** a user creates a task "Refactor shared types" and selects both repos, **THEN** the task's agent session gets worktrees for both repos in the same session.

- **GIVEN** a project with 10 tasks (7 done, 2 in progress, 1 todo), **WHEN** the user views the project detail, **THEN** they see a 70% progress bar, task counts by status, and the task list grouped by status.

- **GIVEN** a CEO agent creating subtasks for a user request, **WHEN** the CEO determines the work fits the "API v2 Migration" project and involves the backend repo, **THEN** the created task has `project_id` set and targets the backend repo.

- **GIVEN** a task assigned to a project with a budget, **WHEN** the task's agent sessions incur costs, **THEN** the costs roll up to both the agent instance budget and the project budget.

- **GIVEN** a user on the settings Sync page, **WHEN** new YAML files exist on disk that aren't in the DB, **THEN** the UI shows them as "incoming changes" with green + indicators and the user clicks "Review & Apply" to import.

- **GIVEN** a user who created agents via the UI, **WHEN** they click "Export to FS", **THEN** YAML files are written to disk for each agent and the user can `git add && git commit && git push`.

- **GIVEN** a team member who pulled new config via `git pull`, **WHEN** they open the Sync page, **THEN** the diff shows the changes from the repo and they apply them to their DB.

- **GIVEN** a user who accidentally deletes a YAML file on disk, **WHEN** they check the Sync page, **THEN** the outgoing diff shows the entity as "missing on disk", the DB is unaffected, and they can re-export.

- **GIVEN** a YAML file with parse errors, **WHEN** the user tries to import, **THEN** the import preview shows the parse error for that file and other files can still be imported.

- **GIVEN** a new user opening `/office` for the first time, **WHEN** no workspace exists on DB or FS, **THEN** they are redirected to `/office/setup` and see the 4-step wizard.

- **GIVEN** a user opening `/office` for the first time, **WHEN** no DB workspace exists but FS workspaces are found, **THEN** they are redirected to `/office/setup` and see the import prompt with workspace names listed.

- **GIVEN** a user on the import prompt, **WHEN** they click "Import & Continue", **THEN** all FS workspaces are imported to DB, onboarding is marked complete, and they are redirected to the dashboard.

- **GIVEN** a user on the import prompt, **WHEN** they click "Start Fresh", **THEN** the import is skipped and the 4-step wizard is shown.

- **GIVEN** a user on step 4 who clicks "Create & Launch" with a task title provided, **WHEN** all inputs are valid, **THEN** the workspace, CEO agent, Onboarding project, and first task are created, a `task_assigned` wakeup is enqueued, and the dashboard shows 1 agent enabled and 1 task in progress.

- **GIVEN** a user who skipped the first task on step 3, **WHEN** they reach the dashboard, **THEN** the CEO agent exists but is idle (no tasks) and the empty state says "Assign a task to your CEO to get started."

- **GIVEN** a returning user who already completed onboarding, **WHEN** they open `/office`, **THEN** they see the dashboard directly.

- **GIVEN** a user with an existing workspace, **WHEN** they click "Add workspace" in the workspace rail, **THEN** they see the setup wizard for a new workspace (not the dashboard redirect).

- **GIVEN** a user with 1 DB workspace and 2 unimported FS workspaces, **WHEN** they click "Add workspace", **THEN** the setup page shows the import prompt listing the 2 unimported workspaces with options to import them or start fresh.

## Out of scope

- Multi-user permissions and role-based access control within Office.
- Cross-workspace orchestration (agent instances are scoped to one workspace).
- Mobile / responsive layout for Office pages (desktop-first).
- Migration of existing tasks into Office-managed tasks (users opt in per task).
- Project templates (creating a project with predefined sub-tasks).
- Project-level permissions (all workspace users see and edit all projects).
- Gantt charts / timeline views for project scheduling.
- Cross-workspace project visibility.
- Automatic repository discovery (users manually add repos to projects).
- Automatic filesystem sync (user always controls import/export).
- Real-time collaborative editing of YAML files.
- Conflict-resolution UI for git merges (user resolves in terminal).
- Plugin system config sync.
- Onboarding template selection (Developer Team, Marketing Team, etc.).
- Agent instruction bundles beyond bundled system skills.
- Onboarding video / tutorial content.

## Related specs

- [office-agents](../office-agents/spec.md) - agent instances, hierarchy, permissions
- [office-skills](../office-skills/spec.md) - skill registry and CWD injection
- [office-scheduler](../office-scheduler/spec.md) - wakeup queue and heartbeat scheduler
- [office-costs](../office-costs/spec.md) - cost tracking and budget management
- [office-routines](../office-routines/spec.md) - recurring scheduled tasks
- [office-inbox](../office-inbox/spec.md) - inbox, approvals, activity log
- [office-assistant](../office-assistant/spec.md) - personal assistant, channels, agent memory
- [office-task-session-lifecycle](../office-task-session-lifecycle/spec.md) - per-(task, agent) session lifecycle
