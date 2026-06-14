# Cursor Platform Reference

Verified against Cursor docs on 2026-06-14.

Sources:
- https://cursor.com/docs/skills
- https://cursor.com/docs/rules

## Skills

Cursor supports Agent Skills as reusable `SKILL.md` packages. Project skills are generated under `.cursor/skills/` by Cursor's migration flow, and skills follow the standard folder shape:

```text
.cursor/skills/<name>/SKILL.md
```

Common fields:

```yaml
---
name: react-component-patterns
description: Conventions for writing React components in this codebase.
paths:
  - "**/*.tsx"
  - "packages/ui/**/*.ts"
disable-model-invocation: true
---
```

Notes:

- `name` and `description` identify the skill.
- `paths` scopes automatic application to matching files.
- `disable-model-invocation: true` makes the skill behave like a manual slash command.
- Optional directories include `scripts/`, `references/`, and `assets/`.
- Cursor can migrate dynamic rules and slash commands to skills with `/migrate-to-skills`.

## Rules

Project rules live under:

```text
.cursor/rules/*.mdc
```

Use rules for always-on or path-scoped instructions, not long procedural workflows.

Typical rule frontmatter:

```yaml
---
description: Style rules for Python files.
globs: "**/*.py, scripts/**/*.py"
alwaysApply: false
---
```

Rule behavior:

- Always Apply rules are persistent.
- Apply Intelligently rules require a clear description.
- Apply to Specific Files rules require matching paths.
- Manual rules can be @mentioned.

## AGENTS.md

Cursor supports root and nested `AGENTS.md`. Use it for simple, readable repo instructions without structured rule metadata. Nested `AGENTS.md` files combine with parent files; more specific instructions take precedence.

## Commands

Cursor has slash-command workflows, but for durable repo behavior prefer skills unless the user specifically wants manual invocation.
