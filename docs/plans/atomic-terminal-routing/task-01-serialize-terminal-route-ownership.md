---
id: "01-serialize-terminal-route-ownership"
title: "Serialize terminal route ownership"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATOMIC-TERMINAL-ROUTING-001
  - REQ-TASKS-ATOMIC-TERMINAL-ROUTING-002
  - REQ-TASKS-ATOMIC-TERMINAL-ROUTING-003
acceptance_criteria:
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.1
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.2
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.3
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.4
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.5
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.6
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.1
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.2
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.3
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.4
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-003.1
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-003.2
  - AC-TASKS-ATOMIC-TERMINAL-ROUTING-003.3
system_design:
  - ../../specs/tasks/system-design/atomic-terminal-routing.md
---

# Task 01: Serialize terminal route ownership

## Intent

Ensure a task route has one winning source generation, and that terminal state
absorbs any deferred route that has not committed.

## Acceptance

- Every common task-move path performs a source-step CAS before changing route
  state; a stale claimant produces no route-owned side effect.
- A CAS rebase keeps move-owned lifecycle metadata and deliberate marker
  removal, while retaining unrelated concurrent metadata from the fresh row.
- An active-session terminal route commits the task lane, terminal state,
  transition, route operation, and deferred-row settlement before responding.
- Deferred prompts are emitted once by a committed winner and never prequeued
  by rollback or cancellation.
- Exact retries return stable absorbing outcomes and cannot recreate a target
  from a terminally settled pending row.
- SQLite and PostgreSQL use equivalent terminal settlement and pending-row
  generation checks.

## Verification

- Focused service tests cover generic source CAS, feeder lifecycle continuation,
  stranded marker cleanup, and optioned zero-transition rejection.
- Focused orchestrator/messagequeue tests cover claim fencing, lease recovery,
  completion retry, terminal settlement, and entry-options persistence.
- The managed manual-move queue E2E covers delayed lifecycle completion and
  single prompt delivery.
- PostgreSQL terminal tests run with `KANDEV_TEST_POSTGRES_DSN`; otherwise the
  exact-head CI PostgreSQL job is authoritative.
