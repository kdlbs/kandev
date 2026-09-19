---
id: "10-task-base-self-heal"
title: "Carry task-repository launch identity"
status: done
wave: 2
depends_on: ["01-failure-taxonomy-contracts"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 10: Carry task-repository launch identity

Carry exact task-repository identity through launch preparation.
Use that identity for failure targeting and fallback self-heal.

## Design constraint

The worktree manager remains task-agnostic.
The orchestrator owns task writes after lifecycle returns the exact row identity and resolved branch.

- **Acceptance:**
  1. `TaskRepositoryID` flows through every request and result type listed in `plan.md`.
  2. Fresh launch, resume, single-repository synthesis, reuse, and multi-repository mapping preserve it.
  3. Same-repository multi-branch results keep distinct task-repository IDs.
  4. A failed per-repository result identifies the exact row without branch-string correlation.
  5. The orchestrator calls `UpdateRepositoryBaseBranch` only after fallback and a changed branch.
  6. No write occurs for an existing base, empty resolved base, or unchanged base.
  7. A write error logs a warning and does not fail the launch.
  8. Only the recovered row changes in multi-repository and multi-branch tasks.
  9. Lifecycle and worktree packages add no task-service dependency or setter.

- **Verification:**
  `cd apps/backend && go test ./internal/orchestrator/executor/... ./internal/agent/runtime/lifecycle/... ./internal/task/service/... -race`

- **Files likely touched:**
  `apps/backend/internal/orchestrator/executor/executor.go`,
  `apps/backend/internal/orchestrator/executor/executor_resume.go`,
  `apps/backend/internal/orchestrator/executor/executor_execute.go`,
  `apps/backend/internal/agent/runtime/lifecycle/types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/env_preparer.go`,
  `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`,
  owning mapping and multi-repository tests.

- **Dependencies:** Task 01.
- **Parallelism:** sequential.
- **Inputs:** spec "Session-owned launch error" and "Persistence guarantees".
  Reuse `Service.UpdateRepositoryBaseBranch` from `service_branch_update.go`.

## Results
- Threaded exact `TaskRepositoryID` through executor, lifecycle, adapter, resume, reuse, synthesis, and multi-repository worktree result paths.
- Added fallback result fields and an optional orchestrator-owned task-service seam that updates only changed, identified fallback rows; write failures are warnings.
- Verification: `go test ./internal/orchestrator/executor/... ./internal/agent/runtime/lifecycle/... ./internal/task/service/... -race` passed.
