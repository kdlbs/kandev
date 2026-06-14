# Always-On Instructions

Use this when editing `AGENTS.md`, `CLAUDE.md`, Cursor rules, OpenCode rules, or other persistent instruction files.

## What Belongs Here

Always-on instructions should be:

- Repo setup and verification commands.
- Safety constraints.
- Codebase architecture and scoped conventions.
- Tooling gotchas that apply broadly.
- Routing pointers to skills.

Do not put long task workflows here. Move those into skills.

## AGENTS.md

`AGENTS.md` is standard Markdown. It should be concise, factual, and agent-focused: setup, tests, style, architecture, security, and PR conventions.

Use scoped `AGENTS.md` files near code they describe. The closest scoped file should carry the detailed convention.

## CLAUDE.md

Claude Code reads `CLAUDE.md`, not `AGENTS.md`. If the repo uses `AGENTS.md` as source of truth, create a lightweight `CLAUDE.md` that imports it:

```md
@AGENTS.md

## Claude Code

Claude-specific notes here.
```

Keep `CLAUDE.md` short. If it grows into task-specific procedure, move that procedure to a skill.

## Cursor Rules

Use `.cursor/rules/*.mdc` for Cursor-specific persistent instructions. Cursor also supports root and nested `AGENTS.md` for straightforward agent instructions.

Use rules for always-on or path-scoped behavior, not long workflows. Dynamic rules and slash commands should usually become skills.

## OpenCode Rules

OpenCode uses `AGENTS.md` for project rules. It also supports Claude-compatible fallbacks such as `CLAUDE.md` when no `AGENTS.md` exists.

## Validation

After instruction edits:

```bash
git diff --check -- AGENTS.md CLAUDE.md .cursor .opencode .claude
rg -n "TODO|TBD|obsolete|deprecated skill name" AGENTS.md CLAUDE.md .agents .cursor .opencode .claude
```
