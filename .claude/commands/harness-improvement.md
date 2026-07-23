---
description: Update Kandev harness files from session learnings or explicit cross-platform harness requests.
argument-hint: "[learning or requested harness change]"
allowed-tools: Bash Read Edit Write Grep Glob Agent
model: inherit
effort: medium
---

Rely on the root `AGENTS.md`/`CLAUDE.md` planner/worker contract and use
`.agents/skills/harness-improvement/SKILL.md`. Make small, localized harness
edits and validation in the primary command; delegate only broad cross-platform
migration or independent evidence with clear positive ROI.

First read the relevant bundled reference files under `.agents/skills/harness-improvement/references/`. For platform-specific formats, use `references/platforms/` as the first source of truth and do not browse unless a bundled reference is missing, contradictory, or the user explicitly asks for latest upstream behavior.

Honor the user's scope. For subagents, update all requested project-local platform mirrors. Do not commit unless explicitly asked.
