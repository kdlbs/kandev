---
id: "01-stall-detection-and-healing"
title: "Detect and heal orphaned active sessions in the sweep"
status: done
wave: 1
depends_on: []
plan: "plan.md"
system_design:
  - ../../specs/tasks/system-design/session-stall-visibility.md
requirements:
  - REQ-TASKS-SESSION-STALL-VISIBILITY-001
acceptance_criteria:
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.1
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.2
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.3
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.4
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.5
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.6
---

# Task 01: Detect and Heal Orphaned Active Sessions in the Sweep

## Outcome

The session reconciliation sweep's active-task pass detects and reports
execution-less, event-silent active sessions on unarchived tasks, and heals
them after a grace window, per
[REQ-TASKS-SESSION-STALL-VISIBILITY-001](../../specs/tasks/requirements/session-stall-visibility.md).

## In scope

- Active-task pass on the existing one-minute reconciliation tick.
- `task.stalled` event with one report per session per stall episode;
  delivery recorded only after publish succeeds.
- Grace-window healing through the archived pass's cancelled-session
  transition, scoped to the classified session IDs, with liveness re-checked
  at the cancellation boundary.
- Turn persistence refreshes the session row's `updated_at` so
  auto-dispatched turns reset the silence clock.
- `tasks.stallDetectionThreshold` startup setting with catalog, validation,
  environment alias, and public documentation.

## Exclusions

- Front-end consumption of `task.stalled`; archived-task reconciliation;
  live-execution stall detection; review-workflow advancement logic.

## Acceptance conditions

1. An orphaned, silent active session on an unarchived task emits exactly
   one `task.stalled` per episode and is not healed inside the grace
   window.
2. Healing never touches sessions with live executions or sessions outside
   the classified set, and never runs while a sibling session is live or
   merely stalled.
3. Read failures on the activity clock skip the task rather than
   classifying from an incomplete clock; a missing registry skips the pass.

## Verification

```sh
cd apps/backend && go test ./internal/task/service/ -run 'TestService_ActiveSessionSweep' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite/ -run 'TestCancelActiveTaskSessionsByIDs|TestCreateTurnRefreshesSessionUpdatedAt' -count=1
cd apps/backend && go test ./internal/common/config/ ./internal/events/... -count=1
cd apps/backend && go vet ./internal/...
```

## Files touched

- `internal/task/service/active_session_stall.go`, `active_session_stall_test.go`
- `internal/task/service/archived_session_reconciliation.go`, `service_tasks.go`, `service.go`
- `internal/task/repository/sqlite/{session,task,message}.go`, `session_test.go`, `task_active_sessions_test.go`
- `internal/task/repository/interface.go`
- `internal/events/types.go`
- `internal/common/config/{catalog,config,source,validation}.go`
- `internal/backendapp/{main,orchestrator,adapters}.go`
- `docs/public/configuration.md`, root `AGENTS.md`

## Results

Implemented; 13 sweep regressions (detection, live-execution skip,
threshold skip, episode dedupe, healing, sibling guard, archived skip,
registry fail-closed, mid-sweep registration abort, ID-scoped cancel,
per-session episode pruning, publish-retry, payload timing alignment) plus
repository regressions for the ID-scoped cancel and the turn-clock refresh
all pass. CI and review-driven remediations (session-scoped cancellation,
fail-closed activity reads, episode pruning, publish retry, turn clock) are
included; see PR #3832.
