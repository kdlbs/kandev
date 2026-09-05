---
name: harness-improvement
description: Improve Kandev's AI harness from session learnings or explicit requests. Use when the user asks to record learnings, update or create skills, agents, subagents, commands, AGENTS.md/CLAUDE.md guidance, or adapt harness files across Claude, Codex, Cursor, or OpenCode.
---

# Harness Improvement

Use this skill to turn lessons from real agent sessions into durable harness changes: skills, agents, subagents, commands, scripts, and always-on instruction files.

## Planner Entry

The planner may inventory, edit, and validate a small localized harness change
directly. Delegate broad cross-platform migration or independent work when it
has positive ROI. Do not use Kandev MCP task/session APIs for workers.

## Choose The Artifact

Before editing, classify the requested improvement:

- **Session learning:** recurring failure, workaround, or convention discovered during a session. Read `references/session-learnings.md`.
- **Skill:** task-specific playbook loaded on demand. Read `references/skills.md`.
- **Custom agent:** an exception that requires the user to explicitly reverse the
  repository's single-session policy. Read `references/agents.md` before acting.
- **Command:** explicitly invoked workflow shortcut. Prefer a skill unless the user wants manual invocation only.
- **AGENTS.md / CLAUDE.md / rules:** always-on or path-scoped instruction. Read `references/instructions.md`.
- **Cross-platform migration:** preserving behavior across Claude, Codex, Cursor, and OpenCode. Read the relevant platform files in `references/platforms/`.

If the user names a target platform for a skill, command, or instruction file,
load only that platform reference. Do not create a custom agent or platform
mirror unless the user explicitly requests a policy reversal.

## Workflow

1. **Inventory first**
   - Use `rg --files` to find existing `.agents/skills`, `.claude`, `.codex`,
     `.cursor`, `.opencode`, `AGENTS.md`, and `CLAUDE.md` files.
   - For platform-specific formats, read the bundled files under `references/platforms/` before consulting external docs. Treat those files as the first source of truth for Claude, Codex, Cursor, and OpenCode harness layout.
   - Check for duplicate or superseded skills/agents before adding new ones.
   - Prefer updating the existing artifact when the behavior belongs to an existing workflow.

2. **Normalize the learning**
   - Convert anecdotes into reusable guidance: trigger, problem, correct action, fallback, verification.
   - Remove session-specific IDs, PR numbers, or temporary paths unless they are part of an example that teaches the pattern.
   - Keep wording direct and operational.

3. **Pick the narrowest home**
   - Put durable repo-wide constraints in `AGENTS.md` or scoped `AGENTS.md`.
   - Put task workflows in `.agents/skills/<name>/SKILL.md`.
   - Put deterministic logic in `scripts/` when agents keep retyping fragile shell/API sequences.
   - Avoid creating multiple aliases for the same behavior.

   - **Separate product and harness scope:** When a product PR surfaces a
     request to change `.agents/**`, `.github/**`, `AGENTS.md`, `CLAUDE.md`, or
     other harness policy, create a separate specified/reviewed harness
     initiative and PR by default. Include the harness change in the product
     PR only when the user explicitly requests the same PR for both changes.

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
   - Load `references/validation.md` for the shared harness test, lint,
     whitespace, line-budget, and pre-commit commands.
   - For executable script changes, run syntax checks and a focused dry run or mocked command when possible.

6. **Report**
   - Name each artifact changed.
   - State why the instruction belongs there.
   - Mention validation run and any bundled platform references consulted.

## Guardrails

- Do not blindly copy upstream examples. Adapt model names, package managers, commands, paths, and verification steps to Kandev.
- Do not add always-on instructions for rare workflows; use skills or commands.
- Keep the single-session model policy intact unless the user explicitly asks
  to change it and accepts the cost/context trade-off.
- Do not keep deprecated/replaced skills around without a clear compatibility reason.
- Do not web-search platform formats by default. Use external docs only when the bundled reference is missing the needed detail, conflicts with files already in the repo, or the user explicitly asks for latest/current upstream behavior; if that happens, say why before browsing.
