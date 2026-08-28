---
id: "05-pr-review-autostart-gating"
title: "Gate terminal PR auto-start"
status: done
wave: 2
depends_on: ["01-failure-taxonomy-contracts"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 05: Gate terminal PR auto-start

Skip auto-start only when all relevant PR rows are terminal.
Persist the result on the task because no session exists.

- **Acceptance:**
  1. A positive task-repository PR number matches exact `(repository_id, pr_number)` identity.
  2. The fallback trims one documented Git ref prefix and compares branches case-sensitively.
  3. The gate skips launch only when at least one relevant PR exists and all are merged or closed.
  4. Open, empty, unknown, absent, and lookup-error states launch normally.
  5. A terminal sibling PR cannot gate an open current branch.
  6. The gate writes `tasks.metadata["last_launch_error"]` with a stable stamp from relevant PR identity and state.
  7. It offers `mark_review_done` only when the workflow has a valid terminal final step.
  8. Replaying the same gate is a semantic no-op. A later successful launch clears the task record.
  9. Manual launches remain outside the gate.
  10. Set and clear use atomic metadata-key writes. A clear with a stale stamp does not erase a newer error.

- **Verification:**
  `cd apps/backend && go test ./internal/orchestrator/... -race`

- **Files likely touched:**
  `apps/backend/internal/orchestrator/event_handlers_workflow.go`,
  `apps/backend/internal/orchestrator/event_handlers_workflow_test.go`,
  the narrow GitHub reader and task metadata writer seams used by the orchestrator,
  key-scoped repository or service methods with focused concurrency tests.

- **Dependencies:** Task 01 (`pr_already_closed` category + `mark_review_done` action).
- **Parallelism:** sequential.
- **Inputs:** spec "Relevant PR selection" and "Launch transitions".

## Results
- Added exact task-repository and PR identity matching, including positive PR
  number metadata and normalized branch fallback.
- Added fail-open terminal-state gating for deferred workflow, loaded-step, and
  GitHub review auto-start paths.
- Persisted a bounded task-owned error with a stable state-sensitive stamp and
  conditional `mark_review_done` action.
- Added compare-and-clear semantics and cleared the task-owned error after a
  later successful launch.
- Verification: focused PR-gate and persistence tests passed, including the
  SQLite compare-and-clear integration test.
