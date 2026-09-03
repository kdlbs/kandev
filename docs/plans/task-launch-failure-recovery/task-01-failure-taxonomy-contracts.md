---
id: "01-failure-taxonomy-contracts"
title: "Launch-error persistence contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 01: Launch-error persistence contracts

Define the shared categories, actions, and persistence owners.
Do not change launch behavior in this task.

- **Acceptance:**
  1. `LastAgentError` gains recovery actions, task-repository identity, and an explicit bounded stamp.
  2. `TaskLaunchError` persists under `tasks.metadata["last_launch_error"]` with the spec fields.
  3. Typed category and recovery-action constants exist in one importable package.
  4. Load, normalize, and stable-stamp helpers have focused tests.
  5. Typed session errors use the explicit stamp. Legacy errors retain their computed-stamp fallback.
  6. Rewriting the same task-owned stamp does not change `occurred_at` or task metadata.
  7. `ErrInvalidBaseBranch` remains detectable through `errors.Is`.

- **Verification:**
  `cd apps/backend && go test ./internal/task/models/... ./internal/worktree/...`

- **Files likely touched:**
  `apps/backend/internal/task/models/models.go`,
  `apps/backend/internal/task/models/models_test.go`,
  `apps/backend/internal/worktree/errors.go`.

- **Dependencies:** None.
- **Parallelism:** parallel-safe (disjoint from task-02).
- **Inputs:** spec "Data model" and "Persistence guarantees".

## Results
- Added bounded task/session launch-error contracts, typed categories/actions, explicit stamps, deterministic stamp hashing, and compare-by-stamp in-memory helpers.
- Added focused persistence, normalization, no-op rewrite, clear, and sentinel tests.
- Verification: `go test ./internal/task/models/... ./internal/worktree/...` passed.
