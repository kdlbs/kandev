---
description: Drive Kandev feature work through spec, plan, independent tasks, implementation, QA, and verification.
argument-hint: "[feature or fix goal]"
allowed-tools: Read Edit Write Grep Glob Agent
model: inherit
effort: high
---

Rely on the root `AGENTS.md`/`CLAUDE.md` planner/worker contract and use
`.agents/skills/spec-driven-development/SKILL.md` for Kandev's planner-driven
flow:

1. Clarify intent with `/interview-me` style questions when needed.
2. Create or update the product spec with `/spec`.
3. Create the implementation plan with `/plan`.
4. Decompose into independent tasks with acceptance criteria, exact verification, likely files, dependencies, and wave ordering.
5. Keep small, localized execution in the planner session. Delegate only a
   substantial independent packet or required independent gate. `qa`,
   `security-auditor`, and `code-review` are exceptional; `verify` remains the
   final post-commit gate. Use `architect` only for a bounded second opinion on
   unusually risky design decisions.

Keep the primary planner responsible for orchestration, integration order, user
communication, and final status. It may directly implement, test, integrate,
and ship small scoped work. If an exceptional required worker is unavailable,
stop and report it.
