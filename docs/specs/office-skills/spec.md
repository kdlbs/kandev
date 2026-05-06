---
status: draft
created: 2026-04-25
owner: cfl
---

# Office: Skill Registry & Agent Skills

## Why

Kandev agents today receive the same system prompt regardless of the task. There is no way to give an agent specialized knowledge -- how to review PRs, how to write tests for a specific framework, how to follow a deploy runbook -- without manually editing prompts each time. Different agents working on different types of tasks need different capabilities, and those capabilities should be reusable, version-controlled, and manageable through a UI.

Office introduces a skill registry where users create, edit, and manage skills as reusable capability directories. Skills are assigned to agent instances and injected into the agent's working environment (CWD) at session start, so the agent CLI discovers them natively.

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
  - **Remove**: delete a skill from the registry. Does not affect running sessions (the skill file is already written to their worktree).
  - **List**: browse all skills in the workspace with name, description, and which agents use them.
- Each skill entry stores:
  - `id`: unique identifier.
  - `name`: human-readable label (e.g. "code-review", "test-writer", "deploy-runbook").
  - `slug`: kebab-case identifier used as the injected directory name (`kandev-<slug>`).
  - `description`: one-line summary.
  - `source_type`: `inline` (content stored in DB), `local_path` (symlink to a directory on disk), or `git` (cloned from a repository).
  - `source_locator`: path or URL for non-inline sources.
  - `content`: the SKILL.md text (for inline skills) or null (for path/git sources where content lives on disk).
  - `file_inventory`: list of files in the skill directory (name, size) for display purposes.
  - `workspace_id`: scoped to workspace.
  - `created_by_agent_instance_id`: nullable. If set, this skill was created by an agent (see [office-assistant](../office-assistant/spec.md) -- agent self-improvement). Agents can only edit skills they created.

### Skill assignment to agents

- Each agent instance has a `desired_skills` list (array of skill IDs).
- Users assign skills to agents via the agent instance config UI or during creation.
- The CEO agent can recommend skills for new hires in its hire requests.
- Skills are additive -- an agent's effective skill set is the union of its assigned skills.

### Skill injection via CWD

Skill content is stored in the **database** (source of truth). For agent session injection, skills are written directly into the agent's working directory (the session worktree) before the session starts.

**Injection path:**

Each agent type defines a `ProjectSkillDir` in its `RuntimeConfig` — the CWD-relative path where skills are written. Skills are written to `<worktree>/<ProjectSkillDir>/kandev-<slug>/SKILL.md`.

Current values:

| Agent type | `ProjectSkillDir` |
|-----------|------------------|
| `claude-acp` (Claude Code) | `.claude/skills` |
| `codex-acp`, `opencode-acp`, `gemini`, `copilot-acp`, `auggie`, `amp-acp` | `.agents/skills` |

Default (if `ProjectSkillDir` is not set): `.agents/skills`.

- Claude Code reads project skills from `.claude/skills/`, not `.agents/skills/`.
- All other supported CLIs read from `.agents/skills/`.
- `kandev-` prefix distinguishes injected skills from team-committed skills already present in the repo.
- Each agent session has its own worktree (CWD), so skill sets are isolated per agent -- no cross-agent pollution.

**Exclusion from git:**

Before writing skills, the backend adds `kandev-*` patterns to the worktree's `.git/info/exclude` file so injected skill directories never appear as dirty files in git status:
```
.claude/skills/kandev-*
.agents/skills/kandev-*
```

**Skill lifecycle:**

- **Before session start**: write each desired skill's `SKILL.md` to the appropriate path for the agent type. Before writing skills, all existing `kandev-*` directories in the target skill path are deleted (clean-slate). This ensures removed skills don't linger and updated skills get fresh content. Ensure `.git/info/exclude` has the `kandev-*` patterns.
- **No explicit cleanup needed**: injected skills live inside the worktree directory. When the worktree is deleted at session end, all injected skills are removed automatically.
- **On skill update**: changes take effect on the next session. Running sessions are unaffected (the file is already written).

**Per-agent isolation:**
- Because each agent session gets its own worktree (CWD), skill directories are fully isolated between concurrent agents.
- No shared HOME directories, no symlink management, no shutdown cleanup hooks.

### Skill compatibility

- Not all agent CLIs support skill discovery. For agent types that don't have a known skill directory, the skill's `SKILL.md` content is appended to the agent's system prompt as a fallback.

### UI at `/office/workspace/skills`

- Skill list showing name, description, source type, and which agents use each skill.
- Inline editor for creating/editing skill content (SKILL.md with markdown preview).
- Import flow for adding skills from a local path or git URL.
- Assignment panel: select which agent instances receive a skill.

### Skills over MCP tools

- Skills are the preferred pattern for teaching agents office capabilities. A skill provides instructions (SKILL.md) and the agent calls API endpoints via curl or a lightweight CLI.
- This is cheaper than MCP tools: the agent reads instructions once per session and makes shell calls. MCP tool definitions add per-call overhead (tool schemas in context, structured I/O parsing on every invocation).
- New office capabilities should follow this pattern: expose API endpoints, write a skill that teaches the agent how to call them, assign the skill to agents that need it.

### System-provided skills

The skill registry ships with built-in skills bundled in the kandev binary (`apps/backend/internal/office/configloader/skills/<slug>/SKILL.md`, `//go:embed skills/*`). On every backend start the office service walks the embedded set and upserts a row per workspace with `is_system = true`, preserving per-agent `desired_skills` references across content updates. Removed slugs are deleted in place.

System skills are **read-only** in the UI: the Skills page hides edit/delete affordances and shows a "System" badge alongside the kandev release version that delivered the current content (`system_version`).

Each system SKILL.md carries an optional `kandev:` frontmatter block:

```yaml
---
name: kandev-hiring
description: …
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---
```

`default_for_roles` drives auto-attach on agent create: when a new agent is created with role `R`, the office service appends every system skill whose `default_for_roles` contains `R` to that agent's `desired_skills` (only when the caller didn't pass an explicit list — explicit requests are respected).

#### v1 system-skill set

| Slug | Default for | Purpose |
|---|---|---|
| `kandev-protocol` | every role | Core control-plane: heartbeat model, env vars, where to find context |
| `memory` | every role | Read/write persistent memory entries via `agentctl kandev memory …` |
| `kandev-escalation` | worker, specialist, assistant, reviewer | When + how to flag work as blocked |
| `kandev-team` | ceo | List/inspect agents (`agentctl kandev agents list`) |
| `kandev-hiring` | ceo | Hire new agents with approval gating (`agentctl kandev agents create`) |
| `kandev-agent-edit` | ceo | Update or retire existing agents |
| `kandev-tasks` | ceo, worker, specialist | List, move, archive, message tasks |
| `kandev-task-comment` | every role | Post a comment on a task without spawning a new one |
| `kandev-routines` | ceo | Schedule recurring work via cron triggers |
| `kandev-approvals` | ceo | Decide pending approvals (`approvals list / decide`) |
| `kandev-budget` | ceo | Check workspace + per-agent spend |
| `kandev-config-export` / `kandev-config-import` | ceo | Round-trip workspace config with the `.kandev/` filesystem |

Users may still untick a default-attached system skill on any individual agent — the role default is a soft suggestion, not a hard restriction.

#### Upgrade sync

Each kandev release that ships modified or removed SKILL.md files takes effect silently on the next backend start. The startup log line `system skills synced workspaces=N inserted=[…] updated=[…] removed=[…]` makes the change auditable. There is no banner — system skills are kandev-owned, like a built-in CLI man page.

## Scenarios

- **GIVEN** a user on `/office/company/skills`, **WHEN** they click "Add Skill" and enter a name, description, and SKILL.md content, **THEN** the skill appears in the registry and is available for assignment to agent instances.

- **GIVEN** a skill assigned to a Claude Code worker agent, **WHEN** the worker starts a new session, **THEN** the skill's `SKILL.md` is written to `<worktree>/.claude/skills/kandev-code-review/SKILL.md` (Claude's `ProjectSkillDir` is `.claude/skills`). For non-Claude agents, the path is `<worktree>/.agents/skills/kandev-code-review/SKILL.md`.

- **GIVEN** a skill sourced from a git URL, **WHEN** the user creates the skill entry, **THEN** the repository is cloned and cached. The file inventory is displayed in the UI.

- **GIVEN** a running session with injected skills, **WHEN** the user edits the skill in the registry, **THEN** the running session is unaffected (the file is already written). The next session for that agent picks up the updated content.

- **GIVEN** an agent instance with three assigned skills, **WHEN** the user removes one skill from the instance's config, **THEN** the next session only writes the remaining two skills to the worktree.

## Out of scope

- Skill marketplace or cross-workspace skill sharing.
- Skill versioning (git-sourced skills can use branches/tags, but the registry does not track versions internally).
- Skill-level permissions (all skills are available to all agents in the workspace; assignment is the access control).
- Automatic skill recommendation based on task content.
