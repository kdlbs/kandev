---
id: "01-fence-destination-allocation"
title: "Fence profile destination allocation by entry"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.15
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Fence profile destination allocation by entry

## Scope

Before repository preparation inserts a session or mutates its workspace seed,
verify the task's accepted step and transition entry against the recorded route.
The same entry may retain its recorded destination but may not allocate another
one, including after an attach failure, a terminal destination, or uncertain
CREATED ownership. Reject stale entries. Permit a genuinely new accepted entry
to allocate according to its configured start policy.

This work does not restart a session, infer ownership, replay a prompt, change
dispatch/runtime behavior, or claim the broader recovery in issue #4477.

## Files

- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/workflow_allocation.go`
- `apps/backend/internal/task/repository/sqlite/workflow_allocation_test.go`
- `apps/backend/internal/task/repository/sqlite/workflow_allocation_postgres_test.go`
- `apps/backend/internal/orchestrator/workflow_allocation_idempotency_test.go`
- Adjacent orchestrator profile-switch fixture tests.

## Acceptance and verification

Assert one durable allocation for replay and concurrent admission, refusal for
stale, failed, terminal and unknown-owner cases, and a fresh destination for a
distinct accepted revisit. Verify persisted route and session state, not only
return values. Run the affected orchestrator and repository tests, race checks,
and changed-file lint. PostgreSQL concurrency parity requires an unskipped run
against a disposable task-owned PostgreSQL 16+ private-socket fixture; record it
separately from SQLite and mocked external-attach evidence.

The SQLite and orchestrator package checks, changed-file lint, and focused race
checks passed on `5b4416a3e6fcb561bd66c5fff5704fa3db99155e`. The focused
PostgreSQL 16.15 concurrency test passed three unskipped race-enabled runs in a
disposable task-owned private-socket fixture. External agent attachment remained
mocked; this work order does not claim provider or deployment acceptance.
