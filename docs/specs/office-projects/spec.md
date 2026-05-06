---
status: draft
created: 2026-04-25
owner: cfl
---

# Office: Projects

## Why

Kandev tasks live on a flat kanban board organized by workflow steps. There is no way to group tasks into a larger initiative ("API v2 Migration"), track progress across related tasks, or scope work to specific repositories. When agents create dozens of subtasks autonomously, the user loses the forest for the trees. Without projects, there is no rollup view of what the team of agents is working toward, and no way to define which codebases a set of tasks operates on.

Projects group tasks into manageable units with their own repositories, budget, and progress tracking.

## What

### Projects

- A project is a container for related tasks, scoped to one or more repositories or filesystem folders.
- Project fields:
  - `id`: unique identifier.
  - `workspace_id`: scoped to workspace.
  - `name`: human-readable label (e.g. "API v2 Migration", "Q2 Security Hardening").
  - `description`: detailed description of the project scope and objectives.
  - `status`: `active`, `completed`, `on_hold`, `archived`.
  - `lead_agent_instance_id`: optional agent instance responsible for this project (typically the CEO or a team lead).
  - `color`: for UI display (sidebar dot, progress bars).
  - `budget_cents`: optional project-level budget (see [office-costs](../office-costs/spec.md)).
  - `repositories`: list of repository sources (see below).
  - `executor_config`: optional project-level executor configuration (see below).
  - `created_at`, `updated_at`.

### Project repositories

- A project has one or more repository sources that define the codebases agents work on.
- Each repository source is:
  - A **git repository URL** (GitHub, GitLab, Bitbucket, or any git remote): `github.com/team/backend`, `gitlab.com/team/frontend`.
  - A **local filesystem path**: `/home/user/docs`, `/opt/services/config`.
- When a task is created in a project, the user or agent selects which repository/repositories the task targets.
- Single-repo tasks: the agent gets a worktree for that repo (existing `TaskSessionWorktree` model).
- Multi-repo tasks: the agent gets worktrees for each targeted repo in the same session (existing multi-worktree support via `TaskSessionWorktree[]`).
- Repository sources are configured at the project level and inherited by tasks. Tasks can target a subset of the project's repos.

### Project executor configuration

- Each project has an optional `executor_config` JSON field defining how agent sessions run for tasks in this project.
- Fields:
  - `type`: executor type -- `local_pc` (standalone), `local_docker`, `sprites`, `remote_docker`.
  - `image`: Docker image (for `local_docker`, `remote_docker`).
  - `resource_limits`: `{ memory_mb, cpu_cores }` (for container executors).
  - `worktree_strategy`: `per_task` (default, each task gets its own branch/worktree) or `shared` (tasks share the project's working directory).
  - `network_policy`: `allow_all` (default), `allow_outbound`, `restricted`.
  - `environment_id`: reference to an existing kandev environment (Docker image, Dockerfile, etc.).
  - `prepare_scripts`: commands to run before the agent starts (e.g. `npm install`, `make build`).
- If not set, the workspace default executor is used.

### Task-project relationship

- The existing `Task` model gains an optional `project_id` field.
- A task belongs to at most one project.
- Tasks without a `project_id` are "unprojectized" -- they work as they do today on the kanban board.
- The CEO and worker agents can assign tasks to projects during creation.
- Users can move tasks between projects or remove them from a project via the UI.

### Project views

- `/office/projects` shows a list of projects with:
  - Name, status, color indicator.
  - Repository count and names.
  - Task counts: total, in progress, completed, blocked.
  - Budget utilization (if budget set).
  - Lead agent name and status.
  - Progress bar (completed / total tasks).
- `/office/projects/[id]` shows a single project detail:
  - Description, status, and repositories list.
  - Task list filtered to this project (same list/board UI as `/office/tasks`).
  - Budget breakdown (total spend, remaining, by agent).
  - Agent instances working on this project's tasks.
  - Clickable tasks navigate to `/office/tasks/[taskId]`.
- The sidebar "Projects" section shows an expandable list of active projects with color dots and task counts. A "+" button creates a new project.

### CEO integration

- The CEO's system prompt includes the current project structure so it can:
  - Assign new tasks to the appropriate project.
  - Create new projects when work doesn't fit existing ones.
  - Select the right repository for each task based on the project's repo list.

## Scenarios

- **GIVEN** a user on `/office/projects`, **WHEN** they click "+" and enter "API v2 Migration" with two repositories (github.com/team/backend, github.com/team/frontend) and a $50 budget, **THEN** the project appears in the list with status `active`, two repos listed, zero tasks, and a budget gauge.

- **GIVEN** a project with repos [backend, frontend], **WHEN** a user creates a task "Update auth endpoints" and selects the backend repo, **THEN** the task's agent session gets a worktree for the backend repo only.

- **GIVEN** a project with repos [backend, frontend], **WHEN** a user creates a task "Refactor shared types" and selects both repos, **THEN** the task's agent session gets worktrees for both repos in the same session.

- **GIVEN** a project with 10 tasks (7 done, 2 in progress, 1 todo), **WHEN** the user views the project detail, **THEN** they see a 70% progress bar, task counts by status, and the task list grouped by status.

- **GIVEN** a CEO agent creating subtasks for a user request, **WHEN** the CEO determines the work fits the "API v2 Migration" project and involves the backend repo, **THEN** the created task has `project_id` set and targets the backend repo.

- **GIVEN** a task assigned to a project with a budget, **WHEN** the task's agent sessions incur costs, **THEN** the costs roll up to both the agent instance budget and the project budget.

- **GIVEN** a user on the sidebar, **WHEN** they look at the "Projects" section, **THEN** they see active projects listed with color dots and task counts, and can click any project to navigate to its detail page.

### Cross-project feature: delegation and analysis-before-execution

This scenario demonstrates how a feature spanning multiple projects flows through the agent hierarchy. No special schema is needed -- it uses existing primitives (agent hierarchy, subtasks, blockers, `requires_approval`).

- **GIVEN** projects "Frontend" (repos: web-app, mobile-app) and "Backend" (repos: auth-service, api-gateway, user-service), **WHEN** the user asks "Add OAuth2 login across all services", **THEN**:

  1. **CEO** receives the task. The CEO does not analyze code -- it delegates technical work. It assigns the task to the **CTO** agent.

  2. **CTO** creates an analysis subtask: "Analyze OAuth2 impact across projects" with `requires_approval=true`, assigned to an **Analyst** agent. The analyst reads the relevant repos, determines which services need changes, identifies API contract implications, and posts findings as a comment. The task moves to `in_review` for user approval.

  3. **User approves** the analysis. The blocker resolves. The CTO reads the analysis output and creates execution subtasks scoped to specific projects and repos:
     - "Add OAuth2 provider to auth-service" -> Backend project, auth-service repo -> backend worker
     - "Add OAuth2 middleware to api-gateway" (blocked_by: auth-service task) -> Backend project, api-gateway repo -> backend worker
     - "Add OAuth2 flow to web-app" (blocked_by: api-gateway task) -> Frontend project, web-app repo -> frontend worker
     - "Add OAuth2 flow to mobile-app" (blocked_by: api-gateway task) -> Frontend project, mobile-app repo -> mobile worker
     - "Integration QA" (blocked_by: all build tasks) -> QA agent, multi-repo session across both projects
     - "Ship" (blocked_by: QA) -> SRE agent

  4. **Build subtasks** execute sequentially via blockers. Each worker agent operates in its specific repo's worktree.

  5. **QA agent** is woken when all builds complete. It targets multiple repos (multi-worktree session), reads the spec and build PR descriptions, runs cross-service validation (integration tests, API contract checks), and approves or rejects.

  6. **CTO** is woken via `task_children_completed` when all subtasks reach terminal state. It reports completion to the CEO.

Key roles in this pattern:
- **CEO**: company-level decisions, hiring, budget. Delegates technical work downward.
- **CTO / project manager**: technical decomposition, task creation, cross-project orchestration.
- **Analyst**: reads codebases, determines impact, proposes a plan before execution starts.
- **Workers**: execute specific tasks in specific repos.
- **QA**: validates changes across repos in an integration environment.

All roles are agent instances with different skills and instructions -- no special schema required.

## Out of scope

- Project templates (creating a project from a predefined template with pre-populated tasks).
- Project-level permissions (all users in the workspace can see and edit all projects).
- Gantt charts or timeline views for project scheduling.
- Cross-workspace project visibility.
- Automatic repository discovery (users manually add repos to projects).
