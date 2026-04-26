---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Skill Registry & Agent Skills

## Why

Kandev agents today receive the same system prompt regardless of the task. There is no way to give an agent specialized knowledge -- how to review PRs, how to write tests for a specific framework, how to follow a deploy runbook -- without manually editing prompts each time. Different agents working on different types of tasks need different capabilities, and those capabilities should be reusable, version-controlled, and manageable through a UI.

Orchestrate introduces a skill registry where users create, edit, and manage skills as reusable capability directories. Skills are assigned to agent instances and injected into the agent's working environment at session start via symlinks, so the agent CLI discovers them natively.

## What

### Skill structure

- A skill is a directory containing:
  - `SKILL.md` (required): markdown instructions that teach the agent how to perform a specific capability. This is the primary content the agent reads.
  - Optional scripts and reference files: helper scripts, templates, examples, checklists that the agent can invoke or reference during execution.
- This matches the structure used by Claude Code's native skill discovery and other agent CLIs.

### Skill registry

- Skills are managed at the workspace level.
- The registry is a CRUD interface for skill entries:
  - **Create**: user provides a name, description, and the skill content (SKILL.md text + optional files). Skills can also be imported from a local directory path or a git repository URL.
  - **Edit**: update skill content in-place. Changes take effect on the next agent session (existing sessions are unaffected).
  - **Remove**: delete a skill from the registry. Does not affect running sessions that already have the skill symlinked.
  - **List**: browse all skills in the workspace with name, description, and which agents use them.
- Each skill entry stores:
  - `id`: unique identifier.
  - `name`: human-readable label (e.g. "code-review", "test-writer", "deploy-runbook").
  - `slug`: kebab-case identifier used for the symlink directory name.
  - `description`: one-line summary.
  - `source_type`: `inline` (content stored in DB), `local_path` (symlink to a directory on disk), or `git` (cloned from a repository).
  - `source_locator`: path or URL for non-inline sources.
  - `content`: the SKILL.md text (for inline skills) or null (for path/git sources where content lives on disk).
  - `file_inventory`: list of files in the skill directory (name, size) for display purposes.
  - `workspace_id`: scoped to workspace.
  - `created_by_agent_instance_id`: nullable. If set, this skill was created by an agent (see [orchestrate-assistant](../orchestrate-assistant/spec.md) -- agent self-improvement). Agents can only edit skills they created.

### Skill assignment to agents

- Each agent instance has a `desired_skills` list (array of skill IDs).
- Users assign skills to agents via the agent instance config UI or during creation.
- The CEO agent can recommend skills for new hires in its hire requests.
- Skills are additive -- an agent's effective skill set is the union of its assigned skills.

### Skill injection via symlinks

Skills live on disk and are symlinked into each agent CLI's native skill discovery directory. No materialization needed -- the agent reads the skill directly via the symlink.

**Skill source directories:**
- Workspace skills: `~/.kandev/workspaces/<name>/skills/<slug>/`
- Bundled system skills: `~/.kandev/skills/<slug>/`
- In dev mode: `.kandev-dev/` instead of `~/.kandev/`

**Agent skill discovery directories** (all get symlinks):

| Path | Agent CLIs |
|------|-----------|
| `~/.agents/skills/` | Codex, Copilot, Cursor, Augment, OpenCode, Amp |
| `~/.claude/skills/` | Claude Code, Copilot, Augment, OpenCode |
| `~/.codex/skills/` | Codex |
| `~/.gemini/skills/` | Gemini CLI |
| `~/.copilot/skills/` | Copilot CLI |
| `~/.augment/skills/` | Augment |
| `~/.cursor/skills/` | Cursor |
| `~/.config/opencode/skills/` | OpenCode |

When an agent runs, symlinks are created in ALL directories that agent type uses. For example, a Claude agent gets symlinks in both `~/.claude/skills/` and `~/.agents/skills/`.

**Symlink lifecycle:**

- **On kandev startup**: scan all agent skill dirs, create symlinks for all active agents' desired skills, remove dangling symlinks (pointing to nonexistent dirs).
- **Before each session**: ensure symlinks exist for that agent's desired skills + bundled system skills.
- **On skill removed from agent**: check if any other agent instance (across all workspaces) still uses that skill. If none do, remove the symlink from all agent dirs.
- **On kandev shutdown**: remove ALL symlinks that point into our kandev base path (`~/.kandev/` or `.kandev-dev/`). Leave symlinks pointing elsewhere untouched (those belong to the user or other tools).

**Conflict handling:**
- Before creating a symlink, check if the path already exists:
  - Symlink pointing to our kandev dir -> ours, update if needed.
  - Symlink pointing elsewhere -> conflict (user's own skill or another tool). Skip, log warning.
  - Real directory (not a symlink) -> conflict. Skip, log warning.
  - Doesn't exist -> create symlink.
- We only manage symlinks that point into our kandev dirs. Never touch anything else.

**No isolation between concurrent agents:**
- All agents on the same host share the same skill directories. This is an accepted trade-off.
- Extra skills visible to an agent don't cause harm -- agents only use skills referenced in their instructions.
- The union of all active agents' skills is present in the skill dirs at any time.

### Skill compatibility

- Not all agent CLIs support skill discovery. For agent types that don't have a known skill directory, the skill's `SKILL.md` content is appended to the agent's system prompt as a fallback.

### UI at `/orchestrate/workspace/skills`

- Skill list showing name, description, source type, and which agents use each skill.
- Inline editor for creating/editing skill content (SKILL.md with markdown preview).
- Import flow for adding skills from a local path or git URL.
- Assignment panel: select which agent instances receive a skill.

### Skills over MCP tools

- Skills are the preferred pattern for teaching agents orchestrate capabilities. A skill provides instructions (SKILL.md) and the agent calls API endpoints via curl or a lightweight CLI.
- This is cheaper than MCP tools: the agent reads instructions once per session and makes shell calls. MCP tool definitions add per-call overhead (tool schemas in context, structured I/O parsing on every invocation).
- New orchestrate capabilities should follow this pattern: expose API endpoints, write a skill that teaches the agent how to call them, assign the skill to agents that need it.

### System-provided skills

The skill registry ships with built-in skills that can be assigned to any agent:

- **`memory`**: teaches agents to read/write persistent memory entries via the orchestrate API. See [orchestrate-assistant](../orchestrate-assistant/spec.md).
- **`kandev-config-export`**: teaches agents to read orchestrate config from the API and serialize to `.kandev/` format. See [orchestrate-config](../orchestrate-config/spec.md).
- **`kandev-config-import`**: teaches agents to read `.kandev/` files and apply config via the API. See [orchestrate-config](../orchestrate-config/spec.md).

System-provided skills are pre-installed in the registry when Orchestrate is enabled. Users can customize them or create their own.

## Scenarios

- **GIVEN** a user on `/orchestrate/company/skills`, **WHEN** they click "Add Skill" and enter a name, description, and SKILL.md content, **THEN** the skill appears in the registry and is available for assignment to agent instances.

- **GIVEN** a skill assigned to a worker agent instance, **WHEN** the worker starts a new session, **THEN** the skill directory is symlinked into the agent's home (e.g. `~/.claude/skills/code-review/`) and the agent discovers it natively.

- **GIVEN** a skill sourced from a git URL, **WHEN** the user creates the skill entry, **THEN** the repository is cloned and cached. The file inventory is displayed in the UI.

- **GIVEN** a running session with symlinked skills, **WHEN** the user edits the skill in the registry, **THEN** the running session is unaffected. The next session for that agent picks up the updated content.

- **GIVEN** an agent instance with three assigned skills, **WHEN** the user removes one skill from the instance's config, **THEN** the next session only symlinks the remaining two skills.

## Out of scope

- Skill marketplace or cross-workspace skill sharing.
- Skill versioning (git-sourced skills can use branches/tags, but the registry does not track versions internally).
- Skill-level permissions (all skills are available to all agents in the workspace; assignment is the access control).
- Automatic skill recommendation based on task content.
