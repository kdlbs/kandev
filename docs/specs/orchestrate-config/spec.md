---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Filesystem-First Configuration

## Why

Configuration (agents, skills, projects, routines, workspace settings) should be editable, versionable, and shareable like code. Storing config in SQLite makes it opaque -- users can't `vim` an agent definition, teams can't review changes in PRs, and there's no version history. The database is the wrong place for config that humans need to read and modify.

Orchestrate uses a **filesystem-first** model: `~/.kandev/workspaces/<name>/` is the source of truth for all configuration. The database stores only runtime/transactional state (tasks, sessions, cost events, wakeup queue). API requests are served from an in-memory cache of the filesystem.

## What

### Directory structure

```
~/.kandev/
├── config.yml                      # global kandev settings (ports, auth)
└── workspaces/
    ├── default/                    # local workspace
    │   ├── kandev.yml              # workspace settings (approvals, defaults, executor config)
    │   ├── agents/
    │   │   ├── ceo.yml             # agent config
    │   │   ├── ceo/
    │   │   │   └── memory/         # agent memory (one file per entry)
    │   │   │       ├── operating/
    │   │   │       │   └── communication-style.md
    │   │   │       └── knowledge/
    │   │   │           └── people-cfl.md
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
    └── my-team/                    # repo-backed workspace (git clone)
        ├── .git/
        ├── kandev.yml
        ├── agents/
        ├── skills/
        └── ...
```

### What lives on filesystem (config)

- **`kandev.yml`**: workspace settings -- approval defaults, budget defaults, default agent profile, default executor config, task prefix.
- **`agents/*.yml`**: one file per agent instance -- name, role, reports_to (by name), permissions, budget, desired_skills (by slug), agent profile (by signature), executor preference, max concurrent sessions.
- **`agents/<name>/memory/`**: agent memory as one markdown file per entry, organized by layer (operating/knowledge/sessions). Shareable via git between team members. See [orchestrate-assistant](../orchestrate-assistant/spec.md).
- **`skills/*/`**: one directory per skill -- `SKILL.md` plus optional scripts and reference files. The directory IS the skill. Agents access skills via symlinks directly from this directory (no materialization needed).
- **`routines/*.yml`**: one file per routine -- name, description, task template, trigger config, concurrency policy, assignee (by agent name).
- **`projects/*.yml`**: one file per project -- name, description, status, repositories, executor config, lead agent (by name).

### What stays in SQLite (runtime state)

- Wakeup queue (atomic claims)
- Cost events (append-only ledger)
- Activity log (append-only)
- Tasks, sessions, messages, turns (existing kandev data)
- Budget spend counters
- Approvals (transactional state)
- Channel configs with secrets (not exportable)

### Data flow

**On startup:**
1. Scan `~/.kandev/workspaces/*/` for workspace directories.
2. For each workspace: read `kandev.yml`, parse agents/skills/routines/projects from YAML files and skill directories.
3. Build in-memory cache: `map[workspaceID]*WorkspaceConfig`.
4. Serve all config API requests from memory cache.

**On UI write (e.g. user creates an agent):**
1. Write YAML file to `~/.kandev/workspaces/<name>/agents/<agent-name>.yml`.
2. Invalidate memory cache for that workspace.
3. Return success to the UI.

**On external file change (user edits YAML, git pull):**
1. `fsnotify` detects change in workspace directory.
2. Invalidate memory cache for that workspace.
3. Re-read affected files.
4. If parse error (git conflict markers, invalid YAML): mark workspace as `config_error`, surface error in UI, keep serving stale cache until fixed.

### Skill symlinks

Skills are directories on disk. When an agent session starts:
- Symlink directly from `~/.kandev/workspaces/<name>/skills/<slug>/` into the agent's home dir (e.g. `~/.claude/skills/<slug>/`).
- No materialization step needed -- the skill files are already on disk.
- Multi-file skills (SKILL.md + scripts + references) work naturally.
- Cleanup: remove symlinks after session ends.

### Repo-backed workspaces

A workspace directory can be a git repository:
- **Setup**: `git clone <repo-url> ~/.kandev/workspaces/<name>/`
- **Pull changes**: `git pull` in workspace dir. `fsnotify` fires, config re-reads.
- **Push UI changes**: write file -> `git add` -> `git commit` -> `git push` (or create task for config-sync agent to open a PR).
- **Merge conflicts**: after a pull with conflicts, YAML parsing fails -> workspace marked as `config_error` -> error banner in UI with file path and error details -> user resolves conflicts manually (or in terminal) -> fsnotify detects fix -> error clears.

### Config error handling

When a workspace has parse errors:
- The config loader marks the workspace as `status=config_error` with the error message and file path.
- API requests for that workspace return stale cached data (last known good state).
- Dashboard shows a banner: "Configuration error in [workspace]: [error]".
- Inbox gets a `config_error` item with details.
- The workspace remains functional (tasks keep running, scheduler works) -- only config mutations are blocked until the error is resolved.
- Once the user fixes the file, `fsnotify` triggers a re-read and the error clears automatically.

### Cross-references via name, not ID

- Agent instances reference each other by name (`reports_to: ceo`), not by database ID.
- Skills are referenced by slug (`desired_skills: [code-review, memory]`).
- Agent profiles are matched by signature (`agent_name: claude, model: claude-sonnet-4-6`).
- IDs are generated (UUIDs) and stored in the YAML `id` field for internal use, but never used for cross-referencing.

### Export/import bundle

- **Export**: downloads the workspace directory as a zip archive. Same files as `~/.kandev/workspaces/<name>/`.
- **Import**: uploads a zip archive, extracts to `~/.kandev/workspaces/<name>/`, triggers re-read.
- Import preview: shows diff of what will change before extracting.

### Future: template marketplace

Config bundles are the foundation for a template marketplace. "Install a template" = extract a zip bundle to `~/.kandev/workspaces/<name>/`. No new primitives required.

## Scenarios

- **GIVEN** a fresh kandev install, **WHEN** the user starts kandev, **THEN** `~/.kandev/workspaces/default/` is created with a `kandev.yml` containing default settings.

- **GIVEN** a workspace with agents configured, **WHEN** the user opens a terminal and edits `~/.kandev/workspaces/default/agents/ceo.yml`, **THEN** `fsnotify` detects the change, the cache is invalidated, and the UI reflects the updated config within seconds.

- **GIVEN** a team with a repo-backed workspace, **WHEN** a team member pushes a new agent file to the repo and another member runs `git pull`, **THEN** the new agent appears in the second member's kandev UI automatically.

- **GIVEN** a git pull that introduces merge conflict markers in `agents/ceo.yml`, **WHEN** fsnotify triggers a re-read, **THEN** the workspace shows a config_error banner with the file path and error message. The agents list shows stale (last good) data. When the user resolves the conflict, the error clears.

- **GIVEN** a user clicking "Export" in settings, **WHEN** the zip is downloaded, **THEN** it contains the exact files from `~/.kandev/workspaces/<name>/` -- YAML files and skill directories.

- **GIVEN** a skill directory at `~/.kandev/workspaces/default/skills/code-review/`, **WHEN** an agent session starts with `code-review` in its desired_skills, **THEN** a symlink is created from `~/.claude/skills/code-review/` pointing to the skill directory. No DB-to-disk copy needed.

## Out of scope

- Automatic conflict resolution for git merges (user resolves manually).
- Secret management in config files (channel tokens, API keys stay in DB).
- Config schema versioning or migration tooling.
- Multi-workspace sync from a single config repo.
- Real-time collaborative editing (one writer at a time per file).

## Related specs

- [orchestrate-skills](../orchestrate-skills/spec.md) -- skills are directories on disk
- [orchestrate-agents](../orchestrate-agents/spec.md) -- agent YAML files
- [orchestrate-routines](../orchestrate-routines/spec.md) -- routine YAML files
- [orchestrate-projects](../orchestrate-projects/spec.md) -- project YAML files
- [orchestrate-scheduler](../orchestrate-scheduler/spec.md) -- wakeup queue stays in DB
