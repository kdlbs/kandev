# OpenCode Platform Reference

Verified against OpenCode docs on 2026-06-14.

Sources:
- https://opencode.ai/docs/skills/
- https://opencode.ai/docs/agents/
- https://opencode.ai/docs/rules/

## Skills

OpenCode discovers skills from:

```text
.opencode/skills/<name>/SKILL.md
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

Launch project workers with OpenCode's native `Task` tool. Kandev MCP task or
session APIs must not be used as an OpenCode subagent fallback.

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
- `model`: provider/model-id format when explicitly pinned. Kandev's OpenCode
  mirrors omit it so the configured provider and primary model are inherited.
- `temperature`: lower for planning/review; higher for brainstorming.
- `steps`: max agentic iterations; legacy `maxSteps` is deprecated.
- `permission`: use this instead of deprecated `tools`.
- `hidden`: hide a subagent from autocomplete.
- `permission.task`: restrict which subagents another agent can invoke.

Kandev intentionally omits `model` from its OpenCode subagent mirrors so users
can keep their configured provider. Because OpenCode then inherits the primary
agent's model, this is the one platform mirror that does not guarantee worker
cost separation. Every OpenCode worker sets `permission.task: deny` to prevent
recursive delegation.

OpenCode `temperature` controls sampling randomness, not reasoning effort. With
`model` unpinned, provider-specific reasoning settings also remain inherited
from the primary agent. Do not map Kandev role effort to `temperature`.

## Rules

OpenCode uses `AGENTS.md` for project instructions. It also has Claude-compatible fallbacks:

- Project rules: `CLAUDE.md` if no `AGENTS.md` exists.
- Project skills: `.claude/skills/` and `.agents/skills/` as compatibility paths.

Prefer `AGENTS.md` as the shared project source of truth.
