# OpenCode Platform Reference

Verified against OpenCode docs on 2026-06-14.

Sources:
- https://opencode.ai/docs/skills/
- https://opencode.ai/docs/agents/
- https://opencode.ai/docs/rules/

## Skills

OpenCode discovers skills from:

```text
.config/opencode/skills/<name>/SKILL.md
.claude/skills/<name>/SKILL.md
.agents/skills/<name>/SKILL.md
```

Recognized frontmatter:

```yaml
---
name: skill-name
description: Clear trigger and scope.
license: MIT
compatibility: opencode
metadata:
  owner: kandev
---
```

Only `name` and `description` are required. Unknown frontmatter fields are ignored. `name` must match the directory name and match:

```text
^[a-z0-9]+(-[a-z0-9]+)*$
```

Description must be 1-1024 characters.

## Agents

OpenCode supports JSON config and Markdown agent files.

Project Markdown agents:

```text
.opencode/agents/<name>.md
```

Markdown example:

```yaml
---
description: Reviews code for quality and best practices.
mode: subagent
model: anthropic/claude-sonnet-4-20250514
temperature: 0.1
permission:
  edit: deny
  bash:
    "*": ask
    "git diff": allow
    "git log*": allow
---

Only analyze code and suggest changes.
```

The Markdown filename becomes the agent name.

JSON example:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "agent": {
    "code-reviewer": {
      "description": "Reviews code for best practices and potential issues",
      "mode": "subagent",
      "model": "anthropic/claude-sonnet-4-20250514",
      "prompt": "You are a code reviewer. Focus on security, performance, and maintainability.",
      "permission": {
        "edit": "deny"
      }
    }
  }
}
```

Important fields:

- `description` is required.
- `mode`: `primary`, `subagent`, or `all`; default is `all`.
- `model`: provider/model-id format.
- `temperature`: lower for planning/review; higher for brainstorming.
- `steps`: max agentic iterations; legacy `maxSteps` is deprecated.
- `permission`: use this instead of deprecated `tools`.
- `hidden`: hide a subagent from autocomplete.
- `permission.task`: restrict which subagents another agent can invoke.

## Rules

OpenCode uses `AGENTS.md` for project instructions. It also has Claude-compatible fallbacks:

- Project rules: `CLAUDE.md` if no `AGENTS.md` exists.
- Project skills: `.claude/skills/` and `.agents/skills/` as compatibility paths.

Prefer `AGENTS.md` as the shared project source of truth.
