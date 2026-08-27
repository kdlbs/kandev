---
id: "02-startup-recovery-publication"
title: "Publish startup recovery state"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 02: Publish Startup Recovery State

Publish authoritative session and activity state when startup recovery settles an active session.

## Acceptance

- A successful startup transition from `STARTING` or `RUNNING` to `WAITING_FOR_INPUT` emits the
  semantic session-state event with current primary-session and activity fields.
- A stored `RUNNING` and `generating` summary advances to `WAITING_FOR_INPUT` with no generating
  activity before backend readiness.
- Existing review-state, interrupted-marker, orphan-turn, executor-row, and resume behavior stays
  unchanged. An already-waiting session remains a publication no-op.

## Verification

Run these commands from the repository root:

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator ./internal/task/service ./internal/task/statussummary
git diff --check
```

The orchestrator regression must fail before the production change because startup recovery writes
the session row directly and emits no session-state event.

## Files Likely Touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/reconcile_restart_test.go`
- `apps/backend/internal/orchestrator/turn_lifecycle_test.go`
- `apps/backend/internal/task/statussummary/projector_test.go`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 01. This task owns orchestrator and status-summary files. Task 01 owns
backend startup, the new lock package, and Windows test-package configuration.

## Inputs

- Spec scenario for stale startup summaries in `bounded-task-status-delivery.md`.
- Spec scenario for the interrupted icon in `tasks/interrupted-task-indicator.md`.
- Plan section **Startup state publication**.
- The existing `updateTaskSessionStateWithHook` and `republishTaskActivityOnSettle` paths.

## Risks

- Do not publish a settled state when the conditional state update fails.
- Keep startup recovery free of agent launches and prompt dispatch.
- Preserve per-task event order so the summary cannot apply an older generating value last.

## Output Contract

Report the behavior, changed files, exact test results, blockers, and risks. Update this task and
`plan.md` in the same conversation.

## Results

- Routed startup session settlement through `updateTaskSessionStateWithHook`.
- Startup recovery now publishes authoritative session state with explicit empty activity and preserves the existing task-activity settle fallback.
- Added `STARTING`/`RUNNING` orchestrator regressions and stale-summary projector coverage.
- Verification: `go test -tags fts5 ./internal/orchestrator ./internal/task/service ./internal/task/statussummary` passed (2,658 tests).
