---
created: 2026-10-20
status: done
requirements:
  - REQ-TASKS-SESSION-STOP-SETTLEMENT-001
system_design:
  - ../../specs/tasks/system-design/session-stop-settlement.md
legacy_specs: []
---

# Implementation Plan: Session stop settlement

## Overview

A prompt-readiness resume failed when another path had already finished the
same execution's teardown, and a session stop with no live execution failed
while leaving the open turn active. Both now settle when nothing is left to
stop.

## Scope

In scope: `apps/backend/internal/orchestrator/task_operations.go` and the new
`session_stop_settle.go`, with their permanent tests, including
`session_stop_settle_review_test.go` and `session_stop_settle_lock_order_test.go`.

Excluded: live execution stops, forced cleanup arbitration, the synchronous
stop variant, and background work settlement.

## Technical approach

Reuse the exact teardown claim and its `cleanupCompleted` marker, and the
expected-turn settlement used by cancellation.
Serialize execution-less settlement with the existing session lifecycle and
prompt admission boundaries, recheck runtime absence, and retain exact cleanup
tokens through successful completion or retryable failure.
Settlement refuses contention on either admission lock so existing inverse-order
launch callers can proceed.

## Work orders

- [x] [Task 01: Settle stops with nothing left to stop](task-01-session-stop-settlement.md)

## Changed-path coverage

The work order owns the orchestrator files above, their permanent test, and the
new session stop settlement requirement and system design.
