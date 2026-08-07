---
id: "06-review-blockers"
title: "Close admission review blockers"
status: done
wave: 4
plan: "plan.md"
spec: "../../specs/platform/git-subprocess-admission.md"
depends_on:
  - "01-class-aware-admission"
  - "02-interactive-git-fanout"
  - "03-lifecycle-git-classification"
  - "04-poller-admission-liveness"
---

# Task 06: Close Admission Review Blockers

## Root Cause

The first migration audited the old `subproc.Git()` accessor but not every raw
`exec.Command*(..., "git", ...)` invocation. The scheduler also treated a
cancel-versus-release interleaving as a successful grant, and fresh status used
the background-only polling helper for its final porcelain command.

## Acceptance

- Every production Git command starts through an explicitly classified shared
  helper; a repository guard rejects raw Git execution outside tests and the
  approved command-construction seam.
- A waiter canceled at the grant boundary returns `ErrAdmissionCanceled` (and
  the context error), starts no command, and leaves the round-robin cursor and
  slot accounting correct.
- Every subprocess in a fresh interactive status observation is admitted as
  `GitInteractive`; polling commands remain `GitBackground` and retain
  `GIT_OPTIONAL_LOCKS=0`.

## Verification

```bash
cd apps/backend && go test ./internal/common/subproc -run 'Test(ClassAdmission|GitAdmission|Throttle|RawGit|FreshStatus)' -count=1
cd apps/backend && go test -race ./internal/common/subproc -run 'Test(ClassAdmission|GitAdmission|Throttle|RawGit|FreshStatus)' -count=1
cd apps/backend && go test ./internal/agentctl/server/process -run 'Test(.*FreshStatus|.*Admission|.*Poll)' -count=1
cd apps/backend && go test -race ./internal/agentctl/server/process -run 'Test(.*FreshStatus|.*Admission|.*Poll)' -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/agentctl/server/api -run 'Test.*(Git|Admission|Materialize|Worktree)' -count=1
cd apps/backend && rg -n 'exec\.(Command|CommandContext)\([^\n]*"git"|exec\.LookPath\("git"\)|\b(unix|syscall)\.Exec\(' . --glob '*.go' --glob '!*_test.go'
```

## Files

- `apps/backend/internal/common/subproc/admission.go`
- `apps/backend/internal/common/subproc/admission_test.go`
- `apps/backend/internal/common/subproc/raw_git_audit_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_cmd.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/cmd/agentctl/github_cli_shim.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- direct Git call sites identified by the repository audit
- this task's sibling tests and the shared command helpers

## Results

- Migrated remaining production Git construction paths (task services, local
  repository initialization, worktree helpers, agentctl materialization, and
  lifecycle helpers) to the shared command seam and classified runners.
- Added a repository AST guard that rejects raw Git command construction,
  executable lookup, and direct `unix/syscall.Exec` outside the `subproc` seam.
- Made cancellation win both queue-head dispatch and the post-grant select
  boundary; canceled grants release their slot before returning the context
  error.
- Split lock suppression from scheduling class so fresh status remains
  `GitInteractive` while background polling remains `GitBackground`.
- Added deterministic cancellation/release and fresh-status class-attribution
  regression tests.

Validation:

```text
go test ./internal/common/subproc ./internal/task/gitinit ./internal/task/service -run 'TestProductionGitCommandsUseTheAdmissionSeam|TestClassAdmissionCancellationWinsReleaseBoundary|TestClassAdmissionCancellationDoesNotAdvanceQueue|TestPerformFreshBranch|TestInitializeLocalRepository' -count=1  # pass
go test -race ./internal/common/subproc -count=1  # 32 tests pass
go test ./internal/agentctl/server/process ./internal/common/subproc ./internal/task/service ./internal/task/gitinit ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/agentctl/server/api ./internal/worktree ./internal/repoclone ./internal/office/configloader ./internal/office/skills ./internal/office/testharness ./internal/orchestrator/executor -count=1  # pass
go test ./cmd/agentctl ./cmd/mock-agent -count=1  # 259 tests pass
go test -run '^$' ./...  # compile pass
make -C apps/backend build  # pass
```

The repository-wide `go test -tags fts5 ./...` reached 10,254 passing tests,
29 skips, and one pre-existing failure in
`TestTaskEventBroadcaster_NoDuplicateSubscriptions` (62 subscriptions versus
61). The isolated websocket test reproduces the same failure outside this
change's scope (`go test -tags fts5 ./internal/gateway/websocket -run
TestTaskEventBroadcaster_NoDuplicateSubscriptions -count=1`).
