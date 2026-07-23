---
description: Run the Kandev PR fixup loop for CI failures and automated review threads.
argument-hint: "<PR number>"
allowed-tools: Bash Read Edit Write Grep Glob Agent
model: opus
effort: high
---

Use `.agents/skills/pr-fixup/SKILL.md`; the root `AGENTS.md`/`CLAUDE.md`
planner/worker contract applies. The planner handles small triage and
scope-preserving fixes directly; use `pr-poller` only for long waits,
`implementer` for broad remediation, and post-commit `verify` before push.

If `pr-poller` reports that GitHub access requires approval, surface that gate
and stop. Do not relaunch after denial, cancellation, or interruption; the
shared skill distinguishes approval gates from transient fetch failures.
