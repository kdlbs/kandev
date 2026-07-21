# Cursor Platform Reference

Verified against Cursor docs on 2026-07-20.

Sources:
- https://cursor.com/docs/skills
- https://cursor.com/docs/rules
- https://cursor.com/docs/subagents
- https://cursor.com/changelog/2-4

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

## Custom Subagents

Launch workers through Cursor's native custom-subagent mechanism. Kandev MCP
task/session APIs are platform-management APIs, not Cursor delegation tools.

Cursor project custom subagents live in:

```text
.cursor/agents/<name>.md
```

Use Markdown with YAML frontmatter:

```yaml
---
name: implementer
description: Execute one bounded implementation assignment.
model: composer-2.5
readonly: false
---

Implement only the assigned scope and report verification.
```

Important fields used by this repository:

- `name`: stable role name matching the filename.
- `description`: routing trigger for automatic or explicit delegation.
- `model`: concrete Cursor model slug. Use `composer-2.5` for normal workers
  and `grok-4.5` for frontier architecture, security, and deep review roles.
- `readonly`: `true` for architecture, QA, review, security, and polling;
  `false` for implementation, tests, simplification, and verification.

Custom subagents receive their own context and can use custom prompts, tool
access, and models. Keep worker bodies explicit that they do not spawn further
subagents. Cursor writes remain subject to the user's normal permission policy.

Cursor has no verified per-agent effort or reasoning-effort frontmatter field
as of 2026-07-20. Kandev `.cursor/agents/` mirrors encode model tier only; role
effort mapping lives in `.agents/agents/` and Codex mirrors.

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
