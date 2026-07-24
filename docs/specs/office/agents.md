---
status: draft
created: 2026-04-25
owner: cfl
---

# Office: Agents

## Why

Kandev has execution profiles (configuration templates for a concrete CLI, account, model, flags, environment, and MCP setup) but also needs persistent, stateful Office agents. Without a stable Office identity, switching a provider also risks switching or copying the agent's role, instructions, skills, permissions, budget, and history.

Office therefore treats a workspace-scoped rich `agent_profiles` row as a stable Office identity and selects a separate concrete execution profile for each launch. Both row types remain in the unified table established by ADR 0005, but they have different responsibilities. Office agents run inside a narrow capability-scoped runtime, call kandev through a structured CLI rather than raw curl, and expose per-agent dashboards with run history, costs, and per-run detail pages.

## What

### Agent instances

- An Office agent is a persistent `agent_profiles` row scoped by `workspace_id`; its row ID is the logical `agent_profile_id` referenced by assignments, instructions, skills, budgets, permissions, and Office history.
- A launch separately resolves an `execution_profile_id` from the agent's effective tier and provider order. The referenced execution profile owns the concrete CLI runtime configuration.
- The Office identity owns:
  - **Name**: human-readable label ("CEO", "Frontend Worker", "QA Bot").
  - **Role**: `ceo`, `worker`, `specialist`, `assistant`, or `reviewer`. Determines default permissions and UI treatment.
  - **Status**: `idle`, `working`, `paused`, `stopped`, plus transitional `pending_approval`.
  - **Permissions**: JSON object controlling what the instance can do.
  - **Budget**: remaining spend allowance (see [costs](./costs.md)).
  - **Skills**: list of assigned skill IDs.
  - **Instructions**: per-agent `AGENTS.md`, `HEARTBEAT.md`, `SOUL.md`, `TOOLS.md`.
  - **Icon**: avatar for UI display.
  - **Executor preference**: optional executor override for this agent.
  - **Channels**: optional external messaging channels (Telegram, Slack).
- Multiple Office agents can use the same execution profile without sharing Office instructions, skills, permissions, budgets, or history.
- Changing an agent's tier or provider order changes future execution-profile resolution without changing the Office identity.
- Provider routing and failover are specified in [routing](./routing.md).

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
3. **Project executor config** (see [projects](./overview.md)).
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
reports_to: ""
icon: crown
permissions: '{"can_create_tasks":true,"can_assign_tasks":true,"can_create_agents":true}'
budget_monthly_cents: 5000
max_concurrent_sessions: 1
desired_skills: '["memory","delegation-playbook"]'
executor_preference: ""
```

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

### Office identities and execution profiles

Office identities and execution profiles stay in the existing `agent_profiles` DB table, as required by ADR 0005, but launch resolution does not treat them as the same logical object:

- A workspace-scoped rich row is the Office identity. Its `agent_profile_id` remains stable across provider changes and is used for hierarchy, instructions, skills, Office permissions, budgets, costs, and task/run ownership.
- A concrete execution profile is selected per launch and recorded as `execution_profile_id`. It owns the registered CLI/provider, credentials and environment, model, mode, ACP config options, CLI flags, CLI permission behavior, passthrough mode, and MCP configuration.
- Global shallow profiles are the normal execution-profile choices. Existing same-workspace rows may remain selectable for upgrade compatibility when they carry a valid CLI configuration.
- An execution profile from another workspace cannot be selected.
- Non-Office launches remain compatible: when no distinct execution profile is supplied, runtime treats `execution_profile_id = agent_profile_id`.

The CLI-shaped columns on rich Office rows remain in the schema for compatibility with ADR 0005 and existing data, but they are not authoritative for routed Office launches. New Office configuration changes select tiers and provider order instead of copying CLI runtime fields into the identity row.

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

`skill` DB row (workspace-scoped): `id` PK, `name`, `slug` (kebab-case, used as `kandev-<slug>` directory), `description`, `source_type` (`inline` | `local_path` | `git`), `source_locator` (path/URL), `content` (SKILL.md text for inline, null otherwise), `file_inventory` (JSON list of `{name, size}`), `workspace_id` FK, `created_by_agent_instance_id` (nullable; agents only edit skills they created), `is_system` (bool), `system_version` (kandev release).

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
[ADR-2026-07-24-operator-owned-agent-launcher-settings](../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md).

An interim risk-reduction guard may require a per-boot SPA token on
state-changing agent/settings requests and reject Office bearer tokens. Because
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

## Runtime

Each scheduler-launched agent turn has a runtime context with workspace, agent, task, run, session, wakeup reason, and capability scope. Agents mutate Office through a narrow action surface, not direct service access.

### Capabilities

A run carries an explicit capability scope. Capabilities include: post comment, update task status, `create_task`, `create_subtask`, create project, request approval, read/write memory, inspect assigned skills.

- Runtime actions check capabilities before mutating state.
- Runtime actions attach agent/run/session identity to emitted records whenever the underlying feature supports it.
- A run may update its current task, any task explicitly granted in the scope, or every task only when the wildcard scope is granted.
- Denied runtime actions fail with a forbidden error and do not call downstream services.

### Environment variables

Injected before each agent session:

| Variable | Value | Purpose |
|---|---|---|
| `KANDEV_API_URL` | `http://localhost:<port>/api/v1` | Base URL for API calls |
| `KANDEV_API_KEY` | Per-run JWT | Bearer token authentication |
| `KANDEV_AGENT_ID` | Agent instance ID | Agent's own identity |
| `KANDEV_AGENT_NAME` | Agent name | Human-readable name |
| `KANDEV_WORKSPACE_ID` | Workspace ID | Scope for API calls |
| `KANDEV_TASK_ID` | Task ID | Which task to work on |
| `KANDEV_RUN_ID` | Wakeup request ID | Audit trail header |
| `KANDEV_WAKE_REASON` | Reason string | Why the agent was woken |
| `KANDEV_WAKE_COMMENT_ID` | Comment ID (if applicable) | Which comment triggered wake |
| `KANDEV_WAKE_PAYLOAD_JSON` | Inline JSON | Pre-computed task context |
| `KANDEV_WAKE_PAYLOAD_PATH` | Workspace-relative JSON file path | Pre-computed task context when too large for inline env |
| `KANDEV_CLI` | Path to agentctl | CLI binary for API operations |

`KANDEV_CLI` resolves per executor:
- **Docker** (`local_docker`): `/usr/local/bin/agentctl` (baked into the image).
- **Standalone** (`local_pc`): path from `launcher.findAgentctlBinary()`.
- **Sprites/Remote**: agentctl path inside the remote environment.

### Wake payload

`KANDEV_WAKE_PAYLOAD_JSON` carries pre-computed context. Fresh session: full task context (`task` object with id, identifier, title, status, priority, project, `blockedBy`, `childTasks`). Resume: only new comments since last run plus a `commentWindow` rollup (`{total, included, fetchMore}`). New comments include author, body, createdAt. If the serialized payload exceeds 64KB for inline environment delivery, Kandev writes it under the workspace and sets `KANDEV_WAKE_PAYLOAD_PATH` to that workspace-relative file path instead.

### Instructions delivery

Same strategy for all agent CLIs (no adapter-specific delivery):

1. Read `AGENTS.md` content from `runtime/<ws>/instructions/<agentId>/AGENTS.md`.
2. Append a **path directive** telling the agent where to find sibling files.
3. Prepend the combined text to the user-turn prompt.
4. Agent reads `HEARTBEAT.md`, `SOUL.md` from disk during the session (via cat, Read tool, etc.).

Path directive appended to `AGENTS.md` content:

```
The above agent instructions were loaded from {instructionsDir}/AGENTS.md.
Resolve any relative file references from {instructionsDir}.
This directory contains sibling instruction files: ./HEARTBEAT.md, ./SOUL.md, ./TOOLS.md.
Read them when referenced in these instructions.
```

**On session resume**: instructions are NOT re-injected (agent CLI retains them). Only the wake context is sent.

### Skill injection

Skill content is stored in the DB (source of truth). Before each session, each desired skill's `SKILL.md` is written into the agent's worktree CWD. If the stored content does not already begin with YAML frontmatter delimited by `---`, runtime materialization prepends generated `name` and `description` frontmatter from the skill slug so agent CLIs can load the file as a native skill.

Each agent type defines `ProjectSkillDir` in its `RuntimeConfig`:

| Agent CLI | `ProjectSkillDir` |
|---|---|
| `claude-acp` (Claude Code) | `.claude/skills` |
| `grok-acp` (Grok) | `.grok/skills` |
| `codex-acp`, `opencode-acp`, `gemini`, `copilot-acp`, `auggie`, `amp-acp` | `.agents/skills` |

Default (if unset): `.agents/skills`. Skills are written to `<worktree>/<ProjectSkillDir>/kandev-<slug>/SKILL.md`. The `kandev-` prefix distinguishes injected skills from team-committed skills already in the repo.

Before writing skills, all existing `kandev-*` directories in the target path are deleted (clean-slate). Removed skills don't linger; updated skills get fresh content.

`kandev-*` patterns are added to `<worktree>/.git/info/exclude` so injected skills never appear as dirty files:

```
.claude/skills/kandev-*
.grok/skills/kandev-*
.agents/skills/kandev-*
```

**Per-agent isolation:** each agent session gets its own worktree (CWD), so skill directories are fully isolated between concurrent agents. No shared HOME directories, no symlink management, no shutdown cleanup hooks.

**Per executor type:**

| Executor | Worktree location | How skills arrive |
|---|---|---|
| `local_pc` / `worktree` | Host filesystem | Written directly by the scheduler |
| `local_docker` | Host dir, mounted into container at same path | Written on host before container start |
| `sprites` | Local staging dir, uploaded during instance setup | Written to staging, uploaded via Sprites filesystem API |

**Compatibility fallback:** for agent types without a known skill directory, the skill's `SKILL.md` content is appended to the system prompt.

### Session preparation flow

When the scheduler processes a wakeup:

1. Resolve agent instance (from wakeup payload).
2. Check guard conditions (status, cooldown, checkout, budget).
3. Export agent instructions from DB to `~/.kandev/runtime/<ws>/instructions/<agentId>/`.
4. Create or reuse session worktree (CWD for the agent process).
5. Clean `kandev-*` from the skill dir; write desired skills to `<worktree>/<ProjectSkillDir>/kandev-<slug>/SKILL.md`; ensure `.git/info/exclude` has `kandev-*` patterns.
6. Build prompt: read `AGENTS.md` content, append path directive, prepend to user-turn prompt, add wake context. For CEO heartbeat: add workspace status section.
7. Set env vars (`KANDEV_API_KEY`, `KANDEV_TASK_ID`, `KANDEV_CLI`, etc.).
8. Set `KANDEV_WAKE_PAYLOAD_JSON` with pre-computed task context, or `KANDEV_WAKE_PAYLOAD_PATH` when the payload is too large for inline env.
9. Launch agent via the task starter (prompt + env, CWD = worktree). Skills are cleaned up automatically when the worktree is deleted at session end.

### Default instruction templates per role

Seeded on agent creation; users edit them in the Instructions tab.

- **CEO `AGENTS.md`**: persona ("You are the CEO. You lead the company, not do individual work."), delegation routing table (code -> CTO, marketing -> CMO, etc.), rules (directly handle first triage and small coordination/status concerns; delegate implementation work; post comments explaining decisions), subtask creation procedure, references to `./HEARTBEAT.md`.
- **CEO `HEARTBEAT.md`** (8-step checklist): read wake reason; if `task_assigned` triage directly and delegate only when independent evidence justifies it; if `task_comment` read and respond; if `task_children_completed` review and complete parent; if `approval_resolved` act on decision; if `heartbeat` check workspace status and reassign stalled tasks; post comments on all actions; exit.
- **Worker `AGENTS.md`**: persona ("You are a worker agent. You implement tasks assigned to you."), procedure (read task -> check blockers -> do the work -> post progress -> update status), rules (only work on assigned tasks, write tests, focused commits), and scope escalation to the CEO rather than recursive self-decomposition by default.
- **Reviewer `AGENTS.md`**: persona ("You are a reviewer. You review work done by other agents."), review checklist (correctness, quality, security, performance), approve/reject procedure, rules (be specific, suggest fixes, approve if meets requirements).

## API surface

### Agent CRUD

- `GET /api/v1/office/agents` - list agents in workspace.
- `POST /api/v1/office/agents` - create agent (UI or CEO).
- `GET /api/v1/office/agents/:id` - agent detail.
- `PATCH /api/v1/office/agents/:id` - update agent (permissions, name, budget, etc.).
- `DELETE /api/v1/office/agents/:id` - delete agent.

### Dashboard and runs

- `GET /api/v1/office/agents/:id/summary?days=14` - aggregate dashboard payload composing existing data (`office_runs`, `tasks`, `office_cost_events`, `office_activity_log`) into the precomputed shapes the four charts and costs view need. Fields:
  - `agent_id`
  - `latest_run` (SessionSummary-shaped, including run id)
  - `run_activity[]`: per-day `{date, succeeded, failed, other, total}`
  - `tasks_by_priority[]`: per-day `{date, critical, high, medium, low}`
  - `tasks_by_status[]`: per-day `{date, todo, in_progress, in_review, done, blocked, cancelled, backlog}`
  - `success_rate[]`: per-day `{date, succeeded, total}`
  - `recent_tasks[]`: `{task_id, identifier, title, status, last_active_at}`
  - `cost_aggregate`: `{input_tokens, output_tokens, cached_tokens, total_cost_cents}`
  - `recent_run_costs[]`: `{run_id, run_id_short, date, input_tokens, output_tokens, cost_cents}`

- `GET /api/v1/office/agents/:id/runs?cursor=&limit=` - cursor-paginated run list. Cursor = `(requested_at, id)` desc. Default limit 25, max 100. Used by both the full-page runs list and the recent-runs sidebar (fixed `limit=30`).

- `GET /api/v1/office/agents/:id/runs/:runId` - run detail: status, short id, invocation source, start/finish timestamps, duration, agent adapter/model, token + cost rollup, `session_id_before`/`session_id_after`, error message, tasks touched, invocation (adapter + cwd + command + env), events list, log offset.

### Permissions metadata

`/meta` includes a `permissions` array (each entry `{key, label, description}` for every permission in the table above) and a `permissionDefaults` object (per role, default value per permission) so the frontend renders the configuration UI without hard-coding the catalogue.

### agentctl CLI surface

Agents call the `kandev` command group on the agentctl binary instead of raw curl. Mutating subcommands use the signed runtime action surface under `/api/v1/office/runtime/…`; they never use generic Kanban or dashboard mutation endpoints. Auth reads `KANDEV_API_URL`, `KANDEV_API_KEY`, `KANDEV_RUN_ID`, `KANDEV_AGENT_ID`, `KANDEV_TASK_ID` from environment. Output is structured JSON by default; `--format text` for human-readable. Errors: non-zero exit + `{"error":"message","code":409}`. Task ID defaults to `$KANDEV_TASK_ID` when `--id`/`--task` is omitted.

```
# Task operations (singular)
agentctl kandev task get    [--id ID]
agentctl kandev task update [--id ID] [--status STATUS] [--comment BODY]
agentctl kandev task create --title TITLE [--description TEXT] [--parent ID] [--assignee AGENT_ID] [--project PROJECT_ID]

# Task operations (plural)
agentctl kandev tasks list         [--status S] [--assignee ID] [--project ID]
agentctl kandev tasks message      --id T-1 --prompt MSG
agentctl kandev tasks conversation --id T-1

# Projects
agentctl kandev projects list
agentctl kandev projects create --name NAME [--description TEXT] [--repository URL_OR_PATH ...] \
  [--lead-agent-profile-id ID] [--color COLOR] [--budget-cents N] [--executor-config JSON]

# Comments + memory + checkout
agentctl kandev comment add        --task ID --body BODY
agentctl kandev comment list       --task ID [--limit N] [--after COMMENT_ID]
agentctl kandev memory get         [--layer LAYER] [--key KEY]
agentctl kandev memory set         --layer LAYER --key KEY --content CONTENT
agentctl kandev memory summary
agentctl kandev checkout           --task ID

# Agents (CEO-only roster control)
agentctl kandev agents list   [--role ROLE] [--status STATUS]
agentctl kandev agents create --name N --role R [--budget-monthly-cents …] [--reason …]
agentctl kandev agents update --id A-1 [--name …] [--budget-monthly-cents …]
agentctl kandev agents delete --id A-1

# Routines
agentctl kandev routines list
agentctl kandev routines create --name N --task-title T --assignee A-1 \
  [--cron "0 9 * * MON-FRI"] [--timezone TZ] [--concurrency …]
agentctl kandev routines pause   --id R-1
agentctl kandev routines resume  --id R-1
agentctl kandev routines delete  --id R-1

# Approvals
agentctl kandev approvals list   [--status pending|approved|rejected]
agentctl kandev approvals decide --id AP-1 --decision approve|reject [--note …]

# Budget
agentctl kandev budget get [--agent-id A-1]
```

### MCP modes

Three MCP modes coexist:

| Mode | Tools | Token cost/turn | Used by |
|---|---|---|---|
| `ModeTask` | 27 (kanban + plans + walkthroughs + coordination + completion) | ~3-5K | Interactive kanban sessions |
| `ModeConfig` | 29 (workflows + agents + executors) | ~8-10K | Config setup sessions |
| `ModeOffice` | 9 (plans + ask_user + related tasks + task documents) | ~1-2K | Office agent sessions |

Agent routing and Office ownership are independent. Workflow-level defaults, per-step agent profiles, and `runner` participants select the execution identity only; they never make a Kanban task Office-owned. A task is Office-owned only when it is linked to an Office project or its workflow matches the workspace's Office workflow.

`ModeOffice` includes:
- 4 plan tools (`create_task_plan_kandev`, `get_task_plan_kandev`, `update_task_plan_kandev`, `delete_task_plan_kandev`).
- `ask_user_question_kandev` (only meaningful when the user opens the task in advanced mode).
- `list_related_tasks_kandev`.
- 3 task-document tools (`list_task_documents_kandev`, `get_task_document_kandev`, `write_task_document_kandev`).

`ModeOffice` excludes kanban tools, config tools, `list_workspaces_kandev`, `list_workflows_kandev`, `list_workflow_steps_kandev`, and `step_complete_kandev`. Office advances work through its Office task and approval surfaces, not the Kanban workflow-step completion signal. The first-turn Office context lists only tools registered in `ModeOffice` and directs Office mutations to `$KANDEV_CLI`; it never advertises an excluded tool.

### Skills are preferred over MCP tools

Skills are the preferred pattern for teaching agents office capabilities. A skill provides instructions in `SKILL.md` and the agent calls API endpoints via `$KANDEV_CLI`. This is cheaper than MCP tools: instructions read once per session, shell calls thereafter; MCP tool definitions add per-call overhead (tool schemas in context, structured I/O parsing on every invocation). The `kandev-protocol` system skill teaches CLI usage and replaces the earlier curl-based version. New office capabilities expose API endpoints, ship a skill that teaches the agent how to call them, and assign the skill to agents that need it.

## UI

### `/office/agents`

Agent list page: cards for each instance (icon, name, role, status indicator, current task if working, budget gauge, skill badges); "+" button to create a new instance (select profile, set name/role/skills/budget); sidebar "Agents" section shows a compact list of all instances with status dots and channel indicators (Telegram, Slack icons if configured); each card shows pending wakeup count and oldest wait time when the agent has a backlog ("3 pending, oldest: 12m ago").

### `/office/agents/[id]`

Real bookmarkable sub-routes; tab strip is `<Link>`s to each sub-route. Default redirects to `/dashboard`.

```
/office/agents/[id]
├── /                -> redirect to /dashboard
├── /dashboard       -> charts, latest run, recent tasks, costs
├── /instructions    -> instruction files
├── /skills          -> assigned skills
├── /configuration   -> permissions + model + executor
├── /runs            -> cursor-paginated run list
├── /runs/[runId]    -> run detail
├── /memory          -> memory entries
├── /channels        -> messaging channels
└── /budget          -> cost limits + spend
```

Every page is a Next.js Server Component that fetches initial data on the server (direct HTTP to the Go backend) and hydrates a Client Component with the response. Server Component owns the data fetch; Client Component owns interactivity (collapsibles, "Load more", live mode WS). Live mode is a strict enhancement: when a run is RUNNING, the Client Component subscribes to the run WS channel and merges appended messages/events into the SSR-supplied initial state.

#### Dashboard

- **Latest Run card** at the top: status badge, short run id (8 chars), invocation-source pill (`task_assigned`, `task_comment`, `manual_resume_after_failure`, etc.), one-line replied-to summary, relative timestamp, click-through to the run detail.
- **Four 14-day charts** in a 2×2 grid (4×1 on wide screens):
  - **Run Activity**: stacked bars (succeeded / failed+timed_out / other).
  - **Tasks by Priority**: stacked bars (critical / high / medium / low).
  - **Tasks by Status**: stacked bars (todo / in_progress / in_review / done / blocked / cancelled / backlog).
  - **Success Rate**: succeeded ÷ total per day, as a percentage bar or thin line.
- All charts are custom SVG flexbox bars (no chart library). 14-day window is fixed for v1.
- **Recent Tasks**: last 10 tasks the agent worked on, sorted by most recent activity. Identifier + title + status badge. Row click opens the task page.
- **Costs**: aggregate row (input / output / cached / total) plus per-run table for last 10 runs with cost (date / short run id / input / output / cost).

All dashboard data comes from `GET /api/v1/office/agents/:id/summary?days=14` (single round-trip).

#### Run detail

`/office/agents/[id]/runs/[runId]`.

- **Recent runs sidebar** (left): chronological strip of last ~30 runs, each row with status icon (animated when RUNNING), short run id, invocation-source pill, timestamp, one-line summary, optional token + cost. Active row highlighted.
- **Header strip** (main panel):
  - Status badge (queued / running / failed / completed / cancelled / scheduled_retry).
  - Adapter family + model (`claude_local · claude-sonnet-4-6`).
  - Time range: absolute start/end + relative + duration.
  - Token + cost summary (input / output / cached / total).
  - Action buttons by status: **Cancel** (RUNNING), **Resume session** + **Start fresh** (FAILED), **Retry** (scheduled_retry).
  - "Auth required" banner when the error indicates an expired token, with link to agent settings.
- **Session collapsible**: `session_id_before`, `session_id_after`, underlying ACP session id. "Reset session for touched tasks" action clears the resume token on each affected `(task, agent)` pair.
- **Tasks Touched table**: distinct tasks the agent acted on during the run. Each row links to the task. Sourced from `office_activity_log` rows whose `run_id` matches, plus the run's primary task.
- **Invocation panel**: adapter type, working directory, optional Details collapsible with command, env vars, prompt context.
- **Transcript**: embed the existing session-messages component (`AdvancedChatPanel` / `MessageList` from `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`), scoped to the run's `session_id`. It already supports messages, tool calls, status rows, scrollback, and live updates.
- **Events log**: structured run events (init, adapter invoke, completion, errors) with timestamp, level, stream (system / stdout / stderr).
- **Live mode**: when running, transcript and events stream in via the existing session WS channel filtered by `run_id`.

#### Other tabs

- **Instructions**: file list (`AGENTS.md` marked ENTRY, `HEARTBEAT.md`, `SOUL.md`, `TOOLS.md`) with byte sizes. Click to view/edit (markdown editor). "+" button to add custom instruction files. Default templates seeded by role on agent creation. `AGENTS.md` is required; others optional. Changes save to DB immediately.
- **Skills**: assigned skills with enable/disable toggles. Agent-created skills marked with an indicator. System skills show a "System" badge with the kandev release version (`system_version`); edit/delete affordances are hidden.
- **Configuration**: all permissions as labeled toggles with on/off state. Role defaults shown as baseline (dimmed label: "from role: worker"). User can toggle to override. `max_subtask_depth` is a number input. Model and executor settings on the same tab. Saves via `PATCH /agents/:id`.
- **Memory**: browsable entries grouped by layer (operating, knowledge, session). View, delete, clear all, export, search.
- **Channels**: configured messaging channels with status, platform icon, setup/edit.
- **Budget**: cost limits and current spend (see [costs](./costs.md)).

### `/office/workspace/skills`

Skill list (name, description, source type, which agents use each skill), inline editor for SKILL.md content with markdown preview, import flow for local path or git URL, assignment panel selecting which agent instances receive a skill. System skills are read-only with a "System" badge.

## Failure modes

- **Denied runtime action**: agent attempts an action it lacks capability or permission for. Runtime returns a forbidden error; no downstream service is called; no DB mutation occurs.
- **Out-of-scope task mutation**: agent attempts to mutate a task outside its claim scope. Backend returns 403; activity log records the rejection.
- **Privilege escalation attempt**: CEO tries to grant a permission it doesn't have when creating a new agent. Request rejected at the service layer; no agent created.
- **Subtask depth exceeded**: agent attempts to create a subtask deeper than `max_subtask_depth`. Creation rejected; agent receives the rejection in the response.
- **Hire rejection**: user rejects a hire request. The pending agent row is deleted; the requesting agent receives a wakeup with the rejection reason.
- **Budget exhaustion**: agent's budget reaches zero. Status auto-transitions to `paused` with `pause_reason="budget"`. Active sessions complete the current turn but no further prompts are dispatched. Surfaces as a banner on the agent card. See [costs](./costs.md).
- **Concurrency saturation**: agent at `max_concurrent_sessions`. Scheduler skips claiming wakeups for this agent; wakeups remain in `queued` indefinitely until a slot frees up. No retry, no expiry.
- **Stale MCP tool reference**: agent in `ModeOffice` calls a tool not in the mode (e.g. a kanban tool from an old skill). MCP server returns `"Tool not available in office mode. Use $KANDEV_CLI instead."`.
- **CLI auth failure**: agentctl call returns 401 because the JWT is expired or invalid. The CLI exits non-zero with structured error. Agent sees a clear failure and can surface it via comment.
- **Adapter without skill discovery**: agent type has no known `ProjectSkillDir`. Skill `SKILL.md` content appended to the system prompt as fallback.
- **Skill registry edit while session runs**: the running session is unaffected (file already written). Next session for that agent picks up updated content.
- **Worktree deletion**: when the worktree is deleted at session end, all injected skill directories are removed automatically. No explicit cleanup hook needed.

## Persistence guarantees

What survives a kandev backend restart:

- **Agent identity** persists in `agent_profiles` (Office rows: `workspace_id != '' AND deleted_at IS NULL`). Name, role, icon, `reports_to`, Office permissions, budget, concurrency/cooldown settings, skills, instructions, executor preference, and failure policy are durable on the stable identity. Startup profile reconciliation must not replace an Office agent's name with a generated model label.
- **Execution configuration** is referenced, not copied, for Office launches. The resolved `execution_profile_id` supplies the provider CLI, account environment, model, mode, ACP config options, CLI flags and permissions, passthrough behavior, and MCP configuration. Route attempts and sessions retain that ID for audit and restart recovery.
- **Runtime status** persists in `office_agent_runtime` (PK `agent_id`). On restart a `paused` agent stays `paused`, `pause_reason` (e.g. `"budget"`) is preserved, and `last_run_finished_at` is retained so the cooldown guard works across restarts.
- **Reconciliation at startup**: `infra.Reconciler.ReconcileAll` (called once during boot) drops `office_agent_runtime` rows whose `agent_id` no longer exists in `agent_profiles`, deletes `office_channels` and `office_budget_policies` rows that reference removed agents/projects, and seeds default routine triggers for routines without one. Reconciliation is best-effort: any sub-step that errors is logged but does not block boot.
- **Hire requests** persist as `pending_approval` agent rows plus an approval entry in the inbox. A restart mid-hire leaves the approval visible; the user can still approve or reject and the same activation/deletion paths run.
- **Instructions** (`AGENTS.md`, `HEARTBEAT.md`, `SOUL.md`, `TOOLS.md` and any custom files) live in the office DB (`office_agent_instructions`) as the source of truth. The exported copy under `~/.kandev/runtime/<workspace-slug>/instructions/<agentId>/` is regenerated from DB on every session preparation — losing or wiping this directory between runs has no observable effect.
- **Skills** persist in the workspace `skill` table (DB is the source of truth for inline skills; git-sourced skills cache their `file_inventory` in DB and re-clone on demand). Skill content materialized into `<worktree>/<ProjectSkillDir>/kandev-<slug>/` is ephemeral: it is rewritten at the start of every session and the `kandev-*` patterns added to `.git/info/exclude` are idempotent.
- **System skills** are re-synced from the embedded `//go:embed` set on every boot via `office.service.SystemSkills.Sync`. Removed slugs are deleted, content is upserted, and per-agent `desired_skills` references are preserved across content updates.
- **Run history** (`office_runs`, `office_run_events`, `office_activity_log`, `office_cost_events`) is fully durable. Per-run lookups (`tasks_touched`, costs, events) survive restarts because each row carries `run_id`/`session_id`.
- **Filesystem config** (`workspace/agents/<name>.yml`, `workspace/skills/`, `workspace/routines/`) is a separate snapshot used by `config.ScanFilesystem` for the Sync UI. It is not authoritative: the DB is, and a missing or stale on-disk config never breaks the runtime — at most the Incoming/Outgoing diff is empty or noisy until the user re-syncs.

What does NOT survive a restart:

- **In-flight agent sessions**: the agent subprocess and its agentctl HTTP server are owned by the kandev process; both die when the backend exits. The `TaskSession.ACPSessionID` and `office_runs` row are retained, so the orchestrator's `RecoverInstances` path can resume the session — but the partially-streamed turn at the moment of shutdown is lost unless the underlying CLI itself supports replay.
- **Queued wakeups for capacity-saturated agents**: wakeups that were already persisted survive (they live in the same DB tables as the rest). Wakeups that were only held in memory at the time of shutdown are lost; the originating event (task assign, comment, routine fire) must re-emit to re-queue.
- **Per-run JWTs** (`KANDEV_API_KEY`) and the rest of the agent's environment block are minted fresh per session. Old JWTs are not honoured after restart.
- **Worktrees on disk** belong to `task_session_worktrees`. They survive a backend restart but are reaped by the worktree GC if their owning `TaskSession` is gone. Injected `kandev-*` skill directories under a surviving worktree are stale until the next session rewrites them (the clean-slate step at session start handles this).

There are no TTLs on agent rows, runtime rows, instructions, skills, run history, or activity logs. Retention is by user action only.

## Scenarios

### Agent lifecycle

- **GIVEN** a workspace with no agent instances, **WHEN** the user creates a CEO instance (selecting a profile, role=ceo), **THEN** the instance appears in the agents list with status `idle` and the sidebar shows it under "Agents".

- **GIVEN** an existing CLI profile with a custom mode, permission behavior, flags, config options, environment variables, and MCP setup, **WHEN** routing selects it for an Office launch, **THEN** that complete execution profile is used while the Office name, role, instructions, skills, permissions, budget, and history remain unchanged.

- **GIVEN** the same Office agent later resolves from a Codex execution profile to a Claude execution profile, **WHEN** the new route launches, **THEN** the task and Office identity remain stable, Claude receives its own complete runtime configuration, and no Codex-native resume token is sent to Claude.

- **GIVEN** an Office task assigned directly to an Office agent and no existing task session, **WHEN** the task session is prepared, **THEN** the assignee's stable `agent_profile_id` is used before the workspace default so the session can start.

- **GIVEN** a running CEO instance, **WHEN** the CEO determines a task requires a frontend specialist and no suitable worker exists, **THEN** the CEO submits a hire request for a new worker with appropriate skills, and the request appears in the user's inbox as a pending approval.

- **GIVEN** a pending hire approval, **WHEN** the user approves it, **THEN** the new agent activates (status=idle), appears in the org tree under the CEO, and the CEO receives a wakeup notification.

- **GIVEN** a worker instance with `can_create_tasks=true`, **WHEN** the worker creates a subtask exceeding `max_subtask_depth`, **THEN** the creation is rejected and the worker is informed.

- **GIVEN** a worker instance with status `working`, **WHEN** the user clicks "Pause", **THEN** the current session completes its turn, the instance moves to `paused`, and no new wakeups are processed.

### Runtime capabilities

- **GIVEN** an agent run scoped to task `KAN-1`, **WHEN** the agent posts a comment on `KAN-1`, **THEN** Office records an agent-authored comment tied to that run context.

- **GIVEN** an agent run scoped to task `KAN-1`, **WHEN** the agent tries to update `KAN-2` without explicit scope, **THEN** the runtime denies the action and no task mutation is attempted.

The following operator-boundary scenarios are acceptance criteria that must
pass before launcher settings are described as an isolation boundary:

- **GIVEN** a running Office agent that can reach the backend, **WHEN** it
  submits an execution-profile mutation with its runtime JWT or without a
  credential, **THEN** the backend rejects the request and persists no launcher
  or environment change.

- **GIVEN** an authenticated operator authorized for the target profile,
  **WHEN** the settings UI saves an execution-profile change, **THEN** the
  backend persists the change without exposing the operator credential to any
  agent runtime surface.

- **GIVEN** an Office agent requests execution-profile discovery, **WHEN** the
  backend returns the agent-facing catalog, **THEN** the response contains no
  literal profile environment values, MCP environment values, MCP headers, or
  resolved secret material.

- **GIVEN** agent-controlled JavaScript is rendered in a task preview, **WHEN**
  it attempts to use the operator session, **THEN** browser origin isolation
  prevents it from reading operator state or issuing an authenticated
  control-plane mutation.

- **GIVEN** an agent run with `create_subtask` capability and mutation scope for a parent task, **WHEN** it creates a subtask under that parent, **THEN** Office creates the task through the runtime action surface and preserves the caller agent identity.

- **GIVEN** an agent run without `create_subtask` capability or without mutation scope for the requested parent, **WHEN** it attempts parented task creation, **THEN** Office rejects the request before reading parent relationships or creating any task.

- **GIVEN** a run without a capability, **WHEN** the agent attempts the matching action, **THEN** Office returns a forbidden error and logs no downstream mutation.

### Context and instructions

- **GIVEN** a CEO agent assigned a new task, **WHEN** the scheduler wakes it, **THEN** the agent's `AGENTS.md` with delegation rules is in the system prompt, `HEARTBEAT.md` is on disk at the instructions dir, env vars are set, and the wake payload contains the task details.

- **GIVEN** a worker agent being resumed for a `task_comment`, **WHEN** it's a resume session, **THEN** only the new comment is sent in the prompt (instructions not re-injected; agent CLI retains them).

- **GIVEN** a user editing the CEO's `AGENTS.md` in the Instructions tab, **WHEN** they save, **THEN** the DB is updated. The next time the CEO wakes, the updated instructions are exported to disk and used.

- **GIVEN** a reviewer agent woken for a review, **WHEN** the scheduler prepares the session, **THEN** the reviewer's `AGENTS.md` (review checklist) is in the prompt, its desired skills are written to the worktree, and the wake payload contains the task's changes.

### Skills

- **GIVEN** a user on `/office/workspace/skills`, **WHEN** they click "Add Skill" and enter a name, description, and SKILL.md content, **THEN** the skill appears in the registry and is available for assignment.

- **GIVEN** a skill assigned to a Claude Code worker, **WHEN** the worker starts a new session, **THEN** the skill's `SKILL.md` is written to `<worktree>/.claude/skills/kandev-<slug>/SKILL.md` (Claude's `ProjectSkillDir`) and begins with YAML frontmatter. For non-Claude agents, the path is `<worktree>/.agents/skills/kandev-<slug>/SKILL.md`.

- **GIVEN** a skill sourced from a git URL, **WHEN** the user creates the skill entry, **THEN** the repository is cloned and cached and the file inventory displays in the UI.

- **GIVEN** a running session with injected skills, **WHEN** the user edits the skill in the registry, **THEN** the running session is unaffected (file already written). The next session for that agent picks up updated content.

- **GIVEN** an agent with three assigned skills, **WHEN** the user removes one from the agent config, **THEN** the next session only writes the remaining two skills to the worktree.

### CLI and MCP

- **GIVEN** a worker agent woken for `task_assigned`, **WHEN** it needs to update task status, **THEN** it runs `$KANDEV_CLI kandev task update --status in_progress`, which reads auth from env vars, calls `POST /api/v1/office/runtime/tasks/:id/status`, and returns structured JSON.

- **GIVEN** an Office agent wants to add a comment without changing status, **WHEN** it invokes `task update --comment` without `--status`, **THEN** the CLI rejects the command locally and directs it to the signed `tasks message` surface.

- **GIVEN** a CEO agent delegating work with `create_subtask` capability and scope for the parent, **WHEN** it creates a subtask, **THEN** it runs `$KANDEV_CLI kandev task create --title "..." --parent $KANDEV_TASK_ID --assignee agent_id`, which calls `POST /api/v1/office/runtime/tasks` with the run token and returns the created task ID.

- **GIVEN** an Office agent tries to move a task between workflow steps or archive it, **WHEN** it invokes `tasks move` or `tasks archive`, **THEN** the CLI fails before making an HTTP request and directs the operation to a human or admin because no signed Office runtime capability exists for it.

- **GIVEN** a CEO with `can_create_projects=true`, **WHEN** the setup task requires a project for a repository, **THEN** it runs `$KANDEV_CLI kandev projects create --name "..." --repository "..."`, the project is created in `KANDEV_WORKSPACE_ID`, and the structured response contains its ID.

- **GIVEN** an Office agent without `can_create_projects`, **WHEN** it attempts `projects create`, **THEN** the request is rejected with 403 and no project is created.

- **GIVEN** an Office agent creating a follow-up task for an existing project, **WHEN** it passes `--project PROJECT_ID`, **THEN** `task create` sends `project_id`, runtime validation forces the token workspace and verifies the project, parent, and assignee are owned by it, and the created runner is assigned to the requested Office agent.

- **GIVEN** a direct Office runtime caller, **WHEN** it supplies a cross-workspace project, parent, or assignee, **THEN** task creation is denied before persistence or wakeup.

- **GIVEN** an Office agent passes priority, blocker, or workspace-policy flags to `task create`, **WHEN** the runtime cannot preserve those fields, **THEN** the CLI rejects the request explicitly instead of silently dropping them.

- **GIVEN** a user viewing an office task in advanced mode, **WHEN** the agent needs clarification, **THEN** it uses `ask_user_question` (available in ModeOffice) and the user sees the question in the UI.

- **GIVEN** an office agent in ModeOffice, **WHEN** something tries to call `create_task_kandev` MCP tool, **THEN** the MCP server returns an error saying to use `$KANDEV_CLI` instead.

- **GIVEN** an office agent in ModeOffice, **WHEN** its first-turn system context is generated, **THEN** every advertised MCP tool is registered in ModeOffice and `step_complete_kandev` is absent.

- **GIVEN** a regular kanban task (non-office), **WHEN** a user starts a session, **THEN** ModeTask is used with the full Kanban MCP surface, including `step_complete_kandev`. No Office CLI capability changes that behavior.

- **GIVEN** a regular Kanban task on a step with a default agent profile, **WHEN** the runner projection resolves that profile, **THEN** the profile selects the execution identity without changing Office ownership or the session's `ModeTask` MCP surface.

- **GIVEN** a Docker executor, **WHEN** the agent runs `$KANDEV_CLI kandev task get`, **THEN** agentctl resolves to `/usr/local/bin/agentctl` (on PATH inside the container), reads env vars, and calls the backend API.

- **GIVEN** a worker in a Docker container, **WHEN** the scheduler prepares the session, **THEN** skill files are written to the worktree on the host (under the agent type's `ProjectSkillDir`), the worktree is mounted into the container, and the agent discovers skills in its CWD.

- **GIVEN** a CEO on Sprites, **WHEN** the scheduler prepares the session, **THEN** skill and instruction files are uploaded via the filesystem API to the equivalent paths inside the sprite.

### Permissions

- **GIVEN** a worker that calls `POST /office/agents`, **WHEN** the backend validates the JWT, it loads the worker's permissions, sees `can_create_agents: false`, and returns 403 Forbidden.

- **GIVEN** a CEO creating a worker, **WHEN** it passes `role: "worker"` with no permission overrides, **THEN** the backend applies worker defaults. The CEO can optionally pass `permissions: {"can_assign_tasks": true}` to give the worker delegation ability.

- **GIVEN** a user on the agent detail page, **WHEN** they click Configuration, **THEN** they see all permissions as toggles with the role default indicated. They can override any permission and save.

- **GIVEN** a CEO trying to create an agent with `can_create_agents: true`, **WHEN** the CEO itself has that permission, **THEN** it's allowed. If a worker (who lacks it) tries the same, it's rejected.

### Dashboard and runs

- **GIVEN** an agent with run history, **WHEN** the user opens `/office/agents/:id/dashboard`, **THEN** the page returns server-rendered with the latest run card, the four 14-day charts, recent tasks, and costs populated from a single `summary` round-trip.

- **GIVEN** a RUNNING run, **WHEN** the user opens its run detail page, **THEN** the SSR-supplied transcript and events render immediately and the Client Component subscribes to the run's WS channel, merging appended messages into state without a refetch.

- **GIVEN** a FAILED run with an auth error, **WHEN** the user opens the run detail, **THEN** the header shows an "auth required" banner linking to the agent settings.

- **GIVEN** a long run list, **WHEN** the user scrolls `/office/agents/:id/runs`, **THEN** "Load more" fetches the next page using a `(requested_at, id)` cursor with stable order.

## Out of scope

- Agent-to-agent real-time communication. Agents communicate via tasks and comments only.
- Custom agent binaries. Instances use the existing agent registry (Claude, Codex, Copilot, etc.).
- Automatic scaling of agent instances by workload; agent migration between workspaces.
- Replacing the existing scheduler. Distributed scheduling or multi-backend leader election. A public external API for third-party agent runtimes.
- `SOUL.md` / `TOOLS.md` content for v1 (empty files created, content later). Automatic `TOOLS.md` generation from API schema.
- Per-task instruction overrides. All agents of a role share the same instructions.
- Skill marketplace or cross-workspace skill sharing. Skill versioning beyond what git-sourced skills offer.
- Skill-level permissions. All skills are available to all agents in the workspace; assignment is the access control.
- Automatic skill recommendation based on task content.
- Bash completion for `agentctl kandev` subcommands. Offline/cached mode for CLI. Incremental skill sync.
- CLI commands for workspace config CRUD (workflows, executors). Handled by ModeConfig MCP tools used by IDE agents.
- CLI creation of additional Office workspaces. Office agents operate inside `KANDEV_WORKSPACE_ID`; humans create or import workspaces through onboarding and settings.
- Configurable date range on dashboard charts. Fixed 14 days for v1.
- Bespoke transcript renderer or adapter-specific "Nice mode" parsers. The existing session-messages component is reused.
- Object-store offload for run logs. The `office_run_events` table covers the structured event log.
- Per-run browser/system notifications. The inbox covers the failure path.
- Pagination for the recent-runs sidebar beyond a fixed ~30 row window.
