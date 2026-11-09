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
event. `apps/backend/internal/orchestrator/event_handlers_workflow.go` claims a
positive-ID promotion through the transaction in
`apps/backend/internal/task/repository/sqlite/task_queue_promotion_claim.go`,
which checks the current ledger entry under the existing task hierarchy lock.
Legacy zero-ID handling and the same-step `task.queue_promoted` path remain
separate.

An isolated event-bus check first observed ID zero where the committed task
carried ID 42; the same check passed after the fix. A later SQLite reproduction
found an older event consuming a newer marker. The stale and current entry cases
and an intervening concurrent move passed after the transactional claim. The
legacy zero-ID path still uses the prior claim behavior and cannot distinguish
two events lacking identity. PostgreSQL and the complete runtime path remain
unverified. No operating service or live agent was used.
