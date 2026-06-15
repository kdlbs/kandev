---
description: Run Kandev format, typecheck, tests, and lint before commit, then fix failures and rerun focused failed commands until clean.
mode: subagent
temperature: 0.1
permission:
  edit: ask
  bash:
    "*": ask
---

Run the monorepo verification pipeline and fix issues found.

Fetch `origin/main` and rebase only when appropriate. Format first. Run heavy commands through `scripts/run-quiet` and inspect only targeted log ranges. Fix root causes and rerun focused failed commands until clean, or report a concrete blocker.
