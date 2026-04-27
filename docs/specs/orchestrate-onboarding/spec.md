---
status: draft
created: 2026-04-27
owner: cfl
---

# Orchestrate: Onboarding & First Workspace Setup

## Why

When a user opens `/orchestrate` for the first time, they see an empty dashboard with zero metrics and no guidance. There's no way to know what to do first -- create an agent? a project? a task? The user must discover the system on their own, clicking through sidebar items and empty pages.

A guided onboarding flow that creates a working workspace with a CEO agent gives users immediate momentum. The CEO agent can then hire workers, create tasks, and start orchestrating -- the user just needs to give it direction.

## What

### First-time detection

- When a user navigates to `/orchestrate`, the backend checks if ANY orchestrate workspace has been set up for this user.
- If no workspace exists: redirect to `/orchestrate/setup` (the onboarding wizard).
- If a workspace exists: show the normal dashboard.
- Detection is backend-driven (`GET /api/v1/orchestrate/onboarding-state`), not localStorage.

### Onboarding wizard (`/orchestrate/setup`)

A full-page wizard (not a modal) with 4 steps:

**Step 1: Welcome + Workspace**
- Headline: "Set up your Orchestrate workspace"
- Subtitle: "Orchestrate manages a team of AI agents that work on your tasks autonomously."
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

1. **Orchestrate workspace**: `kandev.yml` on filesystem + DB row in `workspaces` table + system orchestrate workflow (7 steps)
2. **CEO agent instance**: with `role=ceo`, full permissions, linked to selected agent profile, bundled skills (kandev-protocol, memory)
3. **Agent runtime row**: `status=idle` in `orchestrate_agent_runtime`
4. **"Onboarding" project** (auto): default project for the initial task
5. **First task** (if provided): assigned to CEO, in the Onboarding project, status=todo
6. **Onboarding state**: marked as completed in DB

After creation, redirect to `/orchestrate` (dashboard). If a task was created, the scheduler will wake the CEO agent to start working on it.

### Onboarding state API

```
GET /api/v1/orchestrate/onboarding-state
  -> { completed: bool, workspaceId?: string, ceoAgentId?: string }

POST /api/v1/orchestrate/onboarding/complete
  body: { workspaceName, taskPrefix, agentName, agentProfileId, executorPreference, taskTitle?, taskDescription? }
  -> { workspaceId, agentId, taskId?, projectId }
```

The `complete` endpoint does all creation in a single transaction -- workspace, agent, project, task, onboarding state.

### Returning users

- Backend checks `onboarding_state` per user
- If completed: skip wizard, show dashboard
- If not completed (but user navigates away): wizard shown again on next visit
- "Re-run setup" option in settings for users who want to reconfigure

### Shared workspaces

- Onboarding state is per-workspace, not per-user
- If one team member completes onboarding, others see the completed workspace
- Additional users joining an existing workspace skip onboarding

## Scenarios

- **GIVEN** a new user opening `/orchestrate` for the first time, **WHEN** no workspace exists, **THEN** they are redirected to `/orchestrate/setup` and see the 4-step wizard.

- **GIVEN** a user on step 2 of the wizard, **WHEN** they select a Claude agent profile and click Next, **THEN** step 3 shows the optional first task form.

- **GIVEN** a user on step 4 who clicks "Create & Launch", **WHEN** all inputs are valid, **THEN** the workspace, CEO agent, project, and task are created. The user is redirected to the dashboard which now shows 1 agent enabled and 1 task in progress.

- **GIVEN** a user who skipped the first task on step 3, **WHEN** they reach the dashboard, **THEN** the CEO agent exists but is idle (no tasks). The dashboard empty state says "Assign a task to your CEO to get started."

- **GIVEN** a returning user who already completed onboarding, **WHEN** they open `/orchestrate`, **THEN** they see the dashboard directly (no wizard).

## Out of scope

- Multi-workspace creation in the wizard (one workspace per onboarding run)
- Template selection (Developer Team, Marketing Team, etc.) -- future feature
- Agent instruction bundles beyond the bundled system skills
- Onboarding video/tutorial content
