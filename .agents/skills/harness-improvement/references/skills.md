# Skills

Use this when creating or updating task-specific instructions loaded on demand.

## Local Kandev Convention

Kandev project skills live in:

```text
.agents/skills/<skill-name>/SKILL.md
.agents/skills/<skill-name>/references/*.md
.agents/skills/<skill-name>/scripts/*
.agents/skills/<skill-name>/assets/*
```

Skill names should be lowercase alphanumeric with single hyphen separators. Keep names under 64 characters.

Required frontmatter:

```yaml
---
name: skill-name
description: Clear trigger, scope, and exclusions for when to use this skill.
---
```

Keep the description specific. It is the primary routing surface for automatic skill selection.

## When To Create A Skill

Create or update a skill when:

- The behavior is a repeatable workflow.
- It should load only for certain tasks.
- The user would otherwise paste the same checklist repeatedly.
- The guidance has optional references, scripts, or templates.

Do not create a skill for:

- Always-on repo constraints. Use `AGENTS.md`.
- A one-off prompt.
- Behavior better encoded in a deterministic script.
- A role requiring a distinct model/tool surface. Use an agent/subagent.

## Body Structure

Prefer:

1. Purpose and trigger.
2. Decision tree or workflow.
3. Required commands/fallbacks.
4. Stop conditions.
5. Validation and final report.
6. Links to references.

Move long platform details and examples to `references/`. Do not add README files inside skill folders.

## Cross-Tool Notes

- Codex and OpenCode recognize `.agents/skills/<name>/SKILL.md`.
- Claude project skills normally live in `.claude/skills/<name>/SKILL.md`; keep `.agents/skills` as source of truth in this repo unless the user asks for Claude-native mirrors.
- Cursor supports Agent Skills and `.cursor/skills/`; it can also migrate dynamic rules/commands to skills. If targeting Cursor specifically, read `platforms/cursor.md`.
- For manual-only workflows, some platforms support skill frontmatter to disable automatic invocation. Use only in platform-specific mirrors unless the local runtime supports that field.

## Validation

Run:

```bash
git diff --check -- .agents/skills/<skill-name>
rg -n "<old-name>|<stale-reference>" .agents/skills .agents/agents AGENTS.md CLAUDE.md
```
