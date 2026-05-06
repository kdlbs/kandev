---
status: shipped
created: 2026-04-27
owner: cfl
---

# Office: Onboarding & First Workspace Setup

## Why

When a user opens `/office` for the first time, they see an empty dashboard with zero metrics and no guidance. There's no way to know what to do first -- create an agent? a project? a task? The user must discover the system on their own, clicking through sidebar items and empty pages.

A guided onboarding flow that creates a working workspace with a CEO agent gives users immediate momentum. The CEO agent can then hire workers, create tasks, and start orchestrating -- the user just needs to give it direction.

## What

### First-time detection

- When a user navigates to `/office`, the backend checks both DB and filesystem state.
- Three possible states:

| State | DB workspaces | FS workspaces | Action |
|-------|------|-----|--------|
| **Fresh install** | 0 | 0 | Redirect to `/office/setup` → onboarding wizard |
| **Shared config** | 0 | ≥1 | Redirect to `/office/setup` → import prompt |
| **Normal** | ≥1 | any | Show dashboard |

- Detection is backend-driven (`GET /api/v1/office/onboarding-state`), not localStorage.
- No default workspace is auto-created on startup. The filesystem is only populated by explicit user action (onboarding wizard or DB→FS export).

### Adding additional workspaces

- The workspace rail has an "Add workspace" (+) button that navigates to `/office/setup?mode=new`.
- The setup page checks for the `mode=new` query parameter. When present, it skips the onboarding-complete redirect and shows the wizard directly (steps 1-4), pre-filling defaults for the new workspace.
- The FS import prompt is also shown when `mode=new` if there are unimported FS workspaces (workspaces on disk that don't exist in the DB yet).
- After creating the new workspace, the user is redirected to `/office` with the new workspace selected.

### Import from filesystem (`/office/setup`, shared config state)

When the onboarding state indicates `fsWorkspaces ≥ 1` but `completed == false`, the setup page shows an import prompt instead of the wizard:

- Headline: "Existing configuration found"
- Subtitle: "Found N workspace(s) on the filesystem. Import settings to get started?"
- Lists workspace names found on disk
- **[Import & Continue]** button: calls `POST /api/v1/office/onboarding/import-fs` which creates DB workspace rows and imports all config (agents, skills, projects, routines) using the existing `ApplyIncoming` sync infrastructure, then marks onboarding complete
- **[Start Fresh]** button: skips import, proceeds to the onboarding wizard (ignores FS workspaces)

This supports the use case where a user clones a shared config directory (e.g. from git) and starts kandev for the first time.

### Onboarding wizard (`/office/setup`)

A full-page wizard (not a modal) with 4 steps:

**Step 1: Welcome + Workspace**
- Headline: "Set up your Office workspace"
- Subtitle: "Office manages a team of AI agents that work on your tasks autonomously."
- Workspace name input (default: "Default Workspace")
- Task prefix input (default: "KAN") with explanation: "Tasks will be numbered KAN-1, KAN-2, etc."
- [Next]

**Step 2: Create CEO Agent**
- Headline: "Create your CEO agent"
- Subtitle: "The CEO manages other agents, delegates tasks, and monitors progress. It doesn't write code -- it orchestrates."
- Agent name input (default: "CEO")
- Agent profile selector (dropdown of existing kandev agent profiles -- Claude, Codex, etc.)
- Executor preference (Local / Docker / Sprites) with descriptions
- Budget input (default: $0 = unlimited) with explanation
- [Back] [Next]

**Step 3: First Task (optional)**
- Headline: "Give your CEO something to do"
- Subtitle: "The CEO will analyze this task, break it into subtasks, and assign them to worker agents."
- Task title input (placeholder: "Explore the codebase and create an engineering roadmap")
- Task description textarea (optional, pre-filled with a helpful default)
- [Back] [Skip] [Next]

**Step 4: Review & Launch**
- Summary card showing what will be created:
  - Workspace: [name] with prefix [prefix]
  - CEO agent: [name] using [profile]
  - Task: [title] (or "No initial task")
- [Back] [Create & Launch]

### What gets created on "Create & Launch"

1. **Office workspace**: `kandev.yml` on filesystem + DB row in `workspaces` table + system office workflow (7 steps)
2. **CEO agent instance**: with `role=ceo`, full permissions, linked to selected agent profile, bundled skills (kandev-protocol, memory)
3. **Agent runtime row**: `status=idle` in `office_agent_runtime`
4. **"Onboarding" project** (auto): default project for the initial task
5. **First task** (if provided): assigned to CEO, in the Onboarding project, status=todo
6. **Onboarding state**: marked as completed in DB

After creation, redirect to `/office` (dashboard). If a task was created, the scheduler will wake the CEO agent to start working on it.

### Onboarding state API

```
GET /api/v1/office/onboarding-state
  -> { completed: bool, workspaceId?: string, ceoAgentId?: string,
       fsWorkspaces: [{ name: string }] }

POST /api/v1/office/onboarding/complete
  body: { workspaceName, taskPrefix, agentName, agentProfileId, executorPreference, taskTitle?, taskDescription? }
  -> { workspaceId, agentId, taskId?, projectId }

POST /api/v1/office/onboarding/import-fs
  -> { workspaceIds: string[], importedCount: int }
```

The `complete` endpoint does all creation in a single transaction -- workspace, agent, project, task, onboarding state. When `taskTitle` is provided, it creates a task assigned to the CEO agent and enqueues a `task_assigned` wakeup so the scheduler picks it up. It can be called multiple times to create additional workspaces.

The `import-fs` endpoint creates DB workspace rows for each FS workspace found that doesn't already exist in the DB, runs the config sync import for each, and marks onboarding complete.

The `fsWorkspaces` field in the state response only includes workspaces on disk that are **not** already imported to the DB. This allows the setup page to show "2 unimported workspaces found" when adding a new workspace.

### Returning users

- Backend checks `onboarding_state` per user
- If completed and no `mode=new` param: skip wizard, show dashboard
- If not completed (but user navigates away): wizard shown again on next visit

### Shared workspaces

- Onboarding state is per-workspace, not per-user
- If one team member completes onboarding, others see the completed workspace
- Additional users joining an existing workspace skip onboarding

## Scenarios

- **GIVEN** a new user opening `/office` for the first time, **WHEN** no workspace exists on DB or FS, **THEN** they are redirected to `/office/setup` and see the 4-step wizard.

- **GIVEN** a user opening `/office` for the first time, **WHEN** no DB workspace exists but FS workspaces are found, **THEN** they are redirected to `/office/setup` and see the import prompt with workspace names listed.

- **GIVEN** a user on the import prompt, **WHEN** they click "Import & Continue", **THEN** all FS workspaces are imported to DB, onboarding is marked complete, and they are redirected to the dashboard.

- **GIVEN** a user on the import prompt, **WHEN** they click "Start Fresh", **THEN** the import is skipped and the 4-step wizard is shown.

- **GIVEN** a user on step 2 of the wizard, **WHEN** they select a Claude agent profile and click Next, **THEN** step 3 shows the optional first task form.

- **GIVEN** a user on step 4 who clicks "Create & Launch", **WHEN** all inputs are valid and a task title was provided, **THEN** the workspace, CEO agent, project, and task are created. A `task_assigned` wakeup is enqueued. The user is redirected to the dashboard which now shows 1 agent enabled and 1 task in progress.

- **GIVEN** a user who skipped the first task on step 3, **WHEN** they reach the dashboard, **THEN** the CEO agent exists but is idle (no tasks). The dashboard empty state says "Assign a task to your CEO to get started."

- **GIVEN** a returning user who already completed onboarding, **WHEN** they open `/office`, **THEN** they see the dashboard directly (no wizard).

- **GIVEN** a user with an existing workspace, **WHEN** they click the "Add workspace" button in the workspace rail, **THEN** they see the setup wizard for a new workspace (not the dashboard redirect).

- **GIVEN** a user with 1 DB workspace and 2 unimported FS workspaces, **WHEN** they click "Add workspace", **THEN** the setup page shows the import prompt listing the 2 unimported workspaces, with options to import them or start fresh.

- **GIVEN** a user who just created a second workspace via the wizard, **WHEN** the wizard completes, **THEN** they are redirected to `/office` with the new workspace selected in the rail.

## Out of scope

- Template selection (Developer Team, Marketing Team, etc.) -- future feature
- Agent instruction bundles beyond the bundled system skills
- Onboarding video/tutorial content
