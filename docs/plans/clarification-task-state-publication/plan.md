---
spec: docs/specs/tasks/requirements/runtime-state-publication-order.md
created: 2026-08-02
status: complete
---

# Implementation Plan: Clarification Task-State Publication

## Overview

Repair the direct `ask_user_question_kandev` answer path so its persisted task
transition is published before the already-correct `RUNNING` session event.
The confirmed root cause is that `setTaskInProgressForClarification` writes
through the raw SQLite task repository; the database reaches `IN_PROGRESS`, but
that repository does not own `task.state_changed`, so connected clients remain
on `REVIEW` until boot hydration reloads the row. Review follow-up also
requires preserving that order when another task publication is already
draining and reporting the persisted session transition if task reconciliation
fails.

The fix reuses the task service's existing session-state-guarded update and
event publisher. It does not change task/session schemas, WebSocket payloads,
frontend merge rules, sidebar grouping, or clarification UI.

## Backend

### Publish the guarded clarification transition

- Update `setTaskInProgressForClarification` in
  `apps/backend/internal/mcp/handlers/handlers.go` to use the already-wired task
  service for the production guarded `REVIEW -> IN_PROGRESS` transition.
- Reuse `task.Service.UpdateTaskStateIfSessionState` with expected session state
  `RUNNING`. This keeps cancellation/archive guards and emits the canonical
  rich `task.state_changed` payload before the handler publishes
  `session.state_changed(RUNNING)`. Enqueue the session event through the same
  per-task FIFO so a busy queue cannot reverse the order and reentrant
  subscribers do not deadlock.
- If guarded task reconciliation returns an error, log it and still publish the
  authoritative `session.state_changed(RUNNING)` event. Retain the early return
  for a clean `taskStateChanged == false` stale-state guard.
- Preserve the existing repository fallback for narrow handler tests or
  alternate construction where no task service is available.
- Do not infer task state from session state in the frontend or add a second
  event publisher in the MCP handler.

## Frontend

No frontend production change is planned. The desktop sidebar and mobile task
drawer already share the task store and State grouping. Once the missing task
event arrives, both consume the existing authoritative `task.state` update.

Mobile parity is state-only: there is no composition, navigation, scrolling,
safe-area, pointer, or touch-target change. Existing mobile clarification
coverage continues to exercise answering a question; the backend ordering test
proves the shared state event, so no duplicate mobile layout test is required.

## Tests

- **What:** answering a clarification from `REVIEW` / `WAITING_FOR_INPUT`
  persists `IN_PROGRESS` and publishes `task.state_changed` before
  `task_session.state_changed(RUNNING)`.
  **File:** `apps/backend/internal/mcp/handlers/handlers_test.go`.
  **How:** use the real SQLite repository, real task service, and shared memory
  event bus; record lifecycle events through one wildcard subscription and
  assert durable state plus exact event order. The regression test must first
  fail with only the session event on the current implementation.
- **What:** a pre-existing task publication cannot be overtaken by the
  clarification task/session pair, and a task-service reconciliation error
  still emits the session event.
  **File:** `apps/backend/internal/mcp/handlers/handlers_test.go`.
  **How:** block one task publication with a channel barrier, resume the
  clarification concurrently, and assert the final FIFO order; inject a task
  repository error and assert the session event remains observable.
- **What:** the existing clarification cancellation races remain fail-closed.
  **File:** `apps/backend/internal/mcp/handlers/clarification_pause_test.go`.
  **How:** rerun the focused coordinator-stop race cases alongside the new test,
  including the package with `-race`.

## E2E Tests

- **Scenario:** **GIVEN** the sidebar is grouped by State and a task is in
  `Review` on an open clarification, **WHEN** the user answers while the mock
  agent intentionally remains active, **THEN** the live sidebar moves the task
  to `In progress` without reload.
- **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`.
- **What to verify:** use a test-local mock-agent script with a bounded post-answer
  delay, select State grouping through the existing sidebar page object, and
  assert the `REVIEW` group is replaced by the `IN_PROGRESS` group immediately
  after the answer. No fixed wait is used in the Playwright assertion.

## Verification Results

- Backend targeted tests passed: `go test -count=1 -run
  'Test(SetSessionRunning_PublishesTaskStateBeforeSession|SessionStateEventsIncludeUpdatedAt|HandleAskUserQuestion_CoordinatorStopWinsRunningTransition|HandleAskUserQuestion_CoordinatorStopWinsAfterRunningTransition)$'
  ./internal/mcp/handlers` — 7 passed.
- Backend race coverage passed: `go test -race -count=1 -run
  'Test(SetSessionRunning_PublishesTaskStateBeforeSession|HandleAskUserQuestion_CoordinatorStopWinsRunningTransition|HandleAskUserQuestion_CoordinatorStopWinsAfterRunningTransition)$'
  ./internal/mcp/handlers` — 3 passed.
- Focused E2E passed: `pnpm e2e:run tests/chat/clarification.spec.ts -- --grep
  'moves answered task from Review to In progress without reload'` — 1 passed.
- The E2E setup waits for the mock session's `WAITING_FOR_INPUT` state and
  establishes `REVIEW` before navigation, keeping the pre-answer sidebar group
  deterministic without adding production hooks or fixed assertion waits.
- Review follow-up targeted tests passed in normal and race modes for the
  busy-queue ordering and task-service-error paths. The changed-file
  `golangci-lint` check passed with no issues.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Publish clarification task state](task-01-publish-clarification-task-state.md) — completed

Wave 2:

- [x] [Task 02: Prove live sidebar regrouping](task-02-prove-live-sidebar-regrouping.md) — completed

Both tasks are sequential. The E2E proof depends on the backend event repair;
the waves do not authorize subagents.

Wave 3:

- [x] [Task 03: Address ordered publication review findings](task-03-address-publication-review.md) — completed

Task 03 is sequential with the backend implementation and documentation
corrections.

## Risks

- The task event is intentionally added before the existing session event;
  consumers already depend on this ordering contract for other runtime starts.
- The task service performs canonical event enrichment reads. A publication
  failure remains logged by the service and must not fabricate frontend state.
- The E2E mock agent must remain active long enough to observe `IN_PROGRESS`
  without turning the assertion into a timing sleep.

## Out of Scope

- Changing the clarification waiting transition to `REVIEW`.
- Changing sidebar grouping or deriving task state from session events.
- Changing Office task-state ownership, schemas, event names, or payloads.
