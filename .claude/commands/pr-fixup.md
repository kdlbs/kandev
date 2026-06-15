---
description: Run the Kandev PR fixup loop for CI failures and automated review threads.
argument-hint: "<PR number>"
allowed-tools: Bash Read Edit Write Grep Glob
model: sonnet
effort: high
---

Use `.agents/skills/pr-fixup/SKILL.md`.

Resolve the PR number, use `scripts/pr-state --summary <PR>` from the repo root for state, use `scripts/pr-resolve list <PR>` for actionable threads, and use `scripts/pr-resolve reply <PR> <comment_id> <thread_id> <body>` for replies.

Fix failed checks and unresolved current-head review threads. After pushing fixes, re-check with `scripts/pr-state --summary <PR>`; if there are no failed checks and no unresolved review threads but checks remain pending, report the exact pending state instead of waiting indefinitely unless the user asked to wait.
