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
5. Delegate every execution step to `implementer`, `test-engineer`, `qa`, `security-auditor`, `code-review`, `simplify`, and `verify` as appropriate. Use `architect` only for a bounded second opinion on unusually risky design decisions.

Keep the primary planner responsible for orchestration, integration order, user
communication, and final status. It must not implement, test, integrate, verify,
or ship directly. If a required worker is unavailable, stop and report it.
