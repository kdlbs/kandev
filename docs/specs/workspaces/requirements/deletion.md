---
status: active
system: workspaces
created: 2026-05-02
owners:
  - cfl
---
# Workspace Deletion Requirements

## Overview

Users who create a workspace by mistake, finish an experiment, or consolidate workspaces have no way to remove one. The workspace and all its data (agents, tasks, skills, routines, cost history, filesystem config) persist forever, cluttering the UI and wasting disk space.

## Requirements

### REQ-WORKSPACES-DELETION-001: Workspace Deletion

**Intent:** Users who create a workspace by mistake, finish an experiment, or consolidate workspaces have no way to remove one. The workspace and all its data (agents, tasks, skills, routines, cost history, filesystem config) persist forever, cluttering the UI and wasting disk space.

#### Acceptance criteria

- **AC-WORKSPACES-DELETION-001.1:** The workspace settings page shows a "Delete workspace" button in a danger zone section.
- **AC-WORKSPACES-DELETION-001.2:** Clicking it opens a confirmation dialog that shows a summary of what will be deleted: number of tasks, agents, and skills. It also displays the full filesystem path that will be removed (e.g. `~/.kandev/workspaces/my-workspace/`). The user must type the workspace name to proceed.
- **AC-WORKSPACES-DELETION-001.3:** Deletion stops all running agents in the workspace before removing data.
- **AC-WORKSPACES-DELETION-001.4:** All workspace-owned data is removed: agents (+ memory, instructions, runtime, runs), skills, projects, routines (+ triggers, runs), run events, run route attempts, run skill materializations, wakeup requests, continuation summaries, approvals, channels, labels, cost events, budget policies, routing settings, provider health, activity logs, governance settings, workspace groups, tree holds, workspace settings, and the onboarding record.
- **AC-WORKSPACES-DELETION-001.5:** All tasks in the workspace are deleted, including their sessions, worktrees, blockers, and comments.
- **AC-WORKSPACES-DELETION-001.6:** The filesystem config directory (`~/.kandev/workspaces/<slug>/`) and any quick-chat workspace directories created for the workspace's sessions are removed.
- **AC-WORKSPACES-DELETION-001.7:** After deletion the user is redirected to `/office/setup?mode=new` if no other workspaces remain. If workspaces remain, another Office workspace is preferred; otherwise the first remaining workspace is selected and the user is redirected to that workspace's native home route (`/office?workspaceId=<id>` for Office, `/?workspaceId=<id>` for Kanban).
- **AC-WORKSPACES-DELETION-001.8:** The operation is irreversible. There is no soft-delete or undo.

### REQ-WORKSPACES-DELETION-002: Preserve current catalogue during Settings deletion

**Intent:** Accepting a deletion from the shared workspace Settings page shall
remove the target without discarding independent workspace choices or changes
already visible while deletion was pending. Workspaces owns this boundary
because it owns workspace lifecycle and the catalogue consumed by navigation.

This requirement applies only to deletion from the shared Settings workspace
editor. Requirement 001 retains its existing complete-deletion and Office
post-delete destination contract. The shared Settings editor retains its
existing return to `/settings/workspaces`; this adds no routing repair.

#### Acceptance criteria

- **AC-WORKSPACES-DELETION-002.1:** When independent workspaces are added,
  updated, or removed while Settings deletion is pending, successful
  acknowledgement shall remove only the target from the current catalogue.
  Every surviving workspace shall retain its current descriptor and relative
  order. Added choices shall remain available, removed choices shall stay
  absent, and earlier names or metadata shall not replace current values.
- **AC-WORKSPACES-DELETION-002.2:** The actual workspace picker shall expose
  surviving current choices and names after accepted deletion and the existing
  return to `/settings/workspaces`. The target shall be absent. Desktop and
  phone users shall receive the same catalogue outcome through their existing
  shared controls and state.
- **AC-WORKSPACES-DELETION-002.3:** Accepted deletion shall preserve the active
  identity and selection revision current at acknowledgement under the existing
  catalogue publication policy. A non-null identity shall remain unchanged;
  with a null identity and survivors, the first survivor shall become active
  without a new selection revision. Independent selection or deletion-event
  fallback already applied while the request was pending shall retain its
  existing effect. This acknowledgement introduces no explicit selection or
  cookie write beyond the existing catalogue policy.
- **AC-WORKSPACES-DELETION-002.4:** Ordinary accepted deletion shall retain
  target removal and the existing Settings return. A rejected deletion shall
  publish no removal from that request and shall retain independently received
  catalogue changes, the current selection, confirmation dialog and entered
  name, existing failure feedback, and the current route.
- **AC-WORKSPACES-DELETION-002.5:** Settings deletion shall retain the exact
  workspace-name confirmation guard, existing owner/manage eligibility, and
  generic-versus-Office request selection. A mismatched confirmation shall send
  no deletion request. Backend cascade and Office deletion behavior remain
  governed by their existing contracts.

## Migrated source detail

## Why

Users who create a workspace by mistake, finish an experiment, or consolidate workspaces have no way to remove one. The workspace and all its data (agents, tasks, skills, routines, cost history, filesystem config) persist forever, cluttering the UI and wasting disk space.

## What

- The workspace settings page shows a "Delete workspace" button in a danger zone section.
- Clicking it opens a confirmation dialog that shows a summary of what will be deleted: number of tasks, agents, and skills. It also displays the full filesystem path that will be removed (e.g. `~/.kandev/workspaces/my-workspace/`). The user must type the workspace name to proceed.
- Deletion stops all running agents in the workspace before removing data.
- All workspace-owned data is removed: agents (+ memory, instructions, runtime, runs), skills, projects, routines (+ triggers, runs), run events, run route attempts, run skill materializations, wakeup requests, continuation summaries, approvals, channels, labels, cost events, budget policies, routing settings, provider health, activity logs, governance settings, workspace groups, tree holds, workspace settings, and the onboarding record.
- All tasks in the workspace are deleted, including their sessions, worktrees, blockers, and comments.
- The filesystem config directory (`~/.kandev/workspaces/<slug>/`) and any quick-chat workspace directories created for the workspace's sessions are removed.
- After deletion the user is redirected to `/office/setup?mode=new` if no other workspaces remain. If workspaces remain, another Office workspace is preferred; otherwise the first remaining workspace is selected and the user is redirected to that workspace's native home route (`/office?workspaceId=<id>` for Office, `/?workspaceId=<id>` for Kanban).
- The operation is irreversible. There is no soft-delete or undo.

## Scenarios

- **GIVEN** a workspace with 3 agents, 10 tasks, and 2 running sessions, **WHEN** the user deletes the workspace, **THEN** running sessions are stopped, all tasks and agents are removed from the DB, the filesystem config directory is deleted, and the user lands on another workspace's native home route (or the new Office workspace setup page if none remain).

- **GIVEN** the user is on the workspace settings page, **WHEN** they click "Delete workspace", **THEN** a confirmation dialog appears showing "This will delete 3 agents, 10 tasks, 5 skills" and the filesystem path, requiring them to type the workspace name before the delete button becomes active.

- **GIVEN** a workspace with tasks that have worktrees under `~/.kandev/tasks/`, **WHEN** the workspace is deleted, **THEN** the worktree directories for those tasks are cleaned up.

- **GIVEN** a user whose saved workspace ID points to a deleted workspace, **WHEN** they navigate to `/office`, **THEN** the system falls back to the first available workspace, persists the corrected ID in user settings, and shows the correct dashboard data.

## Out of scope

- Workspace archiving or soft-delete — deletion is permanent.
- Exporting workspace data before deletion.
- Deleting individual entities within a workspace (agents, projects) — those have their own flows.
- Multi-workspace bulk deletion.

For requirement 002, changes to backend or Office deletion, selection or route
policy, stale settings drafts, unrelated catalogue writers, global catalogue
arbitration, lifecycle cleanup, rendered composition, or product copy are also
excluded.

## References

- [Deletion system design](../system-design/deletion.md).
- [Settings save and creation publication](workspace-settings-updates.md).
- [Per-tab Settings context](per-tab-settings-context.md).
- [Settings deletion catalogue plan](../../../plans/workspace-delete-catalogue-preservation/plan.md).
