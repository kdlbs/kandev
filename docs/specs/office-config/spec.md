---
status: draft
created: 2026-04-25
owner: cfl
---

# Office: Configuration Storage & Sync

## Why

Configuration (agents, skills, projects, routines, workspace settings) needs to be both reliably stored AND portable. Teams need to version-control agent configs with PRs, share skills across workspaces, and back up their setup. But the database must remain the source of truth for operational reliability -- accidental file deletions, parse errors, and partial writes should never break a running system.

Office uses a **DB-first model with filesystem sync**: the database is the source of truth for all config. The filesystem (`~/.kandev/`) is a portable sync target for git versioning, sharing, and backup. Users control when changes flow in either direction via a Sync UI.

## What

### Storage model

```
DB (source of truth)  <--- user approves --->  Filesystem (portable copy)
     │                                              │
     ├── agents                                     ├── agents/*.yml
     ├── skills                                     ├── skills/*/SKILL.md
     ├── projects                                   ├── projects/*.yml
     ├── routines                                   ├── routines/*.yml
     ├── workspace settings                         ├── kandev.yml
     ├── wakeup queue                               │
     ├── cost events                                │  (not synced)
     ├── activity log                               │
     ├── approvals                                  │
     └── runtime state                              │
```

- **All CRUD operations** read from and write to the database.
- **The filesystem** is an import/export target -- not read during normal operation.
- **Import (FS -> DB)**: user reviews a diff of incoming changes, approves, and changes are applied to the DB.
- **Export (DB -> FS)**: user reviews what's missing or different on disk, then writes files.
- **No automatic reconciliation** -- the user is always in control of when sync happens.

### Why DB-first (not filesystem-first)

- **Safety**: accidental file deletion, parse errors, git conflicts don't break the running system.
- **Cloud-ready**: shared PostgreSQL for team/SaaS offering -- just works.
- **Atomic operations**: DB transactions for budget checks, wakeup claims, approval flows.
- **No race conditions**: no fsnotify/reload/cache-invalidation complexity.
- **Agent profiles**: stay in existing kandev DB tables, referenced by ID from office agents.

### Filesystem directory structure

Three top-level areas with clear purposes:

```
~/.kandev/
├── workspaces/                              # user config (git-syncable)
│   ├── default/                             # first workspace (slug: "default")
│   │   ├── kandev.yml                       # workspace settings
│   │   ├── agents/
│   │   │   ├── ceo.yml
│   │   │   └── frontend-worker.yml
│   │   ├── skills/
│   │   │   ├── code-review/
│   │   │   │   └── SKILL.md
│   │   │   └── deploy-runbook/
│   │   │       ├── SKILL.md
│   │   │       └── scripts/deploy.sh
│   │   ├── routines/
│   │   │   └── daily-digest.yml
│   │   └── projects/
│   │       └── api-migration.yml
│   │
│   └── my-team/                             # repo-backed workspace (slug: "my-team")
│       ├── .git/
│       ├── kandev.yml
│       ├── agents/
│       ├── skills/
│       └── ...
│
├── system/                                  # bundled with kandev binary, read-only
│   └── skills/
│       ├── kandev-protocol/SKILL.md
│       └── memory/SKILL.md
│
└── runtime/                                 # generated at session time, ephemeral
    ├── default/                             # per-workspace runtime cache
    │   ├── instructions/                    # exported agent instructions
    │   │   └── <agentId>/
    │   │       ├── AGENTS.md
    │   │       └── HEARTBEAT.md
    │   └── skills/                          # exported skill content from DB
    │       └── code-review/
    │           └── SKILL.md
    └── my-team/
        └── ...
```

- **`workspaces/`**: user config, git-syncable. Each workspace is a directory named by its immutable slug.
- **`system/`**: bundled with kandev binary, read-only, updated on upgrade.
- **`runtime/`**: generated from DB before agent sessions, ephemeral. Can be deleted anytime -- rebuilt from DB on next session. Per-workspace subdirectories.

### Workspace slugs

Workspace directories use an **immutable slug** generated at creation time, not the display name:

- User input: "My Team Workspace" -> slug: `my-team-workspace`
- Display name can be renamed freely without moving directories or breaking paths
- The slug is set once and never changes

**Sanitization rules:**
- Lowercase
- Replace spaces and underscores with hyphens
- Strip non-alphanumeric characters (except hyphens)
- Collapse multiple consecutive hyphens
- Trim leading/trailing hyphens
- Max 50 characters
- If empty after sanitization: `workspace-<shortId>`
- If duplicate slug: append `-2`, `-3`, etc.

**Storage:**
- DB `workspaces` table: `id` (UUID), `name` (display name, editable), `slug` (filesystem name, immutable)
- Filesystem: `~/.kandev/workspaces/<slug>/`
- Runtime: `~/.kandev/runtime/<slug>/`

The first workspace created during onboarding gets slug `default`.

Files use the same YAML/markdown format as before. The structure is identical -- only the direction of truth changes (DB -> FS on export, FS -> DB on import).

### Sync UI

The settings page has a **Sync** section that shows:

**Incoming changes (filesystem -> DB):**
- Scans the workspace filesystem directory
- Compares with current DB state
- Shows a diff: new entities (green +), modified entities (yellow ~), deleted entities (red -)
- User clicks "Review & Apply" to preview details, then confirms
- Changes are applied to DB as normal CRUD operations

**Outgoing changes (DB -> filesystem):**
- Compares current DB state with what's on disk
- Shows entities that are in DB but missing/different on filesystem
- User clicks "Export to FS" to write files
- If workspace is a git repo: user can then commit + push

**No automatic sync.** The user decides when to import or export. This prevents surprises.

### Git integration

For repo-backed workspaces:
- **Setup**: `git clone <repo-url> ~/.kandev/workspaces/<name>/`
- **Pull new config**: `git pull` -> Sync UI shows incoming diff -> user applies
- **Push config changes**: Export to FS -> `git add` -> `git commit` -> `git push`
- **Conflicts**: `git pull` may conflict. User resolves in terminal. Then imports via Sync UI.

### Dual workspace creation

When Office creates a workspace, it writes to both locations:
1. Write `kandev.yml` to filesystem (`~/.kandev/workspaces/<name>/kandev.yml`) -- the source of truth for config.
2. Create a DB row in the existing `workspaces` table -- for kanban board compatibility (tasks, columns, workflows).

Both the kanban board and Office see the same workspace. The filesystem config is authoritative for office entities (agents, skills, projects, routines). The DB workspace row is authoritative for kanban state (task sequence, default executor, workflow ID).

### Agent profiles

Agent profiles (model, CLI flags, MCP servers) stay in the existing kandev `agent_profiles` DB table. Office agent instances reference profiles by ID:

```yaml
# agents/ceo.yml (filesystem export format)
name: CEO
role: ceo
agent_profile_id: "prof_abc123"
desired_skills: [memory, delegation-playbook]
```

Profiles are managed via the existing kandev settings UI (`/settings/agents/`). Future: profiles may move to filesystem too, but not in v1.

### Skill injection

Skills for agent sessions are written into the agent's worktree (CWD) before each session. The skill content can come from:
- **DB** (inline skills created via UI) -- content written to the agent-specific skill path (`<worktree>/.claude/skills/kandev-<slug>/SKILL.md` for Claude, `.agents/skills/kandev-<slug>/` for others)
- **Filesystem** (imported from GitHub/skills.sh) -- content read and written to worktree
- **Bundled** (shipped with kandev binary) -- same CWD injection

See [office-skills](../office-skills/spec.md) for the full injection approach.

### What stays in SQLite

Only runtime/transactional data that cannot live on the filesystem:
- `office_agent_runtime` (status, pause_reason, last_wakeup_finished_at -- survives restarts)
- `office_wakeup_queue` (atomic claims)
- `office_cost_events` (append-only ledger)
- `office_budget_policies` (transactional, references config IDs)
- `office_routine_triggers` + `office_routine_runs` (atomic cron claims, run tracking)
- `office_routines` (FK target for triggers/runs)
- `office_approvals` (transactional)
- `office_activity_log` (append-only)
- `office_channels` (secrets, not exportable)
- `task_blockers`, `task_comments` (transactional)

**Removed from SQLite** (now filesystem-only): `office_agent_instances`, `office_skills`, `office_projects`. These config tables are replaced by YAML files in `~/.kandev/workspaces/<name>/`.

### Export/import bundle

- **Export**: writes DB config entities to `~/.kandev/workspaces/<name>/` as YAML/markdown files. Also available as zip download.
- **Import**: reads YAML files from filesystem, shows diff against DB, applies approved changes.
- **Preview**: shows what will change before applying (created, updated, deleted).

### Future: cloud offering

With all config in the DB, a cloud/SaaS version works naturally:
- Shared PostgreSQL instead of SQLite
- Multiple users see the same agents/skills/projects
- Real-time collaboration
- Filesystem sync becomes optional (for git versioning / local backup)

## Scenarios

- **GIVEN** a user on the settings Sync page, **WHEN** new YAML files exist on disk that aren't in the DB, **THEN** the UI shows them as "incoming changes" with green + indicators. The user clicks "Review & Apply" to import them.

- **GIVEN** a user who created agents via the UI, **WHEN** they click "Export to FS", **THEN** YAML files are written to disk for each agent. The user can then `git add && git commit && git push`.

- **GIVEN** a team member who pulled new config via `git pull`, **WHEN** they open the Sync page, **THEN** the diff shows changes from the repo. They apply them to their DB.

- **GIVEN** a user who accidentally deletes a YAML file on disk, **WHEN** they check the Sync page, **THEN** the outgoing diff shows the entity as "missing on disk". The DB is unaffected. They can re-export.

- **GIVEN** a YAML file with parse errors (bad syntax), **WHEN** the user tries to import, **THEN** the import preview shows the parse error for that file. Other files can still be imported.

## Out of scope

- Automatic filesystem sync (user always controls import/export)
- Real-time collaborative editing of YAML files
- Conflict resolution UI for git merges (user resolves in terminal)
- Plugin system config sync

## Related specs

- [office-skills](../office-skills/spec.md) -- skill registry and CWD injection
- [office-agents](../office-agents/spec.md) -- agent instances reference profiles by ID
- [office-routines](../office-routines/spec.md) -- routine config + operational state
