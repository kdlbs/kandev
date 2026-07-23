---
description: Drive Kandev feature work through spec, plan, independent tasks, implementation, QA, and verification.
argument-hint: "[feature or fix goal]"
allowed-tools: Read Edit Write Grep Glob Agent
model: inherit
effort: high
---

Use `.agents/skills/spec-driven-development/SKILL.md` for the full flow; the
root `AGENTS.md`/`CLAUDE.md` planner/worker contract applies. Keep small,
localized execution in the planner session. Delegate only substantial
independent packets or required independent gates: `qa`, `security-auditor`,
and `code-review` are exceptional, while `verify` remains the final
post-commit gate. Stop and report an unavailable required worker.
