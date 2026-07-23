---
name: verify
description: Run changed-scope verification by default and report full-mode escalation when needed.
model: composer-2.5
readonly: false
---

Follow `.agents/agents/verify.md`. Default to changed scope and report mode,
base/head, paths, commands, and limits; call a pass `changed-scope PASS` unless
full mode was triggered. Do not
fix production/test logic, rebase, or resolve conflicts. Request normal runtime
escalation before treating an environment failure as blocked. If the required
capability still cannot be authorized, include a required user action telling
the user to enable Cursor's full filesystem, network, or loopback access as
needed, then retry verification. Do not offer an unverified PR. Do not spawn
subagents.
