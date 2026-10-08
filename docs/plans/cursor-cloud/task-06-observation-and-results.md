---
id: "06-observation-and-results"
title: "Integrate cloud activity, recovery, stop, and results"
status: complete
wave: 6
depends_on:
  - "05-runtime-dispatch"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-003
  - REQ-EXECUTORS-CURSOR-CLOUD-004
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-002.3
  - AC-EXECUTORS-CURSOR-CLOUD-002.4
  - AC-EXECUTORS-CURSOR-CLOUD-002.5
  - AC-EXECUTORS-CURSOR-CLOUD-003.1
  - AC-EXECUTORS-CURSOR-CLOUD-003.2
  - AC-EXECUTORS-CURSOR-CLOUD-003.3
  - AC-EXECUTORS-CURSOR-CLOUD-003.4
  - AC-EXECUTORS-CURSOR-CLOUD-003.5
  - AC-EXECUTORS-CURSOR-CLOUD-004.2
  - AC-EXECUTORS-CURSOR-CLOUD-002.7
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 06: Integrate cloud activity, recovery, stop, and results

## Summary

Disconnects, replay, terminal races, and restart produce one history and one terminal outcome without resubmitting work.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Normalize SSE into existing messages and turns with atomic checkpoint persistence, deduplication, and once-only terminal side effects.
- Implement reconnect, retention-gap handling, rate-limited status fallback, backend restart recovery, and provider-liveness integration with task reconciliation.
- Implement durable stop intent, cancellation races, task deletion blockers, backend-shutdown detach, and feature-disabled observation/cancel draining.
- Add authorized submission-resolution commands with verified candidate binding and explicit retry acknowledgment; never infer no-run from a timeout.
- Validate and associate branch/PR result snapshots with the session repository. Preserve workflow completion and user-question guards.
- Add sanitized structured logs and bounded operation/outcome metrics.

- Integrate remote liveness with service.go startup reconciliation, reconcile_liveness.go, event_handlers_stall.go, stuck_signal_watchdog.go, and lifecycle/session.go. Guard mutations for live and unknown runs.
- Test quiet/no-output cloud runs beyond watchdog thresholds, pending completion signals, and backend downtime. Terminal provider evidence alone permits once-only settlement.
- Persist archive termination intent before asynchronous cleanup; stop known runs, retain visible unknown cleanup, revoke grants, and reject new dispatch after archive.
- Implement the four named cursor_cloud metric families and their closed label sets. Test increment boundaries and no repeated unknown count during polling.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- Disconnects, replay, terminal races, and restart produce one history and one terminal outcome without resubmitting work.
- Stop remains pending until remote termination; unavailable or unknown remote state cannot trigger execution-less recovery or destructive cleanup.
- Results attach only to the correct repository; provider completion cannot bypass workflow gates or unresolved user questions.

## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps/backend && go test ./internal/agent/runtime/cursorcloud ./internal/orchestrator/executor ./internal/task/service ./internal/backendapp -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestManaged' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'TestCloud|TestCursorCloud|TestReconcile|TestStuckSignal|TestHandleAgentStalled' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/cursorcloud -count=1)
```

### Evidence mapping

- 003.1, 003.2, 003.3: `internal/agent/runtime/cursorcloud/recovery_test.go: TestReplayCheckpoint, TestRestartObservation, TestRetentionGap`.
- 002.3, 003.4, 003.5: `internal/orchestrator/executor/executor_cursor_cloud_recovery_test.go: TestCloudCancelRace, TestCloudAuthFailure, TestCloudLiveness`.
- 002.4, 002.5, 004.2: `internal/backendapp/cursor_cloud_results_test.go: TestCloudCompletionGuards, TestResolveUnknownSubmission, TestCloudResultIdentity`.

- 003.5: `internal/orchestrator/cursor_cloud_watchdog_test.go: TestCloudNeverStartedGuard, TestCloudStuckSignalGuard, TestCloudStartupRecovery`.
- 002.7: `internal/orchestrator/cursor_cloud_archive_test.go: TestCloudArchiveRunning, TestCloudArchiveUnknown, TestCloudUnarchiveNoAutoLaunch`.
- 003.1-003.5: `internal/agent/runtime/cursorcloud/metrics_test.go: TestCloudMetricLabelsAndTransitions`.

## Files likely touched

- `apps/backend/internal/agent/runtime/cursorcloud/`.
- `apps/backend/internal/task/repository/sqlite/`.
- `apps/backend/internal/orchestrator/executor/`.
- `apps/backend/internal/orchestrator/handlers/`.
- `apps/backend/internal/task/service/`.
- `apps/backend/internal/backendapp/`.
- `apps/backend/internal/gateway/`.

- `apps/backend/internal/orchestrator/service.go`.
- `apps/backend/internal/orchestrator/reconcile_liveness.go`.
- `apps/backend/internal/orchestrator/event_handlers_stall.go`.
- `apps/backend/internal/orchestrator/stuck_signal_watchdog.go`.
- `apps/backend/internal/orchestrator/reconcile_restart_test.go`.
- `apps/backend/internal/orchestrator/stuck_signal_watchdog_test.go`.
- `apps/backend/internal/orchestrator/stuck_signal_watchdog_cancellation_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/session.go`.

## Dependencies

05-runtime-dispatch

## Risks

SSE terminal events can share an ID, and Git snapshots are conversation-wide. Neither can be deduplicated or attributed using a naive run-only mapping.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Completed archive intent, cancellation/recovery guards, persisted follow-up run attribution, unknown-submission candidate binding, explicit duplicate-risk retry, and restart classification for reserved versus interrupted submissions. Added authorized resolution routes and closed the stream/result lifecycle integration.

Passed:

- `(cd apps/backend && go test ./internal/agent/runtime/cursorcloud ./internal/orchestrator/executor ./internal/task/service ./internal/backendapp -count=1)`
- `(cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestManaged' -count=1)`
- `(cd apps/backend && go test ./internal/orchestrator -run 'TestCloud|TestCursorCloud|TestReconcile|TestStuckSignal|TestHandleAgentStalled' -count=1)`
- `(cd apps/backend && go test -race ./internal/agent/runtime/cursorcloud -count=1)`
- `git diff --check`

Automated tests used fake providers; no paid Cursor Cloud run was made.

### Review remediation

Terminal settlement now persists a pending completion receipt in the same database update. Startup and observation replay it until orchestrator completion effects are acknowledged, while idempotent receipt checks prevent repeated queue or workflow effects. Terminal readback also upserts the provider's final assistant result under the operation's stable message ID before settlement, replacing partial streamed content without changing the SSE cursor. Passed: `(cd apps/backend && go test -p 1 ./internal/agent/runtime/cursorcloud -count=1)`, `(cd apps/backend && go test -race -p 1 ./internal/agent/runtime/cursorcloud -count=1)`, `(cd apps/backend && go test -p 1 ./internal/task/repository/sqlite -run 'TestManaged' -count=1)`, and `(cd apps/backend && go test -p 1 ./internal/orchestrator -run 'TestManagedCompletionReplayIsAcknowledgedOnlyOnce|TestCloud|TestCursorCloud|TestReconcile|TestStuckSignal|TestHandleAgentStalled' -count=1)`.
