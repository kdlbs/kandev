# 0005: Agent model unification — enrich `agent_profiles`, drop `office_agent_instances`

**Status:** proposed
**Date:** 2026-05-06
**Area:** backend, frontend

## Context

Two separate concepts describe "an agent" in the codebase today:

1. **`agent_profiles`** (singular table, defined at
   `apps/backend/internal/agent/settings/store/sqlite.go:56`). A
   profile is a *configured variant* of a CLI agent tool — it selects
   a `model`, a `mode` (e.g. ACP for Claude Code, plan for
   OpenCode), permission flags (`auto_approve`,
   `dangerously_skip_permissions`, `allow_indexing`, `cli_passthrough`),
   and an MCP configuration (in the sibling `agent_profile_mcp_configs`
   table). It belongs to a CLI tool via `agent_id`
   (`agents.id` — the CLI registration table). Profiles are global
   today; the kanban settings UI manages them at
   `/settings/agents/<agent>/profiles/<profile-id>`. Workflow steps,
   workflow_step_participants, kanban tasks, and task_sessions all
   reference profiles via `agent_profile_id`.

2. **`office_agent_instances`** (defined at
   `apps/backend/internal/office/repository/sqlite/agents.go`).
   Workspace-scoped. Carries the *organisational* and *runtime*
   identity of an agent: name (e.g. "Backend Engineer Alice"), role
   (`ceo` / `worker` / `qa_lead`), icon, status,
   pause_reason, skill_ids, custom_prompt, reports_to, budget,
   concurrency caps, failure tracking, etc. Each instance has an
   `agent_profile_id` foreign key pointing at concept (1).

The two-table model exists because office was added on top of an
already-shipped kanban concept. It produces real friction:

- **Workflow_step_participants is keyed by `agent_profile_id`.** For
  office reviewers we need the rich per-workspace identity (skills +
  prompt) but the participants table only references the profile tier.
  Two different reviewer instances sharing one CLI profile become
  indistinguishable in the workflow store. This is "Blocker 1" from the
  task-model-unification work.
- **Office tasks reference instances** via
  `tasks.assignee_agent_instance_id` while workflow steps reference
  profiles. Every launch path threads both ids and resolves the union.
- **Kanban can't use office-created agents.** A CEO with skills + a
  custom prompt is invisible to kanban workflow editors today, even
  though the underlying CLI configuration is exactly the same shape as
  any kanban profile.

The fix is to merge them: enrich `agent_profiles` with the office-only
columns, migrate every office_agent_instance into a profile, and rename
`agent_instance_id` → `agent_profile_id` everywhere in code and
schema. After the merge, `agent_profiles` is the single source of
truth for "an agent" — kanban-shallow agents (no skills, no role) and
office-rich agents (with skills + role + workspace scoping) live as
rows in the same table.

## Decision

**One `agent_profiles` table** that supports two richness levels:

- **Shallow profile** (kanban-style): just the existing columns. No
  workspace scope, no skills, no role. The kanban settings UI at
  `/settings/agents/<agent>/profiles/<profile-id>` continues to manage
  these as today.
- **Rich profile** (office-style): all of the above PLUS workspace
  scope, role, skills, custom_prompt, status, budget, concurrency
  caps, failure tracking, etc. The office agents page surfaces both
  the shallow fields (so the user can pick / change the underlying CLI
  client, model, mode, MCP config) AND the office-only enrichment
  fields.

`office_agent_instances` is dropped. Every `agent_instance_id` in
schema and code becomes `agent_profile_id` referencing the merged
table. Skill deployment moves into the runtime's launch-prep so every
launch (kanban or office) treats `skill_ids` + `custom_prompt`
uniformly — agents with empty values get a no-op deploy.

### Final schema

`agent_profiles` after this ADR:

```sql
CREATE TABLE agent_profiles (
    -- Existing columns (unchanged):
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,                          -- FK → agents (CLI tool)
    name TEXT NOT NULL,
    agent_display_name TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    mode TEXT DEFAULT NULL,
    migrated_from TEXT DEFAULT NULL,
    auto_approve INTEGER NOT NULL DEFAULT 0,
    dangerously_skip_permissions INTEGER NOT NULL DEFAULT 0,
    allow_indexing INTEGER NOT NULL DEFAULT 1,
    cli_passthrough INTEGER NOT NULL DEFAULT 0,
    user_modified INTEGER NOT NULL DEFAULT 0,
    plan TEXT DEFAULT '',
    cli_flags TEXT DEFAULT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP,

    -- NEW (workspace scoping):
    workspace_id TEXT NOT NULL DEFAULT '',           -- '' = global / kanban-legacy; set = workspace-scoped

    -- NEW (organisational identity):
    role TEXT NOT NULL DEFAULT '',                   -- '' | 'ceo' | 'worker' | 'qa_lead' | ...
    icon TEXT NOT NULL DEFAULT '',
    reports_to TEXT NOT NULL DEFAULT '',             -- agent_profiles.id (org chart)

    -- NEW (skills + prompt enrichment):
    skill_ids TEXT NOT NULL DEFAULT '[]',            -- JSON array
    desired_skills TEXT NOT NULL DEFAULT '[]',
    custom_prompt TEXT NOT NULL DEFAULT '',

    -- NEW (runtime state):
    status TEXT NOT NULL DEFAULT 'idle'
        CHECK (status IN ('idle','working','paused','stopped','pending_approval')),
    pause_reason TEXT NOT NULL DEFAULT '',
    last_run_finished_at TIMESTAMP,

    -- NEW (concurrency / scheduling):
    max_concurrent_sessions INTEGER NOT NULL DEFAULT 1,
    cooldown_sec INTEGER NOT NULL DEFAULT 0,
    skip_idle_runs INTEGER NOT NULL DEFAULT 0,

    -- NEW (failure tracking):
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    failure_threshold INTEGER NOT NULL DEFAULT 3,

    -- NEW (executor / cost):
    executor_preference TEXT NOT NULL DEFAULT '',
    cheap_agent_profile_id TEXT NOT NULL DEFAULT '', -- self-ref for low-priority work
    budget_monthly_cents INTEGER NOT NULL DEFAULT 0,

    -- NEW (catch-all for office fields not explicitly listed):
    settings TEXT NOT NULL DEFAULT '{}',

    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);

CREATE INDEX idx_agent_profiles_workspace ON agent_profiles(workspace_id);
CREATE INDEX idx_agent_profiles_role ON agent_profiles(workspace_id, role) WHERE role != '';
CREATE INDEX idx_agent_profiles_reports_to ON agent_profiles(reports_to) WHERE reports_to != '';
```

**MCP config stays in the sibling `agent_profile_mcp_configs` table.**
That's already a 1:1 side table keyed by `profile_id`; it doesn't need
to fold in. Both shallow and rich profiles use it the same way.

### Tables that gain renamed columns

Every table that today references office's instance id by
`*_agent_instance_id` (or its variants) is renamed to
`*_agent_profile_id`, pointing at the merged `agent_profiles` table:

```
office_skills.created_by_agent_instance_id      → created_by_agent_profile_id
office_cost_events.agent_instance_id            → agent_profile_id
office_routines.assignee_agent_instance_id      → assignee_agent_profile_id
office_approvals.requested_by_agent_instance_id → requested_by_agent_profile_id
office_agent_memory.agent_instance_id           → agent_profile_id
office_channels.agent_instance_id               → agent_profile_id
office_agent_instructions.agent_instance_id     → agent_profile_id
office_inbox_dismissals.agent_instance_id       → agent_profile_id
office_workspace_settings.lead_agent_instance_id → lead_agent_profile_id
runs.agent_instance_id                          → agent_profile_id
tasks.assignee_agent_instance_id                → assignee_agent_profile_id
office_task_participants.agent_instance_id      → agent_profile_id
```

(After the merge, the `office_task_participants` table is itself
migrated into `workflow_step_participants` — see "Implementation
sequence" below.)

### Identity preservation in the migration

For each `office_agent_instances` row, we INSERT a new
`agent_profiles` row using the **instance's id as the new profile's
id**. That way every existing FK that today points at the instance id
(tasks, participants, decisions, runs, costs, etc.) continues to work
unchanged after the rename — the column name flips but the value is
preserved.

The instance's old `agent_profile_id` (which today points at a
shallow profile that the instance was configured against) gets its
columns *copied* into the new merged row (model, mode, auto_approve,
mcp_config, etc.). The original shallow profile keeps existing as a
distinct row — kanban tasks that referenced it continue to reference
it. After the migration we'll have:

- The original shallow row (kanban-flavour, workspace_id='').
- A new rich row using the old instance's id (office-flavour,
  workspace_id set, with the CLI fields copied from the original
  shallow row).

Same logical kanban behaviour; rich identity now first-class.

### Migration policy

Per the user's directive: **office tables can be wiped without
migrations** (greenfield — the office feature is in development, not
deployed). For non-office tables (`tasks`, `task_sessions`, `runs`,
`workflow_step_participants`) we write proper migrations because they
carry kanban data.

Concretely:

| Table | Migration policy |
|---|---|
| `office_agent_instances` | wipe + repopulate from migration insert |
| `office_skills`, `office_routines`, `office_approvals`, `office_agent_memory`, `office_channels`, `office_agent_instructions`, `office_inbox_dismissals`, `office_workspace_settings`, `office_cost_events`, `office_task_participants`, `office_task_approval_decisions` | recreate-table with renamed column; data preservation only when trivial — wiping is fine for office state |
| `tasks` | proper recreate-table migration: copy `assignee_agent_instance_id` → `assignee_agent_profile_id`, drop the old column |
| `task_sessions` | column rename if `agent_instance_id` exists (kanban data preserved via the rename) |
| `runs` | column rename, kanban + office data preserved |
| `workflow_step_participants` | already has `task_id` (wave 8 piece 1); no schema change |

For SQLite, the recreate-table pattern handles column renames where
`ALTER TABLE RENAME COLUMN` isn't available. Look at
`apps/backend/internal/task/repository/sqlite/base.go` for the existing
pattern.

### UX

**Kanban settings UI — unchanged.** The page at
`/settings/agents/<agent>/profiles/<profile-id>` continues to manage
just the existing fields (model, mode, MCP config, permission flags,
etc.). For shallow / kanban-style profiles, the office fields are not
exposed — the form looks identical to today.

**Office agents page — gains the full picture.** The office page
surfaces:

- The existing CLI fields: which CLI client (claude / codex /
  opencode-acp / amp / etc.), model, mode, MCP config, permissions.
  Today these fields are configured in kanban settings; on the office
  page they become inline so the user doesn't have to context-switch.
- The office enrichment fields: name, role, icon, reports_to,
  skill_ids, custom_prompt, max_concurrent_sessions, cooldown_sec,
  budget, etc.

A profile created in the office page is a rich profile
(workspace_id set, office fields populated). A profile created in
kanban settings is a shallow profile (workspace_id='', office fields
empty). Either way, the same row in the same table.

The office agents page becomes the recommended place to create an
agent that you intend to use for office work. Existing kanban profiles
continue to work and can be enriched into office agents by setting
their workspace + office fields.

**Office onboarding — adds CLI configuration step.** Today's
onboarding creates a CEO agent with hardcoded defaults. After this
ADR, onboarding asks the user to pick:

- Which CLI client (claude / codex / opencode-acp / amp / ...).
- Which model (claude-sonnet / opus / o3-mini / etc.).
- Which mode (ACP / plan mode / etc.).
- Optionally MCP servers + permission flags (or accept defaults).

These were previously handled by reading the user's existing kanban
settings for an inferred default; making them explicit at onboarding
time matches the "office can configure agents end-to-end" model.

### Skill deployment in runtime

Today: office's launch path materialises `skill_ids` + `custom_prompt`
into the worktree before the agent starts (symlink deploy + prompt
file). After this ADR: that step lives in `internal/agent/runtime/`'s
launch-prep — every launch reads the agent profile, looks at
`skill_ids` and `custom_prompt`, and deploys. Empty values → no-op
fast path. Kanban launches that don't use these fields are unaffected;
kanban profiles that the user later enriches with skills get them on
the next launch.

`runtime.LaunchSpec.AgentProfileID` already exists from Phase 1 of
task-model-unification. The runtime resolves the agent record (now
including office enrichment) from the profiles repo.

### Code-level rename

Across backend (~593 lines, ~131 files) and frontend (~164 lines):

| Old | New |
|---|---|
| `agent_instance_id` (column / JSON / param) | `agent_profile_id` |
| `AgentInstanceID` (Go field) | `AgentProfileID` |
| `agentInstanceId` (TS field) | `agentProfileId` |
| `models.AgentInstance` (Go struct) | merge with `models.AgentProfile` (the existing struct gains office fields) |
| `OfficeAgentInstance` (TS type) | merge with `AgentProfile` |
| `office_agent_instances` (table refs in code) | `agent_profiles` |

Most of this is mechanical rename + recompile. A subset of files have
both `AgentProfileID` and `AgentInstanceID` as separate fields on the
same struct (those collapse — needs hand-judgement per case to ensure
no callsite depended on the two values being different).

## Consequences

### Positive

- **Blocker 1 dissolved.** `workflow_step_participants.agent_profile_id`
  carries skills + prompt for office reviewers. Decision attribution
  is unambiguous because there's one identity per agent.
- **Office agents become available to kanban.** A CEO or Backend
  Engineer profile created in the office page is selectable in any
  workflow editor. Kanban profiles are eligible as office reviewers.
- **One settings model.** Skills + custom prompt + role are first-class
  fields on a profile. The runtime treats every launch the same way.
- **Smaller refactor than the workspace_agents alternative.** ~600
  callsites instead of ~2,762. Tractable in 1–2 focused waves.

### Negative

- **`agent_profiles` becomes a fat table.** ~25 columns, many
  nullable for shallow profiles. Mitigated by clear documentation of
  which fields are kanban-relevant vs office-relevant; the kanban UI
  doesn't expose office columns.
- **Workspace scoping coexistence.** Some profiles are global
  (`workspace_id=''`), some are workspace-scoped. Every read that's
  scoped by workspace must filter by `workspace_id IN ('', $ws)` to
  see both. Acceptable cost; encoded in the repo helpers.
- **Office onboarding gains complexity.** Users now have to make CLI
  choices upfront. Mitigated by sensible defaults (preselect the
  user's current kanban-default profile).

### Risks

- **Field-name collisions on existing structs.** `OfficeAgentInstance`
  has both `ID` and `AgentProfileID` today; merging makes them
  potentially the same value. Each callsite needs case-by-case review.
  Mitigated by mechanical sed for the 80% case + careful review of
  conflicts.
- **Frontend type churn.** `AgentProfile` is referenced in many
  components; adding office fields means TypeScript types widen.
  Mitigated by making office fields optional on the type so existing
  consumers don't break.

## Alternatives considered

### Alternative A: Keep two tables, add a bridge

Reject. Preserves Blocker 1.

### Alternative B: This proposal — enrich `agent_profiles`, drop `office_agent_instances`

Chosen. Smallest viable refactor that solves the impedance mismatch.
Keeps the existing table and its FK relationships. Office becomes a
richer view of the same row.

### Alternative C: New `workspace_agents` table, drop both old tables, full rename

Originally proposed in the first draft of this ADR. Rejected as
disproportionately large (~2,762 callsites for a name change). The
user's directive: "i don't want to move away from agent profiles for
now, its a much bigger change."

## Implementation sequence

Three waves, each shippable on its own:

### Wave A — Foundation (schema + migration only, no callsite changes)

1. Add the new columns to `agent_profiles` (idempotent ALTER TABLE
   migrations).
2. Update the `AgentProfile` Go model to carry the new fields.
3. Update the repo to round-trip them.
4. Migration: insert one `agent_profiles` row per
   `office_agent_instances` row, preserving the instance's id.
5. Skill deployment moves into `internal/agent/runtime/`'s launch-prep
   (no-op if `skill_ids` empty).

After Wave A: `agent_profiles` is the canonical store, both shallow
and rich rows live in it. `office_agent_instances` still exists but is
no longer the source of truth — it becomes a redundant copy until
Wave B.

### Wave B — Callsite rename

1. Rename `agent_instance_id` → `agent_profile_id` columns in all
   non-office tables (`tasks`, `runs`, `task_sessions`,
   `workflow_step_participants`, etc.) — proper migrations.
2. Recreate office tables with renamed columns (no data preservation
   needed; greenfield).
3. Rename `AgentInstanceID` → `AgentProfileID` Go fields, code
   references, frontend types, payloads.
4. Drop `office_agent_instances` table.

After Wave B: `office_agent_instances` is gone; everything references
`agent_profile_id` against the unified `agent_profiles` table.

### Wave C — Dashboard reviewer/approver migration

1. Migrate `office_task_participants` → `workflow_step_participants`
   (using the dual-scope `task_id` from wave 8 piece 1).
2. Migrate `office_task_approval_decisions` → `workflow_step_decisions`.
3. Drop the two legacy office tables.
4. Drop `tasks.assignee_agent_profile_id` entirely. Per-task assignee
   resolves through the workflow step's `primary_agent_profile_id`
   (or `workflow_step_participants` for multi-agent stages). The
   per-task snapshot was redundant.
5. Update office UI to surface the new agent fields (Office Agents
   page changes).
6. Update office onboarding to ask for CLI client + model + mode.

After Wave C: the dashboard reviewer/approver flow uses the unified
participants/decisions tables. The Phase 4 deferred items from
ADR 0004 are resolved.

## Out of scope

- Cross-workspace agent sharing (workspace_id is the boundary).
- Agent versioning beyond `updated_at`.
- Per-tool MCP overrides at the office level (use the existing
  `agent_profile_mcp_configs` for now).
- Renaming `agent_profiles` → `agents` or anything else. Keep the
  current name; it fits.

## References

- ADR 0004 — task-model-unification.
- `docs/specs/task-model-unification/plan.md` Phase 4 deferred items.
- Wave-9 / Wave-10 agent scope surveys (in PR commit messages).
