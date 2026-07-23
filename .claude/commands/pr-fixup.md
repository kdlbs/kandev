---
description: Run the Kandev PR fixup loop for CI failures and automated review threads.
argument-hint: "<PR number>"
allowed-tools: Bash Read Edit Write Grep Glob Agent
model: opus
effort: high
---

Rely on the root `AGENTS.md`/`CLAUDE.md` planner/worker contract and coordinate
`.agents/skills/pr-fixup/SKILL.md` as the primary planner.

Read and triage a small thread set, make a small scope-preserving fix, and run
focused checks directly. Use `pr-poller` only for a genuinely long wait and an
implementer only for broad remediation. Commit focused fixes through active
hooks, then delegate post-commit checks to `verify` before push. If a required
exceptional worker cannot be launched, stop and report the blocked phase.

If `pr-poller` reports that GitHub access requires approval, surface that gate
to the user and stop. Do not relaunch polling after approval is denied,
cancelled, or interrupted; follow `.agents/skills/pr-fixup/SKILL.md` for the
full distinction between approval gates and transient fetch failures.
