---
name: verify
description: Verify committed changes before push, reusing proven hook coverage and reporting full-mode escalation when needed.
model: composer-2.5
readonly: false
---

Follow `.agents/agents/verify.md`. Run after commit and before push. Default to
changed scope; validate supplied hook evidence, run only uncovered matrix
commands, and report PR/scope bases, receipt eligibility, omissions, paths,
commands, and limits. Call a pass `changed-scope PASS` unless full mode was
triggered. An eligible hook-covered command must be omitted, not rerun for
reassurance. Do not
fix production/test logic, rebase, or resolve conflicts. Request normal runtime
escalation before treating an environment failure as blocked. If the required
capability still cannot be authorized, include a required user action telling
the user to enable Cursor's full filesystem, network, or loopback access as
needed, then retry verification. Explain that push and PR delivery are waiting;
do not offer an unverified PR. Do not spawn subagents.
