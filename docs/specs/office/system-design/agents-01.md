---
status: draft
system: office
requirements:
  - REQ-OFFICE-AGENTS-001
created: 2026-04-25
owners:
  - cfl
---
# Office: Agents System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AGENTS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AGENTS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Kandev has execution profiles (configuration templates for a concrete CLI, account, model, flags, environment, and MCP setup) but also needs persistent, stateful Office agents. Without a stable Office identity, switching a provider also risks switching or copying the agent's role, instructions, skills, permissions, budget, and history.

Office therefore treats a workspace-scoped rich `agent_profiles` row as a stable Office identity and selects a separate execution agent profile. The selected profile can be concrete or dynamic. A dynamic profile resolves a concrete profile for each launch through the shared routing mechanism used by Kanban. These profile kinds remain in the unified table established by ADR 0005, but they have different responsibilities. Office agents run inside a narrow capability-scoped runtime, call kandev through a structured CLI rather than raw curl, and expose per-agent dashboards with run history, costs, and per-run detail pages.

## What

### Agent instances

- An Office agent is a persistent `agent_profiles` row scoped by `workspace_id`; its row ID is the logical `agent_profile_id` referenced by assignments, instructions, skills, budgets, permissions, and Office history.
- An Office agent selects an execution agent profile. A concrete selection launches directly. A dynamic selection resolves an `execution_profile_id` from the dynamic profile; the resolved concrete profile owns the CLI runtime configuration.
- The Office identity owns:
  - **Name**: human-readable label ("CEO", "Frontend Worker", "QA Bot").
  - **Role**: `ceo`, `worker`, `specialist`, `assistant`, or `reviewer`. Determines default permissions and UI treatment.
  - **Status**: `idle`, `working`, `paused`, `stopped`, plus transitional `pending_approval`.
  - **Permissions**: JSON object controlling what the instance can do.
  - **Budget**: remaining spend allowance (see [costs](../requirements/costs.md)).
  - **Skills**: list of assigned skill IDs.
  - **Instructions**: per-agent `AGENTS.md`, `HEARTBEAT.md`, `SOUL.md`, `TOOLS.md`.
  - **Icon**: avatar for UI display.
  - **Executor preference**: optional executor override for this agent.
  - **Channels**: optional external messaging channels (Telegram, Slack).
- Multiple Office agents can use the same execution profile without sharing Office instructions, skills, permissions, budgets, or history.
- Changing an agent's selected execution profile changes future resolution without changing the Office identity.
- Provider order, mappings, fallback, and shared health belong to [Dynamic Agent Routing](../../agents/requirements/dynamic-agent-routing.md), not to Office settings.

### Hierarchy

- Every agent instance has an optional `reports_to` field pointing to another instance.
- The CEO instance has `reports_to = null` (root of the tree). At most one CEO per workspace.
- The hierarchy is advisory for humans and load-bearing for the CEO's delegation logic: the CEO's system prompt includes the org tree so it knows who to assign work to.
- Worker agents can themselves have sub-agents (e.g. a "Backend Lead" with "Go Worker" and "Test Worker" under it), enabling multi-level delegation.

### CEO agent

- The CEO is an agent instance with `role=ceo` and elevated permissions.
- The CEO does not write code. It reads task descriptions and evidence, handles small coordination or status concerns directly, decomposes implementation work into subtasks, assigns them to workers, and monitors completion.
- The CEO's system prompt includes its delegation rules, the current org tree, the workspace's project structure, and the current task backlog (unassigned and in-progress).
- The CEO creates worker agents when no suitable worker exists for a task type, via the hire flow.
- The CEO is configured with a high-capability reasoning model, user-selectable via the profile.

### Concurrency

- Each agent instance has `max_concurrent_sessions` (default 1).
- At 1, the agent processes tasks sequentially: wakeups queue until the current session finishes.
- At N > 1, the agent can run up to N sessions in parallel on different tasks. Useful for lightweight independent work (code reviews, test runs).
- The scheduler skips agents at capacity. Wakeups remain in `queued` indefinitely until a slot frees up. No re-queuing, no retry limits, no expiry.

### Executor resolution

The executor is resolved automatically when the scheduler launches a session for an agent. No agent picks an executor. Resolution chain (first non-null wins):

1. **Task-level override** (`execution_policy.executor_config` on the task).
2. **Agent instance executor preference** (`executor_preference`).
3. **Project executor config** (see [projects](../requirements/overview.md)).
4. **Workspace default executor**.

`executor_preference` shape mirrors project executor config: `{ type, image, resource_limits, environment_id }`.

When an agent creates another agent and omits `executor_preference`, the new agent inherits the creator's executor preference before defaults are applied. This keeps delegated child agents launchable in the same executor context unless the creator explicitly overrides the preference.

Worktrees are automatic: when a task targets a repository, the system creates a git worktree (branch) using the existing `worktree.Manager`. Strategy (per-task or shared) comes from the project config.

## Data model

### Agent config (filesystem)

`agents/<name>.yml` in the workspace config tree. Source of truth for agent configuration: editable, versionable via git.

```yaml
# agents/ceo.yml
id: "abc-123"
name: CEO
role: ceo
agent_profile_id: "prof_abc123"
execution_agent_profile_id: "dyn_frontier"
reports_to: ""
icon: crown
permissions: '{"can_create_tasks":true,"can_assign_tasks":true,"can_create_agents":true}'
budget_monthly_cents: 5000
max_concurrent_sessions: 1
desired_skills: '["memory","delegation-playbook"]'
executor_preference: ""
```

`agent_profile_id` identifies the Office agent itself.
`execution_agent_profile_id` references the concrete or dynamic profile used to
run it.

The workspace-scoped Office identity row persists this selection in
`agent_profiles.execution_agent_profile_id`, an empty-by-default soft reference
to another global or compatible same-workspace profile. It is not a cascading
foreign key because confirmed profile deletion retains stale bindings for
explicit repair. A missing or disabled selection makes launches fail closed.

### Agent runtime (DB)

`office_agent_runtime` row per agent instance.

```
office_agent_runtime
  agent_id                  string   PK
  status                    enum     idle | working | paused | stopped | pending_approval
  pause_reason              string   nullable
  last_wakeup_finished_at   timestamp nullable
```

Runtime state must survive restarts (a budget-paused agent stays paused). Not user-editable, not exported. On startup, the reconciliation service merges filesystem config with this DB state: missing runtime rows are created with `status=idle`; orphaned rows (no YAML) are deleted.

### Office identities and execution agent profiles

Office identities and execution profiles stay in the existing `agent_profiles` DB table, as required by ADR 0005, but launch resolution does not treat them as the same logical object:

- A workspace-scoped rich row is the Office identity. Its `agent_profile_id` remains stable across provider changes and is used for hierarchy, instructions, skills, Office permissions, budgets, costs, and task/run ownership.
- The Office identity references one execution agent profile. A concrete profile launches directly. A dynamic profile owns reusable routing policy and resolves a concrete profile per launch.
- The resolved concrete profile is recorded as `execution_profile_id`. It owns the registered CLI/provider, credentials and environment, model, mode, ACP config options, CLI flags, CLI permission behavior, passthrough mode, and MCP configuration.
- Global shallow concrete and dynamic profiles are the normal execution-agent choices. Existing same-workspace rows may remain selectable for upgrade compatibility when valid.
- An execution agent profile from another workspace cannot be selected.
- Profile callers pass one selected profile ID to the shared execution resolver.
  For a concrete selection, the resolved `execution_profile_id` equals that ID.
  For a dynamic selection, the logical selected profile remains dynamic while
  the conductor records the concrete candidate as `execution_profile_id`.
  Callers do not resolve candidates or branch on profile kind.
- The profile `kind` describes the execution family. The Office identity role
  is separate. An Office identity can bind to a concrete or dynamic execution
  profile, but a rich Office identity cannot appear in a dynamic candidate list.

The CLI-shaped columns on rich Office rows remain in the schema for compatibility with ADR 0005 and existing data, but they are not authoritative for Office launches. New Office configuration selects a concrete or dynamic execution agent profile instead of copying CLI runtime fields or routing rules into the identity row.

### Instructions

Stored per agent in DB (source of truth) with these well-known files:

- `AGENTS.md` (required): persona, delegation rules, operating procedure. Injected into prompt.
- `HEARTBEAT.md` (optional): per-wakeup checklist, on disk.
- `SOUL.md` (optional): voice/tone guidelines, on disk.
- `TOOLS.md` (optional): living doc the agent updates with discovered tools.
- Plus user-added custom instruction files.

Before each session, instructions are written to `~/.kandev/runtime/<workspace-slug>/instructions/<agentId>/` and the path is injected into the prompt.

### Skills

A skill is a directory containing `SKILL.md` (required: the markdown instructions the agent reads) plus optional scripts and reference files. The structure matches Claude Code's native skill discovery and other agent CLIs. Materialized `SKILL.md` files must be valid Codex/Claude-style skill files: when stored content lacks YAML frontmatter, the runtime prepends generated `name` and `description` frontmatter from the skill slug before writing or uploading the file. Supporting files recorded in `file_inventory` are written beside `SKILL.md` so bundled skills can use progressive disclosure through `references/`. Decision: ADR-0030.

`skill` DB row (workspace-scoped): `id` PK, `name`, `slug` (kebab-case, materialized as a `DirName(slug)` directory — `kandev-<slug>`, or just `<slug>` when the slug already carries the `kandev-` prefix), `description`, `source_type` (`inline` | `local_path` | `git`), `source_locator` (path/URL), `content` (SKILL.md text for inline, null otherwise), `file_inventory` (JSON list of `{name, size}`), `workspace_id` FK, `created_by_agent_instance_id` (nullable; agents only edit skills they created), `is_system` (bool), `system_version` (kandev release).

System skills ship inside the kandev binary (`apps/backend/internal/office/configloader/skills/<slug>/SKILL.md`, `//go:embed`). On every backend start, the office service walks the embedded set and upserts a row per workspace, preserving per-agent `desired_skills` references across content updates. Removed slugs are deleted in place. Startup log: `system skills synced workspaces=N inserted=[…] updated=[…] removed=[…]`.

System SKILL.md carries an optional `kandev:` frontmatter block with `system: true`, `version: "<release>"`, `default_for_roles: [<roles>]`. `default_for_roles` drives auto-attach: a new agent with role `R` automatically gets every system skill whose `default_for_roles` contains `R`, unless the caller passes an explicit `desired_skills`. Users can untick a default-attached system skill on any agent (role default is a soft suggestion).

v1 system-skill set: `kandev-protocol`, `memory`, and `kandev-task-ops` (every role); `kandev-escalation` (worker, specialist, assistant, reviewer); `kandev-team-admin`, `kandev-routines`, `kandev-approvals`, `kandev-config-sync`, and `kandev-projects` (ceo).

### Activity, runs, events

- `office_activity_log` carries `run_id` and `session_id` columns (indexed). Every agent-driven mutation threads the originating run id so per-run "tasks touched" reads are a single `SELECT DISTINCT target_id WHERE run_id = ?`.
- `office_run_events`: `(run_id, seq, event_type, level, payload JSON, created_at)` indexed by `(run_id, seq)`. Captures lifecycle events (init, adapter.invoke, step, complete, error) at well-defined call sites in the orchestrator + office service.
- `office_cost_events` already has `session_id` and `task_id`; per-run cost rollup joins via the session a run claimed.

## Permissions

Permissions are a JSON object on the agent instance. Role determines defaults; individual permissions can be toggled per agent.

| Permission | CEO | Worker | Specialist | Assistant | Reviewer |
|---|---|---|---|---|---|
| `can_create_tasks` | yes | yes | yes | yes | no |
| `can_assign_tasks` | yes | no | no | yes | no |
| `can_create_projects` | yes | no | no | no | no |
| `can_create_agents` | yes | no | no | no | no |
| `can_approve` | yes | no | no | no | yes |
| `can_manage_own_skills` | yes | no | no | yes | no |
| `max_subtask_depth` | 3 | 1 | 1 | 1 | 0 |

`can_manage_own_skills` lets an agent create or edit skills in the registry for itself, subject to approval if `require_approval_for_skill_changes=true`. Agents can only edit skills they created.

### Backend enforcement

Auth middleware on office API routes extracts `Authorization: Bearer <JWT>`, validates signature + expiration, loads the agent instance and resolved permissions, and sets the agent context on the request. UI requests (no JWT / session cookie) bypass as admin.

Service-layer permission checks run on every mutating endpoint. Task scope is enforced: an agent can only operate on the task whose ID matches its run claims, except CEO agents with `can_assign_tasks` which may operate on any task (for delegation).

When a CEO calls `POST /office/agents`: must have `can_create_agents`; must specify `role` (defaults applied automatically); may pass `permissions` overrides, but cannot grant permissions it doesn't have itself (no privilege escalation).

### Required operator boundary (not yet implemented)

The no-JWT UI convention must not apply to execution-profile mutations.
Launcher definitions, executable/prefix argv, CLI configuration, and
environment values are operator control-plane settings. Once operator
authentication is implemented, creating, updating, or deleting those resources
requires an authenticated operator principal; an omitted credential and an
Office agent JWT are both rejected. Operator credentials are never included in
Office runtime environment, workspace files, executor metadata, logs, or
unauthenticated boot/runtime APIs. Full-detail profile and MCP reads containing
literal environment values, headers, or resolved secrets are operator-only;
agent-facing discovery uses a redacted catalog shape. Agent/workspace preview
content runs on an origin that cannot exercise ambient operator credentials or
read operator session/bootstrap state. Until these requirements are enforced,
launcher prefixes are customization rather than an isolation boundary.
Decision:
[ADR-2026-07-24-operator-owned-agent-launcher-settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md).

The interim risk-reduction guard requires a per-boot SPA token on
state-changing agent/settings requests and rejects Office bearer tokens. Because
an intentional agent can fetch and replay the unauthenticated boot payload,
this guard is a CSRF and accidental-mutation interlock only; it does not satisfy
the operator-boundary scenarios below.

### Hire flow

When the CEO (or any instance with `can_create_agents`) creates a new agent instance, a hire request is submitted:

- If the workspace has `require_approval_for_new_agents=true` (default), the hire creates a pending approval in the inbox.
- The user reviews the proposed config (name, role, profile, skills, budget) and approves or rejects.
- On approval, the instance status moves from `pending_approval` to `idle` and becomes available.
- On rejection, the instance is deleted; the requesting agent receives a wakeup with the rejection reason.

## State machine

Agent instance lifecycle:

- **pending_approval**: created via hire request, awaiting user decision.
- **idle**: exists but has no active work. Available for assignment.
- **working**: one or more active sessions running, up to `max_concurrent_sessions`.
- **paused**: manually paused by user, or auto-paused by budget. No new wakeups processed. Active sessions complete their current turn but receive no further prompts.
- **stopped**: deactivated. No longer in the CEO's org tree. Can be reactivated.

Transitions:

| From | To | Trigger | Actor |
|---|---|---|---|
| (none) | pending_approval | hire request via CEO | CEO agent |
| (none) | idle | direct create by user | user |
| pending_approval | idle | approval granted | user |
| pending_approval | (deleted) | approval rejected | user |
| idle | working | scheduler claims a wakeup | scheduler |
| working | idle | last session completes | scheduler |
| any | paused | user clicks Pause, or budget exhausted | user / cost guard |
| paused | idle | user clicks Resume, or budget renewed | user / cost guard |
| any | stopped | user deactivates | user |
| stopped | idle | user reactivates | user |
