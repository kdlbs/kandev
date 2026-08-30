---
id: "06-recovery-actions-ws"
title: "Task launch recovery action"
status: done
wave: 5
depends_on: ["01-failure-taxonomy-contracts", "02-gitref-default-hardening", "03-worktree-live-default-fallback", "04-launch-failure-classification", "05-pr-review-autostart-gating", "10-task-base-self-heal", "12-remove-legacy-launch-guidance"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 06: Task launch recovery action

Add a task-scoped WebSocket action for pre-session and session launch errors.

- **Acceptance:**
  1. `task.launch.recover` implements the exact request and response shapes from the spec.
  2. The handler authorizes the task before it reads any other row.
  3. It requires the active `error_stamp` and rejects stale recovery requests.
  4. It validates optional session ownership and required task-repository ownership.
  5. `retry_default` refreshes the live default, updates both base records, and relaunches.
  6. `pick_base_branch` validates the selected remote branch, updates the exact row, and relaunches.
  7. `mark_review_done` selects only the final step accepted by `IsTerminalStep`.
  8. It rechecks that all relevant PRs are still terminal before the move.
  9. A recovery-only move option permits `FAILED` to become `COMPLETED` at the terminal step.
  10. Normal task moves continue to preserve failed and cancelled task states.
  11. The terminal action is idempotent when the task already occupies the target step.
  12. Invalid ownership, branch, PR, terminal-step, or task/session inputs cause no mutation.
  13. A default-resolution miss records `default_branch_unresolved` with `pick_base_branch`.
  14. Success compares and clears the source stamp. It cannot erase a newer error.
  15. Existing `session.recover` actions do not change.

- **Verification:**
  `cd apps/backend && go test ./internal/orchestrator/... -race`

- **Files likely touched:**
  `apps/backend/pkg/websocket/actions.go`,
  `apps/backend/internal/orchestrator/handlers/handlers.go`,
  `apps/backend/internal/orchestrator/handlers/handlers_test.go`,
  `apps/backend/internal/task/service/service_workflow.go`,
  `apps/backend/internal/task/service/service_workflow_test.go`,
  the narrow remote-default and task-move interfaces used by that handler.

- **Dependencies:** Tasks 01, 02, 03, 04, 05, 10, and 12.
- **Parallelism:** sequential.
- **Inputs:** spec "WebSocket action `task.launch.recover`" and "Recovery transitions".

## Results

- Added and registered `task.launch.recover` with the exact request and
  response shapes.
- Authorized the task before loading session, repository, or workflow rows;
  stale stamps, foreign rows, invalid branches, open PRs, and invalid terminal
  workflows leave state unchanged.
- Added `retry_default`, `pick_base_branch`, and `mark_review_done` handling,
  including live default refresh, exact task-repository writes, terminal PR
  rechecks, bounded source compare-and-clear, and typed unresolved-default
  replacement.
- Added a recovery-only workflow move option for `FAILED` to `COMPLETED` and
  kept normal failed/cancelled move preservation intact.
- Added session metadata compare-and-clear persistence coverage.
- Added a final source reload that rejects an older task or session error
  before any recovery mutation.
- Reused one workflow terminal-step traversal helper for the PR gate and
  recovery action.
- Verification: `cd apps/backend && go test ./internal/orchestrator/... -race`
  passed (2,841 tests in 9 packages).
- PR fixup verification: `cd apps/backend && go test ./internal/orchestrator/... ./internal/common/netutil/... ./internal/task/repository/sqlite/...`
  passed (3,468 tests in 11 packages), and `make lint` reported zero issues.

Second PR fixup hardening added a column-scoped, expected-value guarded default
branch update; it also rejects a relaunch with no session and keeps a successful
relaunch successful when compare-and-clear logging is the only failure. Session
recovery now uses the repository contract directly, and unresolved-default
details are sanitized before task or session persistence.
