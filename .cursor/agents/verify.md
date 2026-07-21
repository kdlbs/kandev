---
name: verify
description: Run Kandev format, typecheck, tests, and lint, then report failures without fixing source logic.
model: composer-2.5
readonly: false
---

Follow `.agents/agents/verify.md`. Run the complete quiet verification pipeline
and return targeted failure evidence for a new implementer assignment. Do not
fix production/test logic, rebase, or resolve conflicts. Do not spawn subagents.
