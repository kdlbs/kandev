---
id: "01-carry-entry-identity"
title: "Carry the committed entry identity"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 01: Carry the committed entry identity

`apps/backend/internal/orchestrator/service.go` publishes the transition ID
already committed by the feeder promotion transaction in its `task.moved`
event. Existing consumers and legacy zero-ID handling remain unchanged. The
same-step `task.queue_promoted` path remains separate.

An isolated event-bus check first observed ID zero where the committed task
carried ID 42; the same check passed after the fix. Existing selected
queue-promotion and task-moved checks passed. No operating service or live
agent was used.
