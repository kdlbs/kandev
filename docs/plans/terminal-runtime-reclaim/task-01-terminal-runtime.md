---
id: "01-terminal-runtime"
title: "Reclaim settled terminal runtimes"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-002
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-002.3
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 01: Reclaim settled terminal runtimes

## Scope

`apps/backend/internal/orchestrator/reconcile_liveness.go` and
`apps/backend/internal/orchestrator/idle_session_reaper.go` handle the live
failed/cancelled case through the existing reaper. Preserve prompt admission,
known-work checks, current execution identity and the resumable record.
`apps/backend/internal/orchestrator/reclaim_lifecycle_test.go` updates existing
expectations for these states.

## Current evidence and remaining work

Focused existing tests and disposable SQLite/fake-manager checks covered the
classification, preserved token, unknown queue, exact owner and serialized
prompt admission. A later check reproduced a no-op cleanup incorrectly marking
the row stopped. The candidate now requires a confirmed standalone lifecycle
cleanup before repairing a live runtime row; no-op and stale-owner controls
passed in temporary tests. Real lifecycle stop/resume, other runtime adapters,
PostgreSQL, and the complete startup recovery path have not been verified.
Startup guard ownership and asynchronous stop outcomes must be preserved before
the original restart portion is complete.
