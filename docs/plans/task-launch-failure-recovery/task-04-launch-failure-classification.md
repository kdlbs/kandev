---
id: "04-launch-failure-classification"
title: "Persist typed launch-failure reason"
status: done
wave: 3
depends_on: ["01-failure-taxonomy-contracts", "10-task-base-self-heal"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 04: Persist typed launch-failure reason

Persist the typed session error with an exact row target.

- **Acceptance:**
  1. `transitionLaunchFailure` maps `ErrInvalidBaseBranch` to `base_branch_missing`.
  2. The record carries `TaskRepositoryID` from the lifecycle failure result.
  3. Repository actions are absent when that identity is empty or ambiguous.
  4. A narrow eligibility resolver controls whether the record includes `mark_review_done`.
  5. Eligibility requires a valid terminal step and terminal relevant PR state.
  6. Resolver absence or error omits that action and does not block error persistence.
  7. The existing session and task failed-state transitions stay unchanged.

- **Verification:**
  `cd apps/backend && go test ./internal/orchestrator/executor/... -race`

- **Files likely touched:**
  `apps/backend/internal/orchestrator/executor/executor_execute.go`,
  `apps/backend/internal/orchestrator/executor/executor_execute_test.go`,
  the narrow review-completion eligibility resolver and composition wiring.

- **Dependencies:** Task 01 and Task 10.
- **Parallelism:** sequential.
- **Inputs:** spec "Session-owned launch error" and "Failure modes".

## Results
- Added typed launch-failure classification for missing base branches, unresolved
  defaults, and generic startup failures.
- Persisted the bounded `LastAgentError` with the exact task-repository ID when
  lifecycle identity is unambiguous, and omitted repository actions otherwise.
- Added a narrow review-completion eligibility resolver. Resolver errors and
  missing wiring fail open without blocking error persistence.
- Preserved the existing failed-session transition and callback behavior.
- Verification: `cd apps/backend && go test ./internal/orchestrator/executor/... -race`
  passed with 445 tests.
