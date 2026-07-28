---
description: Drive Kandev feature work through spec, plan, independent tasks, implementation, QA, and verification.
argument-hint: "[feature or fix goal]"
allowed-tools: Bash Read Edit Write Grep Glob Agent
model: inherit
effort: high
---

Use `.agents/skills/spec-driven-development/SKILL.md` for the full flow; the
root `AGENTS.md`/`CLAUDE.md` single-session model workflow applies. Create the
durable artifacts on the strong model, pause for the user's manual switch, then
implement and run the task-defined tests in the same conversation. The PR AI
reviewers provide the semantic gate after PR creation; do not add local QA,
review, simplify, security, or broad verification gates by default.
Plans may label parallel-safe waves, but use subagents only when the user
explicitly authorizes them after selecting the implementation model.
