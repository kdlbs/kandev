---
id: "03-worktree-live-default-fallback"
title: "Worktree fallback resolves live remote default"
status: done
wave: 2
depends_on: ["02-gitref-default-hardening"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 03: Worktree fallback resolves live remote default

When the stored fallback is absent, refresh the live remote default through the worktree manager.

- **Acceptance:**
  1. Exported `Manager.ResolveRemoteDefaultBranch(ctx, repoPath)` first reads `refs/remotes/origin/HEAD`.
  2. If necessary, it runs `git remote set-head origin --auto` through existing Git admission.
  3. The command uses caller cancellation, `Manager.inspectTimeout`, and noninteractive Git environment.
  4. Auth, network, cancellation, timeout, and unresolved results remain distinguishable.
  5. `resolveBaseRefWithFallback` uses the live result after the stored fallback fails.
  6. The resolved branch and fallback warning still reach the created `Worktree`.
  7. The manager stays task-agnostic and imports no task service.

- **Verification:**
  `cd apps/backend && go test ./internal/worktree/...`

- **Files likely touched:**
  `apps/backend/internal/worktree/manager_lifecycle.go`,
  `apps/backend/internal/worktree/manager_lifecycle_test.go`.

- **Dependencies:** Task 02.
- **Parallelism:** sequential.
- **Inputs:** spec "Default-branch resolution" and "Failure modes".

## Results
- Added `Manager.ResolveRemoteDefaultBranch` with local `origin/HEAD` inspection, bounded noninteractive refresh, typed auth/network/timeout/unresolved outcomes, and caller cancellation.
- Missing stored fallbacks now try the live remote default, and the existing worktree warning/detail path receives the resolved branch.
- Verification: `go test ./internal/worktree/...` passed.
