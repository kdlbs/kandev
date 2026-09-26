---
spec: docs/specs/tasks/system-design/wip-limit-pull-system.md
created: 2026-07-27
status: complete
---

# Implementation Plan: WIP-Limited Task Creation

## Overview

Task moves already enforce workflow-step WIP limits, but `CreateTask` inserts a
task directly into its resolved step without capacity admission. GitHub review
watches fan out one goroutine per newly observed PR and then auto-start every
successfully created task, so a WIP-2 start step can receive and start all eight
tasks from one poll.

The fix adds a typed, transactionally enforced WIP admission primitive, uses it
from task creation for both explicit and resolved start steps, maps capacity
rejections consistently across HTTP, WebSocket, MCP, and watcher dispatch, and
documents the corrected behavior. Watcher integration coverage also locks in
the existing lifecycle contract: an admitted task remains in its configured
step while the agent is starting and working, and advances only on a genuine
turn-complete event.

## Confirmed Root Cause

- `github.Service.TriggerReviewWatch` publishes an event for every new PR.
- `orchestrator.Service.handleNewReviewPR` starts one background
  `createReviewTask` goroutine per event.
- `task/service.Service.CreateTask` calls `TaskRepository.CreateTask` without
  reading the resolved step's `WIPLimit`.
- WIP validation exists only on move/transition paths.
- `createReviewTask` calls `StartTask` for every task created in an
  `auto_start_agent` step.

The smallest reproduction is an empty auto-start workflow start step with
`wip_limit: 2` receiving eight concurrent non-ephemeral task creations. Current
behavior persists eight tasks; the required behavior admits exactly two.

---

## Backend

### Typed WIP capacity contract

- Add `ErrWIPLimitExceeded` to
  `apps/backend/internal/workflow/models/errors.go` so task repository, task
  service, HTTP/WS/MCP adapters, and orchestrator code classify capacity
  without parsing error strings.
- Preserve user-facing context by wrapping the sentinel with the step ID,
  configured limit, and current occupancy.

### Atomic repository admission

- Extend the narrow task-placement repository contract with
  `CreateTaskIfWorkflowStepHasCapacity(ctx, task, targetStepID, limit)`.
- Implement it in
  `apps/backend/internal/task/repository/sqlite/task.go` using the same task
  insert and runner-participant transaction as `CreateTask`.
- Serialize admission on the target `workflow_steps` row. PostgreSQL uses
  `SELECT ... FOR UPDATE`; SQLite already uses a single writer connection, so
  the count and insert remain in one serialized write transaction.
- Count only non-archived, non-ephemeral occupants, matching existing WIP move
  semantics.
- Reuse the step-capacity lock and typed sentinel from
  `UpdateTaskIfWorkflowStepHasCapacity` so creation, moves, and pulls share one
  concurrency rule.

### Task service enforcement

- In `task/service.Service.CreateTask`, resolve and validate the final workflow
  step before persistence.
- For a positive `WIPLimit`, call the capacity-aware repository method instead
  of the unconditional insert. This includes requests that omit
  `workflow_step_id` and resolve to the workflow start step.
- Fail closed when a positive WIP limit is configured but the repository does
  not implement capacity-aware creation.
- Leave `wip_limit: 0`, workflow-less ephemeral tasks, and existing unlimited
  task creation unchanged.
- Ensure a capacity rejection occurs before blockers, repositories,
  `task.created`, task sessions, or auto-start side effects.

### Caller error behavior

- HTTP task creation returns `409 Conflict` with the WIP error.
- WebSocket and MCP task creation return `ErrorCodeConflict` with the same
  message.
- GitHub review dispatch recognizes `ErrWIPLimitExceeded` as a deferral:
  release the review-PR reservation, do not attach a task ID or auto-start, and
  log at a non-error level so later polls can retry without false failure
  noise.
- Other task-creation errors retain their current handling.

---

## Tests

- **What:** concurrent repository admission never exceeds WIP capacity.
  **File:** `apps/backend/internal/task/repository/task_wip_test.go`.
  **How:** seed a WIP-2 step, synchronize eight goroutines, assert exactly two
  inserts and six `errors.Is(err, ErrWIPLimitExceeded)` results; run with
  `-race`.
- **What:** explicit and resolved start-step creation obey WIP, while
  `wip_limit: 0` remains unlimited and rejected creation emits no
  `task.created`.
  **File:** new
  `apps/backend/internal/task/service/service_tasks_wip_test.go`.
  **How:** SQLite-backed service integration tests using real workflow/task
  repositories.
- **What:** HTTP, WebSocket, and MCP task creation classify WIP rejection as a
  conflict.
  **Files:** task handler and MCP handler tests.
  **How:** focused adapter tests asserting status/error code and message.
- **What:** review-watch capacity rejection releases the PR reservation and
  never assigns a task ID or auto-starts.
  **File:**
  `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`.
  **How:** deterministic fake creator returning the typed WIP error, followed
  by a retry that succeeds.
- **What:** an admitted watcher task does not advance its workflow during
  startup or active work.
  **File:**
  `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`.
  **How:** create directly in an auto-start `Review` step, assert the task
  remains there after creation and boot-ready, then emit a real turn-complete
  event and assert one transition to `Done`.

## Frontend

No frontend code changes are required. Existing task-creation surfaces already
display backend errors; the backend adapters must return a conflict instead of
an internal error.

## E2E Tests

No browser E2E is planned because there is no visual or interaction change.
The concurrency invariant and every external error boundary are covered by
targeted Go tests.

## Public Documentation

- Update `docs/public/tasks-and-workflows.md` and
  `docs/public/workflow-tips.md` to state that WIP applies to initial task
  creation, including integration-created tasks, and that full-step creation
  is rejected for later retry.
- Validate the public docs after the wording changes.

## Implementation Waves And Parallel Candidates

All tasks are sequential because each consumes the typed contract or behavior
introduced by the preceding task.

- [x] [Task 01: Atomic repository admission](task-01-atomic-repository-admission.md)
- [x] [Task 02: Task-service WIP enforcement](task-02-task-service-enforcement.md)
- [x] [Task 03: Conflict adapters and review-watch deferral](task-03-conflict-adapters-and-watcher-deferral.md)
- [x] [Task 04: Public documentation](task-04-public-documentation.md)

## Completion Evidence

- Repository admission is transactionally serialized, with PostgreSQL step-row
  locking and SQLite writer serialization; focused repository tests pass.
- Task creation enforces explicit and resolved workflow-step WIP limits, while
  unlimited and ephemeral creation remain compatible; focused service tests
  pass.
- HTTP returns `409 Conflict`; WebSocket and MCP return conflict error codes;
  watcher capacity failures release reservations and skip assignment/start.
- Review watcher lifecycle coverage confirms boot-ready does not advance the
  task and the first genuine turn completion moves Review to Done once.
- Public documentation validation: `node scripts/validate-public-docs.test.mjs`
  (58/58 passed).
- Backend verification: `go test -tags fts5 ./internal/task/repository
  ./internal/task/service ./internal/task/handlers ./internal/mcp/handlers
  ./internal/orchestrator -count=1` (2,562 tests passed).

No task is marked `parallel-safe`; waves do not authorize subagent execution.

## Risks

- A pre-insert count outside the write transaction would preserve the original
  race. Admission must be serialized per target step in both SQLite and
  PostgreSQL.
- Rejected review PRs must release their dedup reservations or they will never
  retry.
- Watcher auto-start coverage must distinguish `agent.boot_ready` from a real
  turn-complete event; treating startup readiness as completion would move a
  newly admitted task out of the limited step before its review runs.
- The resolved start-step path must be covered explicitly; checking only
  request-supplied `workflow_step_id` would leave the reported configuration
  broken.
- Existing tests intentionally seed over-limit UI states through task creation.
  Those fixtures must use repository-level setup or another explicit legacy
  state mechanism rather than weakening the production invariant.

## Out of Scope

- A GitHub review-watch `max_inflight_tasks` setting.
- Profile-wide session concurrency limits.
- Automatic feeder-step creation or watcher configuration changes.
- UI redesign or new WIP controls.
