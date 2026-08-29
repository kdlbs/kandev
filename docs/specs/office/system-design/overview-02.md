---
status: draft
system: office
requirements:
  - REQ-OFFICE-OVERVIEW-001
created: 2026-04-25
owners:
  - cfl
---
# Office: Overview System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-OVERVIEW-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-OVERVIEW-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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

- **GIVEN** a new user opening `/office` for the first time, **WHEN** no workspace exists on DB or FS, **THEN** they are redirected to `/office/setup` and see the 5-step wizard.

- **GIVEN** a user opening `/office` for the first time, **WHEN** no DB workspace exists but FS workspaces are found, **THEN** they are redirected to `/office/setup` and see the import prompt with workspace names listed.

- **GIVEN** a user on the import prompt, **WHEN** they click "Import & Continue", **THEN** all FS workspaces are imported to DB, onboarding is marked complete, and they are redirected to the dashboard.

- **GIVEN** a user on the import prompt, **WHEN** they click "Start Fresh", **THEN** the import is skipped and the 5-step wizard is shown.

- **GIVEN** a user on the review step who clicks "Create & Launch" with the default first task still present, **WHEN** all inputs are valid, **THEN** the workspace, CEO agent, and setup task are created, a `task_assigned` wakeup is enqueued, and the dashboard shows 1 agent enabled and 1 task in progress.

- **GIVEN** a user who skipped the first task on step 4, **WHEN** they reach the dashboard, **THEN** the CEO agent exists but is idle (no tasks) and the empty state says "Assign a task to your CEO to get started."

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

- [Office agents](../requirements/agents.md) - agent instances, hierarchy, permissions
- [Office agents](../requirements/agents.md#skill-injection) - skill registry and CWD injection
- [Office scheduler](../requirements/scheduler.md) - wakeup queue and heartbeat scheduler
- [Office costs](../requirements/costs.md) - cost tracking and budget management
- [Office automations](../requirements/automations-settings.md) - recurring scheduled tasks
- [Office inbox](../requirements/inbox.md) - inbox, approvals, activity log
- [Office assistant](../requirements/assistant.md) - personal assistant, channels, agent memory
- [Office task sessions](tasks-01.md) - per-(task, agent) session lifecycle
