---
description: Run the Kandev PR fixup loop for CI failures and automated review threads.
argument-hint: "<PR number>"
allowed-tools: Read Grep Glob Agent
model: opus
effort: high
---

Rely on the root `AGENTS.md`/`CLAUDE.md` planner/worker contract and coordinate
`.agents/skills/pr-fixup/SKILL.md` as the primary planner.

Delegate PR state collection to `pr-poller` and broad remediation to
`implementer`. Commit focused fixes through active hooks, then delegate
post-commit checks to `verify` before push. Review
each compact result and launch another bounded assignment when needed. Do not
run GitHub, edit, test, commit, or push commands in this primary session. If a
required worker cannot be launched, stop and report the blocked phase.

If `pr-poller` reports that GitHub access requires approval, surface that gate
to the user and stop. Do not relaunch polling after approval is denied,
cancelled, or interrupted; follow `.agents/skills/pr-fixup/SKILL.md` for the
full distinction between approval gates and transient fetch failures.
