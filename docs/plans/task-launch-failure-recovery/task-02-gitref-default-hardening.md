---
id: "02-gitref-default-hardening"
title: "Local default-branch detection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 02: Local default-branch detection

Stop persistence callers from recording a current-`HEAD`-only feature branch.
Keep `gitref` local and free of subprocess work.

- **Acceptance:**
  1. An internal result identifies whether the branch came only from `readHEADBranchFallback`.
  2. `DefaultBranchOrEmpty` returns `""` for that source.
  3. `DefaultBranch` keeps its existing display-fallback behavior.
  4. Origin refs and local `main` or `master` keep their current detection order.
  5. The package adds no `os/exec`, `subproc`, network, or context dependency.

- **Verification:**
  `cd apps/backend && go test ./internal/common/gitref/...`

- **Files likely touched:**
  `apps/backend/internal/common/gitref/gitref.go`,
  `apps/backend/internal/common/gitref/gitref_test.go`.

- **Dependencies:** None.
- **Parallelism:** parallel-safe (disjoint from task-01).
- **Inputs:** spec "Default-branch resolution" and plan "Local default detection".

## Results
- `DefaultBranch` retains its existing integration-branch precedence.
- `DefaultBranchOrEmpty` now returns empty only when the current HEAD is the final fallback, including feature-only repositories.
- Verification: `go test ./internal/common/gitref/...` passed.
