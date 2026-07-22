---
name: verify
description: Run Kandev format, typecheck, tests, and lint, then report failures without fixing source logic.
model: composer-2.5
readonly: false
---

Follow `.agents/agents/verify.md`. Run the complete quiet verification pipeline
and return targeted failure evidence for a new implementer assignment. Do not
fix production/test logic, rebase, or resolve conflicts. Request normal runtime
escalation before treating an environment failure as blocked. If the required
capability still cannot be authorized, include a required user action telling
the user to enable Cursor's full filesystem, network, or loopback access as
needed, then retry verification. Do not offer an unverified PR. Do not spawn
subagents.
