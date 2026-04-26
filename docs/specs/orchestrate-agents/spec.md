---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Agent Instances, Hierarchy & Permissions

## Why

Kandev has agent profiles (configuration templates: model, CLI flags, mode) but no concept of a persistent, stateful agent entity. There is no way to say "this agent named Frontend-1 reports to the CEO, has a $10 monthly budget, and is assigned the code-review and test-writer skills." Without agent instances, there is no hierarchy, no delegation, no budget tracking per agent, and no autonomous coordination.

Orchestrate introduces agent instances -- long-lived entities that reference an agent profile for execution config but carry their own identity, role, permissions, and state. A CEO instance at the top of the hierarchy can create worker instances, assign tasks, and monitor their output.

## What

### Agent instances

- An agent instance is a persistent entity distinct from `AgentProfile`.
- `AgentProfile` remains unchanged -- it describes how to launch a specific agent CLI (model, flags, mode).
- An agent instance references a profile via `agent_profile_id` and adds:
  - **Name**: human-readable label (e.g. "CEO", "Frontend Worker", "QA Bot").
  - **Role**: `ceo`, `worker`, or `specialist`. Determines default permissions and UI treatment.
  - **Status**: `idle` (no active work), `working` (session running), `paused` (manually or budget-stopped), `stopped` (deactivated).
  - **Permissions**: JSON object controlling what the instance can do.
  - **Budget**: remaining spend allowance (see [orchestrate-costs](../orchestrate-costs/spec.md)).
  - **Skills**: list of assigned skill IDs (see [orchestrate-skills](../orchestrate-skills/spec.md)).
  - **Icon**: avatar/icon for UI display.
  - **Executor preference**: optional executor override for this agent (see executor resolution below).
- Multiple instances can share the same profile (e.g. three "Claude Sonnet" workers with different skills and budgets).

### Hierarchy

- Every agent instance has an optional `reports_to` field pointing to another instance.
- The CEO instance has `reports_to = null` (root of the tree).
- There is at most one CEO per workspace.
- The hierarchy is advisory for humans and load-bearing for the CEO's delegation logic -- the CEO's system prompt includes the org tree so it knows who to assign work to.
- Worker agents can themselves have sub-agents (e.g. a "Backend Lead" with "Go Worker" and "Test Worker" under it), enabling multi-level delegation.

### CEO agent

- The CEO is an agent instance with `role=ceo` and elevated permissions.
- The CEO does not write code. It reads task descriptions, decomposes them into subtasks, assigns them to workers, and monitors completion.
- The CEO's system prompt includes:
  - Its delegation rules (which roles handle which types of work).
  - The current org tree (all agent instances and their status).
  - The workspace's project structure.
  - The current task backlog (unassigned and in-progress tasks).
- The CEO creates worker agents when no suitable worker exists for a task type (via the hire flow -- see permissions below).
- The CEO is configured with a high-capability reasoning model (user-configurable via profile selection).

### Permissions

- Permissions are a JSON object on the agent instance.
- Permission keys:
  - `can_create_tasks`: create new tasks and subtasks.
  - `can_assign_tasks`: assign tasks to other agent instances.
  - `can_create_agents`: propose new agent instances (subject to approval).
  - `can_approve`: resolve approvals for sub-agents.
  - `can_manage_own_skills`: create or edit skills in the registry for itself (subject to approval if `require_approval_for_skill_changes=true`). See [orchestrate-assistant](../orchestrate-assistant/spec.md).
  - `max_subtask_depth`: maximum nesting depth for subtask creation (prevents runaway delegation).
- Default permissions by role:
  - `ceo`: all permissions enabled, `max_subtask_depth=3`.
  - `assistant`: `can_create_tasks=true`, `can_assign_tasks=true`, `can_manage_own_skills=true`, all others false.
  - `worker`: `can_create_tasks=true` (for subtask decomposition only), all others false.
  - `specialist`: same as worker.
- Users can override defaults per instance.

### Agent hire flow

- When the CEO (or any instance with `can_create_agents`) creates a new agent instance, it submits a hire request.
- If the workspace has `require_approval_for_new_agents=true` (default), the hire creates a pending approval in the inbox.
- The user reviews the proposed config (name, role, profile, skills, budget) and approves or rejects.
- On approval, the instance status moves from `pending_approval` to `idle` and becomes available for task assignment.
- On rejection, the instance is deleted. The requesting agent receives a wakeup with the rejection reason.

### Concurrency

- Each agent instance has a `max_concurrent_sessions` setting (default 1).
- When set to 1, the agent processes tasks sequentially -- wakeups stay queued until the agent finishes its current session.
- When set to N > 1, the agent can run up to N sessions in parallel on different tasks. This is useful for workers handling independent, lightweight tasks (e.g. code reviews, test runs).
- The scheduler's claim query skips agents at capacity -- wakeups remain in `queued` status indefinitely until the agent has a free slot. No re-queuing, no retry limits, no expiry. This handles slow agents with large backlogs gracefully.
- Users configure concurrency per agent instance via the agent settings UI.

### Executor resolution

When the scheduler launches a session for an agent instance, the executor is resolved automatically. No agent needs to choose an executor -- it's derived from configuration. Resolution chain (first non-null wins):

1. **Task-level override** (`execution_policy.executor_config` on the task) -- rare, for special cases like "this deploy task must run on the staging server."
2. **Agent instance executor preference** (`executor_preference` on the agent instance) -- e.g. "QA Bot always runs in sprites for isolated sandbox."
3. **Project executor config** (`executor_config` on the project, see [orchestrate-projects](../orchestrate-projects/spec.md)) -- e.g. "Backend project uses local_docker with node:20 image."
4. **Workspace default executor** -- the fallback from workspace settings.

The executor preference on an agent instance is a JSON field with the same shape as project executor config: `{ type, image, resource_limits, environment_id }`. It's set by the user during agent creation or by the CEO in the hire request.

Worktrees are automatic: when a task targets a repository, the system creates a git worktree (branch) for that task's session using the existing `worktree.Manager`. The worktree strategy (per-task or shared) comes from the project config.

The CEO/CTO never explicitly picks an executor -- they assign tasks to projects and agents. The executor is resolved from the chain above. The CEO's skill teaches it: "assign backend tasks to the Backend project; the project's executor config handles the rest."

### Agent instance lifecycle

- **Creating**: user or CEO creates an instance. If approval required, starts in `pending_approval`.
- **Idle**: instance exists but has no active work. Available for task assignment.
- **Working**: instance has one or more active sessions running. The number of concurrent sessions is controlled by `max_concurrent_sessions`.
- **Paused**: manually paused by user, or auto-paused by budget enforcement. No new wakeups processed. Active sessions complete their current turn but receive no further prompts.
- **Stopped**: deactivated. No longer appears in the CEO's org tree. Can be reactivated.

### UI at `/orchestrate/agents`

- Agent list showing cards for each instance: icon, name, role, status indicator, current task (if working), budget gauge, skill badges.
- "+" button to create a new agent instance (select profile, set name/role/skills/budget).
- Sidebar "Agents" section shows a compact list of all instances with status dots and channel indicators (Telegram, Slack icons if channels are configured).
- Click an agent card to open the detail page at `/orchestrate/agents/[id]` with tabs:
  - **Overview**: name, role, status, org position, current task, budget gauge.
  - **Skills**: assigned skills with enable/disable toggles. Agent-created skills marked with an indicator.
  - **Runs**: session/run history with status, duration, cost, linked task.
  - **Memory**: browsable memory entries grouped by layer (operating, knowledge, session). View, delete, clear all, export, search. See [orchestrate-assistant](../orchestrate-assistant/spec.md).
  - **Channels**: configured messaging channels with status, platform icon, setup/edit. See [orchestrate-assistant](../orchestrate-assistant/spec.md).

## Scenarios

- **GIVEN** a workspace with no agent instances, **WHEN** the user creates a CEO instance (selecting a profile, setting role=ceo), **THEN** the instance appears in the agents list with status `idle` and the sidebar shows it under "Agents".

- **GIVEN** a running CEO instance, **WHEN** the CEO determines a task requires a frontend specialist and no suitable worker exists, **THEN** the CEO submits a hire request for a new worker instance with appropriate skills. The request appears in the user's inbox as a pending approval.

- **GIVEN** a pending hire approval, **WHEN** the user approves it, **THEN** the new agent instance activates (status=idle), appears in the org tree under the CEO, and the CEO receives a wakeup notification.

- **GIVEN** a worker instance with `can_create_tasks=true`, **WHEN** the worker creates a subtask exceeding `max_subtask_depth`, **THEN** the creation is rejected and the worker is informed.

- **GIVEN** a worker instance with status `working`, **WHEN** the user clicks "Pause" on the agent card, **THEN** the current session completes its turn, the instance moves to `paused`, and no new wakeups are processed for it.

## Out of scope

- Agent-to-agent real-time communication (agents communicate via tasks and comments only).
- Custom agent binaries (instances use the existing agent registry: Claude, Codex, Copilot, etc.).
- Automatic scaling of agent instances based on workload.
- Agent instance migration between workspaces.
