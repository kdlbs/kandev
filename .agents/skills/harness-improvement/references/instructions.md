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

Keep rules that apply to every task in the root file. Keep subtree-wide rules
in the relevant scoped file. Do not move an always-on safety or workflow rule
behind a topic-specific reading trigger.

For conditional detail, first check for an existing skill, specification, or
reference. Keep one authoritative copy. If a new reference is needed, place it
near its owning subsystem and link it from the applicable instruction file.
State when the agent must read it. Keep the reference focused on that topic and
use short, direct instructions. Preserve security, ownership, recovery, and
compatibility invariants when removing background or historical detail.

When moving guidance, check that relative links still resolve. Update links and
reading triggers at the same time. Check that existing links to headings still
point to the intended instructions. Do not rely on a linked reference being
loaded automatically.

Codex selects `AGENTS.override.md` before `AGENTS.md` in the same directory;
the override replaces that directory's file rather than extending it. Do not
introduce an override as a way to bypass shared instructions or the budget.
The 28 KiB lint budget applies to the selected root-to-leaf instruction chain,
not to each file separately. See `validation.md` for the check and its limits.

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

## Automation Coverage

When creating or updating automation that classifies harness artifacts, derive the allowlist from the platform references and include platform-native instruction/config roots, not only the repo's current source-of-truth roots. Include the existing `.agents`, `.augment`, `.claude`, `.codex`, and `.opencode` agent/skill roots plus platform-native surfaces such as `.claude/settings.json`, `.claude/rules/**/*.md`, `.claude/commands/**/*.md`, `.cursor/rules/**/*.mdc`, `.cursor/skills/**/SKILL.md`, and `.opencode/skills/**/SKILL.md`.

## Validation

After instruction edits:

```bash
git diff --check -- AGENTS.md CLAUDE.md .cursor .opencode .claude
rg -n "TODO|TBD|obsolete|deprecated skill name" AGENTS.md CLAUDE.md .agents .cursor .opencode .claude
```
