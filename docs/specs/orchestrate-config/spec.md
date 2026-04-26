---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Configuration Portability & Repository Sync

## Why

Teams need to version-control their agent configurations, skills, and routines the same way they version-control code -- with PRs, reviews, and history. Solo users need to export and back up their setup. Users with multiple projects need project-specific skills alongside shared workspace config. Today, all orchestrate configuration lives in the database with no way to share it, review changes, or restore a previous state.

Orchestrate needs a portable configuration format that lives in a `.kandev/` directory, and a task-based sync mechanism where agents handle the import/export work -- no special sync engine required.

## What

### Portable configuration format

- Orchestrate configuration is represented as files in a `.kandev/` directory.
- Directory structure:

```
.kandev/
├── kandev.yml                    # Workspace settings
├── agents/
│   ├── ceo.yml                   # Agent instance definitions
│   ├── frontend-worker.yml
│   └── qa-bot.yml
├── skills/
│   ├── code-review/
│   │   └── SKILL.md              # Skill directories (native format)
│   ├── memory/
│   │   └── SKILL.md
│   └── deploy-runbook/
│       ├── SKILL.md
│       └── scripts/
│           └── deploy.sh
├── routines/
│   ├── daily-digest.yml
│   └── weekly-security-scan.yml
└── projects/
    ├── api-v2-migration.yml
    └── q2-security.yml
```

- **`kandev.yml`**: workspace-level settings (approval defaults, budget defaults, default agent profile, default executor config).
- **`agents/*.yml`**: one file per agent instance. Contains name, role, hierarchy (`reports_to` by name), permissions, budget, `desired_skills` (by slug), agent profile reference (by agent_name + model + mode signature), optional `executor_preference` (type, image, resource_limits).
- **`skills/*/`**: one directory per skill. The directory IS the skill content -- `SKILL.md` plus optional scripts and reference files. Same format the skill registry uses.
- **`routines/*.yml`**: one file per routine. Contains name, description, task template, trigger config, concurrency policy, assignee (by agent name).
- **`projects/*.yml`**: one file per project. Contains name, description, status, repositories, executor config (type, image, resource_limits, worktree_strategy, network_policy), lead agent (by name).

### What is NOT portable

These stay in the database only -- they are runtime/transactional state, not configuration:
- Agent memory entries
- Cost events and budget spend counters
- Approvals and activity log
- Wakeup queue
- Channel configs with secrets (bot tokens, signing keys)
- Task and session data

### Three usage patterns

**1. DB-only (default)** -- solo users, simple setups.
- All config in SQLite. No repo involved.
- Export button in the UI downloads a `.kandev/` bundle as a zip/tarball.
- Import button uploads a bundle and applies it to the workspace.
- Good for backup, migration between machines, or sharing a setup with someone.

**2. Repo-backed** -- teams with version control.
- Team maintains a dedicated config repo (or a directory in an existing repo) containing `.kandev/`.
- The workspace is linked to this repo via a "config source" setting (repo URL + branch + path).
- Sync is task-based (see below) -- agents read from the repo and apply changes, or export changes and open PRs.
- Team members review config changes as PRs like any other code change.
- Each team member's kandev instance syncs from the same repo. Runtime state (memory, costs) stays local.

**3. Embedded in project repo** -- project-specific config.
- `.kandev/` lives inside a project repository (e.g. `myapp/.kandev/`).
- Good for project-specific skills (build scripts, deploy runbooks, testing conventions).
- Can be combined with a workspace-level config repo (multi-source merge).

### Multi-source config merge

A workspace can have multiple config sources with priority ordering:
1. **Workspace defaults** (DB) -- base configuration.
2. **Workspace config repo** (`.kandev/`) -- shared agents, skills, routines.
3. **Project repo** (`.kandev/`) -- project-specific skills and overrides.

Later sources override earlier ones. An agent defined in the workspace config repo can have its `desired_skills` extended by a project repo's skills. A skill in a project repo with the same slug as a workspace skill overrides it for tasks in that project.

### Task-based sync (no sync engine)

Config sync uses the same orchestrate primitives as everything else -- tasks, agents, and skills. No special bidirectional sync machinery.

#### DB -> Repo (exporting changes)

1. User changes agent config in the UI (e.g. updates budget, adds a skill).
2. System creates a task: "Sync kandev config: updated frontend-worker budget".
3. Task is assigned to a **config-sync agent** (an agent instance with the `kandev-config-export` skill).
4. The agent reads current config from the orchestrate API, serializes to YAML, writes to `.kandev/` in the config repo, and opens a PR.
5. The PR is reviewed and merged by the team like any other change.

#### Repo -> DB (importing changes)

1. A PR is merged to the config repo that changes `.kandev/agents/qa-bot.yml`.
2. A webhook fires (or a routine polls the repo for changes).
3. System creates a task: "Apply kandev config changes from repo".
4. Task is assigned to the config-sync agent (with the `kandev-config-import` skill).
5. The agent reads the `.kandev/` directory, diffs against current DB state, and calls the orchestrate API to apply changes.
6. Changes are reflected in the UI. An activity log entry records what changed and from which commit.

#### Config-sync agent and skills

- The **config-sync agent** is a system agent instance (`role=worker`) with two skills:
  - `kandev-config-export`: instructions for reading orchestrate API endpoints, serializing to the `.kandev/` YAML format, and opening PRs.
  - `kandev-config-import`: instructions for reading `.kandev/` files, parsing YAML, diffing against current state, and calling orchestrate API to apply.
- These skills teach the agent the API contract and file format via SKILL.md -- the agent uses curl/CLI, not MCP tools.
- The config-sync agent is auto-created when a config source repo is linked. It can also be manually created for export-only workflows.

#### Cross-references via name, not ID

- Agent instances reference each other by name (`reports_to: ceo`), not by database ID.
- Skills are referenced by slug (`desired_skills: [code-review, memory]`).
- Agent profiles are matched by signature (`agent_name: claude, model: claude-sonnet-4-6`).
- This enables config files to be portable across workspaces where database IDs differ.

### Export/import bundle

- For users who don't want repo-backed sync, a manual export/import is available.
- **Export**: downloads the full `.kandev/` directory as a zip archive. Accessible from `/orchestrate/company/settings`.
- **Import**: uploads a zip archive and applies the config to the workspace. Deduplication by name -- existing entities with the same name are updated, new ones are created, missing ones are optionally deleted.
- Import preview: before applying, the UI shows a diff of what will change (created, updated, deleted).

### UI

- `/orchestrate/company/settings` gains:
  - **Config source**: link a repository URL + branch + path for repo-backed sync. Test connection button.
  - **Export**: download current config as a `.kandev/` zip bundle.
  - **Import**: upload a bundle with preview diff before applying.
  - **Sync status**: last sync timestamp, last commit applied, pending changes indicator.
  - **Sync now**: manually trigger a repo -> DB sync task.

## Scenarios

- **GIVEN** a solo user with DB-only config, **WHEN** they click "Export" in settings, **THEN** a zip file is downloaded containing the `.kandev/` directory with all agents, skills, routines, and projects as YAML/markdown files.

- **GIVEN** a team with a linked config repo, **WHEN** a user changes the CEO's budget from $50 to $100 in the UI, **THEN** a task is created for the config-sync agent. The agent updates `.kandev/agents/ceo.yml`, commits, and opens a PR titled "Update CEO budget to $100". The team reviews and merges.

- **GIVEN** a config repo where a team member adds a new file `.kandev/agents/security-bot.yml` via PR, **WHEN** the PR is merged and the webhook fires, **THEN** a task is created for the config-sync agent. The agent reads the new file and creates a new agent instance in the workspace. An activity log entry records the import.

- **GIVEN** a project repo with `.kandev/skills/project-build/SKILL.md` and a workspace config repo with shared agents, **WHEN** a task runs on that project, **THEN** the agent sees both the workspace-level skills and the project-specific `project-build` skill. The project skill is available only for tasks in that project's context.

- **GIVEN** a user importing a bundle that contains an agent "Frontend Worker" that already exists, **WHEN** the import preview is shown, **THEN** the UI displays a diff showing which fields will be updated (e.g. budget changed, new skill added). The user confirms before applying.

- **GIVEN** a config-sync agent assigned to export changes, **WHEN** the agent fails to push (e.g. auth error), **THEN** the task fails, an error inbox item is created, and the activity log records the failure.

### Future: template marketplace

Config bundles are the foundation for a template marketplace. A template is a curated `.kandev/` bundle containing agents, skills, routines, and workspace settings for a specific workflow. Examples:

- **Developer Team**: CEO, CTO, Architect, Frontend/Backend Workers, QA, SRE. Skills: code-review, test-writer, deploy-runbook.
- **Solo Developer**: CTO, 2 Workers, QA. Lightweight setup without CEO overhead.
- **Business Analyst**: CEO, Project Manager, Data Analyst, Report Writer. Skills: spreadsheet-ops, data-viz.
- **Marketing Team**: CEO, Marketing Lead, Content Writer, Social agents. Skills: content-calendar, social-posting.
- **DevOps / SRE**: CTO, Infra Worker, Monitoring Agent, Incident Responder. Skills: deploy-runbook, monitoring-check.

"Install a template" = import a config bundle via the existing import flow. The marketplace is a registry/discovery layer on top of the export/import mechanism -- no new primitives required. Not in scope for the initial implementation.

## Out of scope

- Template marketplace (future layer on top of export/import).
- Real-time file watching for local `.kandev/` directories (sync is task-based, not filesystem-event-based).
- Conflict resolution UI (repo is source of truth in repo-backed mode; UI changes create PRs, not direct writes).
- Secret management in config files (channel tokens, API keys stay in DB, never exported).
- Config schema versioning or migration tooling (format is simple YAML, backwards-compatible changes only).
- Multi-workspace sync from a single config repo (one workspace per config source).

## Related specs

- [orchestrate-skills](../orchestrate-skills/spec.md) -- skills are directories, natural fit for `.kandev/skills/`
- [orchestrate-agents](../orchestrate-agents/spec.md) -- agent instance definitions exported as YAML
- [orchestrate-routines](../orchestrate-routines/spec.md) -- routine definitions exported as YAML
- [orchestrate-projects](../orchestrate-projects/spec.md) -- project definitions exported as YAML
- [orchestrate-scheduler](../orchestrate-scheduler/spec.md) -- webhook triggers for repo change detection
