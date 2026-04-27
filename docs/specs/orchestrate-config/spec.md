---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Configuration Storage & Sync

## Why

Configuration (agents, skills, projects, routines, workspace settings) needs to be both reliably stored AND portable. Teams need to version-control agent configs with PRs, share skills across workspaces, and back up their setup. But the database must remain the source of truth for operational reliability -- accidental file deletions, parse errors, and partial writes should never break a running system.

Orchestrate uses a **DB-first model with filesystem sync**: the database is the source of truth for all config. The filesystem (`~/.kandev/`) is a portable sync target for git versioning, sharing, and backup. Users control when changes flow in either direction via a Sync UI.

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
- **Agent profiles**: stay in existing kandev DB tables, referenced by ID from orchestrate agents.

### Filesystem directory structure

```
~/.kandev/
├── config.yml                      # global kandev settings
└── workspaces/
    ├── default/
    │   ├── kandev.yml              # workspace settings
    │   ├── agents/
    │   │   ├── ceo.yml
    │   │   └── frontend-worker.yml
    │   ├── skills/
    │   │   ├── code-review/
    │   │   │   └── SKILL.md
    │   │   └── deploy-runbook/
    │   │       ├── SKILL.md
    │   │       └── scripts/
    │   │           └── deploy.sh
    │   ├── routines/
    │   │   └── daily-digest.yml
    │   └── projects/
    │       └── api-migration.yml
    │
    └── my-team/                    # repo-backed workspace
        ├── .git/
        ├── kandev.yml
        ├── agents/
        ├── skills/
        └── ...
```

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

When Orchestrate creates a workspace:
1. Create DB row in existing `workspaces` table (for kanban compatibility)
2. Optionally write `kandev.yml` to filesystem (if user wants git sync)

Both the kanban board and Orchestrate see the same workspace.

### Agent profiles

Agent profiles (model, CLI flags, MCP servers) stay in the existing kandev `agent_profiles` DB table. Orchestrate agent instances reference profiles by ID:

```yaml
# agents/ceo.yml (filesystem export format)
name: CEO
role: ceo
agent_profile_id: "prof_abc123"
desired_skills: [memory, delegation-playbook]
```

Profiles are managed via the existing kandev settings UI (`/settings/agents/`). Future: profiles may move to filesystem too, but not in v1.

### Skill injection

Skills for agent sessions are symlinked from the filesystem skill directories into agent CLI discovery paths. The skill content can come from:
- **DB** (inline skills created via UI) -- exported to disk for symlinking
- **Filesystem** (imported from GitHub/skills.sh) -- already on disk
- **Bundled** (`~/.kandev/skills/`) -- shipped with kandev binary

Before each agent session, the scheduler ensures symlinks are current. See [orchestrate-skills](../orchestrate-skills/spec.md) for the full symlink lifecycle.

### What stays in SQLite

Everything. Config entities AND runtime state:
- Agent instances, skills, projects, routines, workspace settings (config)
- Agent runtime state (status, pause_reason -- survives restarts)
- Wakeup queue (atomic claims)
- Cost events (append-only ledger)
- Budget policies (transactional)
- Routine triggers + runs (atomic cron claims)
- Approvals (transactional)
- Activity log (append-only)
- Channels (secrets)
- Task blockers, comments (transactional)

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

- [orchestrate-skills](../orchestrate-skills/spec.md) -- skill symlinking from disk
- [orchestrate-agents](../orchestrate-agents/spec.md) -- agent instances reference profiles by ID
- [orchestrate-routines](../orchestrate-routines/spec.md) -- routine config + operational state
