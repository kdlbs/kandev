---
id: "01-clean-relocation"
title: "Keep clean worktrees resumable"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.4
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 01: Keep clean worktrees resumable

## Summary

Detect when a task worktree belongs to an older managed clone of the same
repository. Relocate only a verified clean, idle worktree before launch
admission. Keep the original checkout and exact commit.

## In scope

- Add source-clone identity to `task_environment_repos` with an idempotent
  SQLite and PostgreSQL migration and exact-slot writes.
- Prove legacy source identity from Git registration and provider identity.
- Reuse the environment recovery claim and filesystem claim for clean
  relocation and restart-safe publication.
- Refuse dirty, busy, foreign, ambiguous, and incomplete environments without
  changing their Git state or inventory.

## Out of scope

- User-initiated dirty transfer and rendered recovery controls.
- Local or remote executor behavior.

## Acceptance

1. A real-Git regression fails before the change with the reported two-clone
   error and passes after a clean relocation, preserving exact branch and HEAD.
2. Legacy and new worktree rows record a proven source identity. Unproved
   identities fail closed without modifying the original.
3. Concurrent claims, busy runtimes, and a later multi-repository failure
   cannot start an agent with a partial or foreign worktree.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/repoclone ./internal/worktree ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'TestManagedCloneRelocation' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Set `KANDEV_TEST_POSTGRES_DSN` for the PostgreSQL migration and concurrency
matrix in the implementation environment. Run the same targeted repository
test command with that variable set and record its result separately.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_targets.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/repoclone/clone.go`

## Dependencies

None.

## Risks

Existing worktree migration code rebuilds the environment-repository table.
The new column must survive its fresh schema and replacement copy lists.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md)
- [System design](../../specs/tasks/system-design/managed-clone-relocation.md)
- [Worktree metadata recovery](../../specs/tasks/system-design/worktree-metadata-recovery.md)
- `manager_reuse_required_test.go` and executor recovery integration tests.

## Results

Implemented clean relocation with canonical source-clone identity, guarded
admission, and exact-commit transfer. Admission accepts the verified
provider/origin source layout and the legacy owner/name layout. It recognizes a
replacement already attached to the selected destination even after the old
source clone is removed. Lifecycle preflight defers a possible relocation to
admission, while final execution validation remains fail-closed if admission
cannot run.

The legacy owner/name dirty recovery and post-relocation source-removal
regressions passed with real Git worktrees. Full worktree, repoclone, lifecycle,
and executor package tests passed, as did the backend build. The focused
worktree regressions also passed with `-race`. Earlier SQL guard and persistence
race-conformance checks passed. PostgreSQL-specific migration and concurrency
tests were not run because `KANDEV_TEST_POSTGRES_DSN` was unset. Successful
task-owned runtime stops now settle resumable executor rows to `stopped` before
recovery admission; lifecycle and SQLite claim regressions passed, and the real
dirty relocation E2E confirmed admission succeeds only after that durable state.
Final PR-fixup verification also passed the changed-code backend linter with
zero issues and the focused worktree relocation/admission regression set.
Follow-up regressions cover implicit default-local repository validation,
canonical absolute task-environment paths, and retaining a stopped passthrough
execution ID across graceful backend shutdown. The full lifecycle and task
service Go packages passed, along with the focused dirty-relocation, CLI
fallback, single-repository TUI, and multi-repository TUI restart E2Es.
The PR inventory regression also changes the task's requested base branch while
the session still selects its original worktree. Authorized relocation now
validates and resumes against that exact selected worktree identity. The new
regression and complete executor/worktree packages passed, along with the
desktop dirty-relocation E2E after a fresh backend build.
