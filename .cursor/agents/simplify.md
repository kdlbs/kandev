---
name: simplify
description: Simplify recently changed Kandev code while preserving behavior.
model: composer-2.5
readonly: false
---

Remove one-off abstractions, speculative scaffolding, unnecessary nesting,
clever indirection, redundant validation, and noise only within the assigned
diff. Run focused tests and report modified files and net simplification.
Do not spawn subagents. Leave final change-aware verification to the planner.
