---
id: "01-publish-clarification-task-state"
title: "Publish clarification task state"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/runtime-state-publication-order.md"
---

# Task 01: Publish clarification task state

## Acceptance

- An answered clarification persists an eligible task as `IN_PROGRESS` and
  publishes `task.state_changed` before
  `task_session.state_changed(RUNNING)`.
- The update remains guarded by the owning session's authoritative `RUNNING`
  state, so existing coordinator-stop races cannot revive a cancelled task.
- No new event, payload field, frontend-derived state, or schema is introduced.

## Verification

```bash
(cd apps/backend && go test -count=1 \
  -run 'Test(SetSessionRunning_PublishesTaskStateBeforeSession|SessionStateEventsIncludeUpdatedAt|HandleAskUserQuestion_CoordinatorStopWinsRunningTransition|HandleAskUserQuestion_CoordinatorStopWinsAfterRunningTransition)$' \
  ./internal/mcp/handlers)
(cd apps/backend && go test -race -count=1 \
  -run 'Test(SetSessionRunning_PublishesTaskStateBeforeSession|HandleAskUserQuestion_CoordinatorStopWinsRunningTransition|HandleAskUserQuestion_CoordinatorStopWinsAfterRunningTransition)$' \
  ./internal/mcp/handlers)
```

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/mcp/handlers/clarification_pause_test.go` only if the
  existing race fixture needs a non-behavioral adjustment for the canonical
  task-service path

## Dependencies

None.

## Parallelism

Sequential. Production behavior and its regression test share the same handler
lifecycle seam.

## Inputs

- Spec `What`, `State machine`, `Failure modes`, and clarification scenario.
- Plan section `Publish the guarded clarification transition`.
- `task.Service.UpdateTaskStateIfSessionState` as the canonical guarded writer
  and event publisher.
- Existing clarification coordinator-stop race tests.

## Output contract

Report the RED failure, GREEN results, files changed, exact commands and
outcomes, remaining risks, and synchronized task/plan status.

## Results

- RED: `cd apps/backend && go test ./internal/mcp/handlers -run
  '^TestSetSessionRunning_PublishesTaskStateBeforeSession$' -count=1` failed with
  only `task_session.state_changed` observed.
- GREEN: `cd apps/backend && go test -count=1 -run
  'Test(SetSessionRunning_PublishesTaskStateBeforeSession|SessionStateEventsIncludeUpdatedAt|HandleAskUserQuestion_CoordinatorStopWinsRunningTransition|HandleAskUserQuestion_CoordinatorStopWinsAfterRunningTransition)$'
  ./internal/mcp/handlers` — 7 passed.
- GREEN: `cd apps/backend && go test -race -count=1 -run
  'Test(SetSessionRunning_PublishesTaskStateBeforeSession|HandleAskUserQuestion_CoordinatorStopWinsRunningTransition|HandleAskUserQuestion_CoordinatorStopWinsAfterRunningTransition)$'
  ./internal/mcp/handlers` — 3 passed.
- Files changed: `apps/backend/internal/mcp/handlers/handlers.go` and
  `apps/backend/internal/mcp/handlers/handlers_test.go`.
- Temporary repro test was removed; no generated artifacts or external side
  effects remain.
