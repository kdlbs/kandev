---
status: shipped
created: 2026-05-02
owner: cfl
---

# Workspace Deletion

## Why

Users who create a workspace by mistake, finish an experiment, or consolidate workspaces have no way to remove one. The workspace and all its data (agents, tasks, skills, routines, cost history, filesystem config) persist forever, cluttering the UI and wasting disk space.

## What

- The workspace settings page shows a "Delete workspace" button in a danger zone section.
- Clicking it opens a confirmation dialog that shows a summary of what will be deleted: number of tasks, agents, and skills. It also displays the full filesystem path that will be removed (e.g. `~/.kandev/workspaces/my-workspace/`). The user must type the workspace name to proceed.
- Deletion stops all running agents in the workspace before removing data.
- All workspace-owned data is removed: agents (+ memory, instructions, runtime), skills, projects, routines (+ triggers, runs), approvals, channels, labels, cost events, budget policies, wakeup queue entries, activity logs, governance settings, and the onboarding record.
- All tasks in the workspace are deleted, including their sessions, worktrees, blockers, and comments.
- The filesystem config directory (`~/.kandev/workspaces/<slug>/`) and any quick-chat workspace directories created for the workspace's sessions are removed.
- After deletion the user is redirected to `/office/setup` if no other workspaces remain, or to `/office` with the next available workspace selected.
- The operation is irreversible. There is no soft-delete or undo.

## Scenarios

- **GIVEN** a workspace with 3 agents, 10 tasks, and 2 running sessions, **WHEN** the user deletes the workspace, **THEN** running sessions are stopped, all tasks and agents are removed from the DB, the filesystem config directory is deleted, and the user lands on the dashboard of another workspace (or the setup page if none remain).

- **GIVEN** the user is on the workspace settings page, **WHEN** they click "Delete workspace", **THEN** a confirmation dialog appears showing "This will delete 3 agents, 10 tasks, 5 skills" and the filesystem path, requiring them to type the workspace name before the delete button becomes active.

- **GIVEN** a workspace with tasks that have worktrees under `~/.kandev/tasks/`, **WHEN** the workspace is deleted, **THEN** the worktree directories for those tasks are cleaned up.

- **GIVEN** a user whose saved workspace ID points to a deleted workspace, **WHEN** they navigate to `/office`, **THEN** the system falls back to the first available workspace, persists the corrected ID in user settings, and shows the correct dashboard data.

## Out of scope

- Workspace archiving or soft-delete — deletion is permanent.
- Exporting workspace data before deletion.
- Deleting individual entities within a workspace (agents, projects) — those have their own flows.
- Multi-workspace bulk deletion.
