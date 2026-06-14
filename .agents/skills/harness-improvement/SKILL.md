---
name: harness-improvement
description: Improve Kandev's AI harness from session learnings or explicit requests. Use when the user asks to record learnings, update or create skills, agents, subagents, commands, AGENTS.md/CLAUDE.md guidance, or adapt harness files across Claude, Codex, Cursor, or OpenCode.
---

# Harness Improvement

Use this skill to turn lessons from real agent sessions into durable harness changes: skills, agents, subagents, commands, scripts, and always-on instruction files.

## Choose The Artifact

Before editing, classify the requested improvement:

- **Session learning:** recurring failure, workaround, or convention discovered during a session. Read `references/session-learnings.md`.
- **Skill:** task-specific playbook loaded on demand. Read `references/skills.md`.
- **Subagent/agent:** role with a distinct model, tools, permissions, or isolation. Read `references/agents.md`, then all platform references unless the user explicitly scopes the request to one platform.
- **Command:** explicitly invoked workflow shortcut. Prefer a skill unless the user wants manual invocation only.
- **AGENTS.md / CLAUDE.md / rules:** always-on or path-scoped instruction. Read `references/instructions.md`.
- **Cross-platform migration:** preserving behavior across Claude, Codex, Cursor, and OpenCode. Read the relevant platform files in `references/platforms/`.

If the user names a target platform for a skill, command, or instruction file, load only that platform reference. For subagents/agents, the default is cross-platform sync: update every existing platform mirror (`.agents/agents`, `.codex/agents`, `.claude/agents`, `.opencode/agents`) unless the user explicitly says to update only one platform.

## Workflow

1. **Inventory first**
   - Use `rg --files` to find existing `.agents/skills`, `.agents/agents`, `.claude`, `.codex`, `.cursor`, `.opencode`, `AGENTS.md`, and `CLAUDE.md` files.
   - Check for duplicate or superseded skills/agents before adding new ones.
   - For subagent/agent edits, map the role across all platform directories before editing so mirrors stay aligned.
   - Prefer updating the existing artifact when the behavior belongs to an existing workflow.

2. **Normalize the learning**
   - Convert anecdotes into reusable guidance: trigger, problem, correct action, fallback, verification.
   - Remove session-specific IDs, PR numbers, or temporary paths unless they are part of an example that teaches the pattern.
   - Keep wording direct and operational.

3. **Pick the narrowest home**
   - Put durable repo-wide constraints in `AGENTS.md` or scoped `AGENTS.md`.
   - Put task workflows in `.agents/skills/<name>/SKILL.md`.
   - Put role definitions in `.agents/agents/<name>.md` for Claude-style agents in this repo.
   - Put deterministic logic in `scripts/` when agents keep retyping fragile shell/API sequences.
   - Avoid creating multiple aliases for the same behavior.

4. **Preserve progressive disclosure**
   - Keep `SKILL.md` concise.
   - Move platform tables, long examples, templates, and edge-case notes to `references/`.
   - Reference each supporting file explicitly from the main skill so future agents know when to load it.

5. **Edit and validate**
   - Use `apply_patch` for file edits.
   - Validate markdown/frontmatter shape with targeted checks:
     ```bash
     git diff --check -- <changed-files>
     rg -n "old-skill|old-agent|stale-command" .agents AGENTS.md CLAUDE.md
     ```
   - For executable script changes, run syntax checks and a focused dry run or mocked command when possible.

6. **Report**
   - Name each artifact changed.
   - State why the instruction belongs there.
   - Mention validation run and any platform docs consulted.

## Guardrails

- Do not blindly copy upstream examples. Adapt model names, package managers, commands, paths, and verification steps to Kandev.
- Do not add always-on instructions for rare workflows; use skills or commands.
- Do not make subagents recursively spawn other subagents unless the user explicitly asks for nested orchestration.
- Do not keep deprecated/replaced skills around without a clear compatibility reason.
- When platform docs are time-sensitive or user asks for latest behavior, verify current docs before editing.
