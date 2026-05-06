---
status: shipped
created: 2026-04-28
owner: cfl
---

# Agent Creation Governance

## Why

Any agent with `can_create_agents: true` (CEO by default) can call `POST /workspaces/:wsId/agents` and create unlimited subordinate agents that immediately become `idle` and start receiving wakeups. The approval infrastructure already exists — `ApprovalTypeHireAgent` in `models.go`, `AgentStatusPendingApproval` in the status enum, `pending_approval → idle` in `allowedTransitions` — but nothing wires them into the creation path.

Without governance, a misbehaving or compromised CEO agent can silently expand the agent roster, consume budget, and trigger downstream wakeups before the human operator notices. The infrastructure is present; it just isn't connected.

## What

### Workspace setting: `require_approval_for_new_agents`

A boolean workspace setting stored in `office_workspace_settings` (new table, keyed by workspace ID). Default: `true`.

The setting is readable and writable via `GET/PATCH /workspaces/:wsId/settings`. It appears in the workspace settings UI under a "Governance" section.

### Creation path changes

The `createAgent` handler currently sets `agent.Status = models.AgentStatusIdle`. After this change:

1. The handler reads `require_approval_for_new_agents` for the workspace.
2. If the caller is an authenticated agent (JWT present) **and** the setting is `true`:
   - Agent is persisted with `status = pending_approval`.
   - An `ApprovalTypeHireAgent` approval record is created via `ApprovalService.CreateApprovalWithActivity`. The payload includes: agent name, role, requested permissions JSON, creator agent ID, and the optional `reason` field from the request body.
   - Response is `201 Created` with the agent in `pending_approval` status.
3. If the caller is a human (no JWT) **or** the setting is `false`:
   - Agent is persisted with `status = idle` — unchanged from current behavior.

A `pending_approval` agent cannot receive wakeups. The scheduler already skips non-`idle` agents; no scheduler changes are needed.

### Approval resolution

`ApprovalService.applyApprovalSideEffects` currently calls `queueApprovalWakeup` for `ApprovalTypeHireAgent`. This changes:

- **Approved**: call `agentWriter.UpdateAgentStatusFields(ctx, agentInstanceID, "idle", "")` to activate the agent, then queue the approval wakeup so the creating agent is notified.
- **Rejected**: call `agentWriter.UpdateAgentStatusFields(ctx, agentInstanceID, "stopped", "rejected by human")` to prevent activation, then queue the wakeup.

`ApprovalService` needs an `AgentWriter` dependency (the same `shared.AgentWriter` interface already implemented by `AgentService`).

The `Approval.Payload` must include `agent_instance_id` so the service can look up which agent to activate or stop.

### Request body addition

`CreateAgentRequest` gains an optional `reason string` field. When the caller is an agent, this is included in the approval payload so the human reviewer sees why the agent is being hired.

### Workspace settings model and API

New table `office_workspace_settings`:

```sql
CREATE TABLE IF NOT EXISTS office_workspace_settings (
    workspace_id TEXT PRIMARY KEY,
    require_approval_for_new_agents INTEGER NOT NULL DEFAULT 1,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

New repository methods: `GetWorkspaceSettings`, `UpsertWorkspaceSettings`.

New handler (mounted under existing config or a new settings group):
- `GET /workspaces/:wsId/settings` — returns current settings, with defaults if no row exists.
- `PATCH /workspaces/:wsId/settings` — updates one or more fields.

## UX

### Inbox

Pending agent hire approvals appear in the inbox as items with type `hire_agent`. Each item shows:

- Title: "New agent request: {agent name}"
- Body: role, requested permissions (human-readable), creator agent name, reason (if provided)
- Actions: **Approve** / **Reject** buttons (call `POST /approvals/:id/decide`)

### Agent list

Agents with `status = pending_approval` appear in the agent list with:

- A "Pending approval" badge (amber, not green)
- Dimmed appearance
- No "Assign task" button

### Workspace settings

Under **Settings > Governance** (or equivalent section in the office settings page):

- Toggle: "Require approval for new agents created by agents" (default: on)
- Help text: "When enabled, agents created by other agents must be approved before they can receive tasks. Agents created directly from the UI are always created immediately."

## Scenarios

- **GIVEN** `require_approval_for_new_agents = true`, **WHEN** the CEO agent calls `POST /workspaces/:wsId/agents`, **THEN** the new agent is persisted with `status = pending_approval`, an `ApprovalTypeHireAgent` record is created, and the API returns the agent with `pending_approval` status.

- **GIVEN** a `pending_approval` agent with an open hire approval, **WHEN** the human approves via `POST /approvals/:id/decide` with `status = approved`, **THEN** the agent's status changes to `idle` and the CEO receives a `approval_resolved` wakeup.

- **GIVEN** a `pending_approval` agent, **WHEN** the human rejects, **THEN** the agent's status changes to `stopped` and the CEO receives a `approval_resolved` wakeup with `status = rejected`.

- **GIVEN** a human user creates an agent via the UI (no JWT), **WHEN** `POST /workspaces/:wsId/agents` is called, **THEN** the agent is created as `idle` regardless of the governance setting.

- **GIVEN** `require_approval_for_new_agents = false`, **WHEN** the CEO creates an agent, **THEN** the agent is created as `idle` immediately with no approval record.

## Out of scope

- Board-level revision requests (`revision_requested` → `pending` state); kandev uses a simpler two-outcome approve/reject model for now.
- Limiting which human users can approve (all authenticated users can decide approvals today; role-based approval gating is a separate feature).
- CEO agents creating other CEO agents (already blocked by `ErrAgentCEOAlreadyExists`; governance adds no new constraint here).
- Retroactively gate-checking agents already at `idle` status.
