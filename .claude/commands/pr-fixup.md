---
description: Run the Kandev PR fixup loop for CI failures and automated review threads.
argument-hint: "<PR number>"
allowed-tools: Read Grep Glob Agent
model: opus
effort: high
---

Rely on the root `AGENTS.md`/`CLAUDE.md` planner/worker contract and coordinate
`.agents/skills/pr-fixup/SKILL.md` as the primary planner.

Delegate PR state collection to `pr-poller`, remediation to `implementer`, full
checks to `verify`, and commit/push to a bounded delivery assignment. Review
each compact result and launch another bounded assignment when needed. Do not
run GitHub, edit, test, commit, or push commands in this primary session. If a
required worker cannot be launched, stop and report the blocked phase.
