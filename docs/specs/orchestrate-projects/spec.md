---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Projects & Goals

## Why

Kandev tasks live on a flat kanban board organized by workflow steps. There is no way to group tasks into a larger initiative ("Project Alpha: migrate to new API"), track progress across related tasks, or align work to high-level objectives. When agents create dozens of subtasks autonomously, the user loses the forest for the trees. Without projects and goals, there is no rollup view of what the team of agents is working toward.

Projects group tasks into manageable units with their own budget and progress tracking. Goals provide the high-level "why" that projects serve.

## What

### Projects

- A project is a container for related tasks.
- Project fields:
  - `id`: unique identifier.
  - `workspace_id`: scoped to workspace.
  - `name`: human-readable label (e.g. "API v2 Migration", "Q2 Security Hardening").
  - `description`: detailed description of the project scope and objectives.
  - `status`: `active`, `completed`, `on_hold`, `archived`.
  - `goal_id`: optional link to a goal this project serves.
  - `lead_agent_instance_id`: optional agent instance responsible for this project (typically the CEO or a team lead).
  - `color`: for UI display (sidebar dot, progress bars).
  - `budget_cents`: optional project-level budget (see [orchestrate-costs](../orchestrate-costs/spec.md)).
  - `created_at`, `updated_at`.

### Task-project relationship

- The existing `Task` model gains an optional `project_id` field.
- A task belongs to at most one project.
- Tasks without a `project_id` are "unprojectized" -- they work as they do today on the kanban board.
- The CEO and worker agents can assign tasks to projects during creation.
- Users can move tasks between projects or remove them from a project via the UI.

### Project views

- `/orchestrate/projects` shows a list of projects with:
  - Name, status, color indicator.
  - Task counts: total, in progress, completed, blocked.
  - Budget utilization (if budget set).
  - Lead agent name and status.
  - Progress bar (completed / total tasks).
- `/orchestrate/projects/[id]` shows a single project detail:
  - Description and status.
  - Task list filtered to this project, grouped by status.
  - Budget breakdown (total spend, remaining, by agent).
  - Agent instances working on this project's tasks.
  - Clickable tasks navigate to `/t/[taskId]`.
- The sidebar "Projects" section shows an expandable list of active projects with color dots and task counts. A "+" button creates a new project.

### Goals

- A goal is a high-level objective that one or more projects serve.
- Goal fields:
  - `id`: unique identifier.
  - `workspace_id`: scoped to workspace.
  - `title`: the objective (e.g. "Reduce API latency by 50%", "Ship v2.0 by end of Q2").
  - `description`: context and success criteria.
  - `status`: `active`, `completed`, `archived`.
  - `parent_id`: optional self-reference for goal hierarchy (company goal -> team goal -> sprint goal).
  - `owner_agent_instance_id`: optional agent instance responsible for this goal.
  - `created_at`, `updated_at`.

### Goal-project relationship

- A project has an optional `goal_id` linking it to a goal.
- Multiple projects can serve the same goal.
- Goal progress is derived from its linked projects' completion status.

### Goal views

- `/orchestrate/goals` shows:
  - Goal hierarchy as an indented list or tree view.
  - Each goal shows: title, status, linked project count, overall progress (derived from projects).
  - Click a goal to expand and see its linked projects.
- The sidebar "Goals" link navigates to this page.

### CEO integration

- The CEO's system prompt includes the current project and goal structure so it can:
  - Assign new tasks to the appropriate project.
  - Create new projects when work doesn't fit existing ones.
  - Report progress toward goals in its heartbeat summaries.
- The CEO does not create goals -- goals are set by the user as strategic direction.

## Scenarios

- **GIVEN** a user on `/orchestrate/projects`, **WHEN** they click "+" and enter "API v2 Migration" with a description and $50 budget, **THEN** the project appears in the list with status `active`, zero tasks, and a budget gauge.

- **GIVEN** a project "API v2 Migration" with 10 tasks (7 done, 2 in progress, 1 todo), **WHEN** the user views the project detail, **THEN** they see a 70% progress bar, task counts by status, and the task list grouped by status.

- **GIVEN** a CEO agent creating subtasks for a user request, **WHEN** the CEO determines the work fits the "API v2 Migration" project, **THEN** the created tasks have `project_id` set to that project. The project's task count increments.

- **GIVEN** a goal "Ship v2.0" with two linked projects (one 100% complete, one 50% complete), **WHEN** the user views the goals page, **THEN** the goal shows 75% overall progress derived from its projects.

- **GIVEN** a task assigned to a project with a budget, **WHEN** the task's agent sessions incur costs, **THEN** the costs roll up to both the agent instance budget and the project budget.

- **GIVEN** a user on the sidebar, **WHEN** they look at the "Projects" section, **THEN** they see active projects listed with color dots and task counts, and can click any project to navigate to its detail page.

## Out of scope

- Project templates (creating a project from a predefined template with pre-populated tasks).
- Project-level permissions (all users in the workspace can see and edit all projects).
- Gantt charts or timeline views for project scheduling.
- Cross-workspace project visibility.
- Milestones within a project (use goals or subtask grouping instead).
- Goal OKR-style key results with quantitative targets.
