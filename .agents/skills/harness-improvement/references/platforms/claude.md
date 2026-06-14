# Claude Code Platform Reference

Verified against Claude Code docs on 2026-06-14.

Sources:
- https://code.claude.com/docs/en/skills
- https://code.claude.com/docs/en/sub-agents
- https://code.claude.com/docs/en/memory
- https://code.claude.com/docs/en/commands

## Skills

Claude skills live in:

```text
.claude/skills/<name>/SKILL.md
```

`.claude/commands/<name>.md` still works and creates `/name`, but custom commands have been merged into skills. Prefer skills for new workflow content because they support supporting files.

Common skill frontmatter:

```yaml
---
name: deploy
description: Deploy the application to production.
when_to_use: Use when the user asks to deploy or release.
argument-hint: "[environment]"
arguments: [environment]
disable-model-invocation: true
allowed-tools: Bash Read
model: inherit
effort: medium
context: fork
agent: implementer
paths:
  - "apps/**"
---
```

Notes:

- `description` is recommended and drives automatic loading.
- `disable-model-invocation: true` makes a skill manual-only.
- `allowed-tools` and `disallowed-tools` scope tools while the skill is active.
- `context: fork` runs the skill in a subagent context; `agent` chooses the subagent type.
- Skills can include supporting files and scripts; keep the main body concise.

## Subagents

Claude project subagents live in:

```text
.claude/agents/<name>.md
```

Subagent files are Markdown with YAML frontmatter. Required fields are `name` and `description`.

```yaml
---
name: code-reviewer
description: Reviews code for quality and best practices.
tools: Read, Glob, Grep
model: sonnet
permissionMode: default
maxTurns: 8
skills:
  - code-review
effort: high
isolation: worktree
color: blue
---

You are a code reviewer. Provide specific, actionable feedback.
```

Supported fields include `tools`, `disallowedTools`, `model`, `permissionMode`, `mcpServers`, `hooks`, `maxTurns`, `skills`, `initialPrompt`, `memory`, `effort`, `background`, `isolation`, and `color`.

Important behavior:

- Subagents inherit all tools when `tools` is omitted.
- Use `tools` as an allowlist or `disallowedTools` as a denylist.
- `permissionMode` supports `default`, `acceptEdits`, `auto`, `dontAsk`, `bypassPermissions`, and `plan`.
- Use `isolation: worktree` when the subagent should work in a temporary worktree.
- Use `skills` to preload full skill content into the subagent at startup.
- Subagents loaded from disk need a session restart unless created through `/agents`.
- Do not enable nested agent spawning unless intentional.

## CLAUDE.md

Claude reads `CLAUDE.md`, not `AGENTS.md`. To share repo instructions, use:

```md
@AGENTS.md

## Claude Code

Claude-specific additions here.
```

Keep under roughly 200 lines when possible. Use `.claude/rules/` for modular/path-scoped rules and skills for task-specific workflows.
