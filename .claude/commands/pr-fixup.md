---
description: Run the Kandev PR fixup loop for CI failures and automated review threads.
argument-hint: "<PR number>"
allowed-tools: Bash Read Edit Write Grep Glob
model: opus
effort: high
---

Use `.agents/skills/pr-fixup/SKILL.md`; the root `AGENTS.md`/`CLAUDE.md`
single-session model workflow applies. Handle polling and remediation directly
in the same conversation; rerun only the affected task-defined check after a fix.

If GitHub access requires approval, surface that gate
and stop. Do not relaunch after denial, cancellation, or interruption; the
shared skill distinguishes approval gates from transient fetch failures.
