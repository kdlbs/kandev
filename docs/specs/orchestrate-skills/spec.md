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
- This matches the structure used by Claude Code's native skill discovery, Paperclip's skill system, and other agent CLIs.

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

### Skill injection at session start

- When the orchestrator creates a session for an agent instance, it resolves the instance's `desired_skills` to their on-disk locations.
- For inline skills: the content is materialized to a temporary directory under the workspace's skill cache.
- For local_path skills: the source directory is used directly.
- For git skills: a cached clone is used (refreshed periodically).
- Symlinks are created from the agent's home directory skill location to the resolved skill directories:
  - Claude: `~/.claude/skills/<slug>/` (discovered natively by Claude Code).
  - Other agents: adapter-specific skill directories as supported.
- The agent CLI discovers the skills through its native mechanism -- no system prompt injection is needed for skill content.
- Symlinks are cleaned up when the session ends.

### Skill compatibility

- Not all agent CLIs support skill discovery. The registry tracks which agent types are compatible with each skill.
- For agent types that don't support native skill discovery, the skill's `SKILL.md` content is appended to the agent's system prompt as a fallback.

### UI at `/orchestrate/company/skills`

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
