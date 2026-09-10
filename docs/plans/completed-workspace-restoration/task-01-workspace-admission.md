---
id: "01-workspace-admission"
title: "Restore retained workspace access"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-002
  - REQ-TASKS-COMPLETION-003
acceptance_criteria:
  - AC-TASKS-COMPLETION-002.2
  - AC-TASKS-COMPLETION-002.3
  - AC-TASKS-COMPLETION-002.4
  - AC-TASKS-COMPLETION-002.8
  - AC-TASKS-COMPLETION-002.13
  - AC-TASKS-COMPLETION-003.1
  - AC-TASKS-COMPLETION-003.2
  - AC-TASKS-COMPLETION-003.3
  - AC-TASKS-COMPLETION-003.4
  - AC-TASKS-COMPLETION-003.5
  - AC-TASKS-COMPLETION-003.6
  - AC-TASKS-COMPLETION-003.7
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 01: Restore retained workspace access

## Summary

Allow workspace infrastructure for retained environments independently of agent
session completion. Preserve explicit Resume, permissions, cleanup ownership,
and provider conversation data.

## In scope

- Add the workspace admission check at all creation/registration boundaries
  and keep agent launch/promotion guards unchanged. Validate cache reuse too.
- Apply it to all three workspace ensure entry points. Retain common
  singleflight and cleanup-aware registration; re-read current ownership and
  roll back only the losing attempt's resources.
- Preserve attach-only retained inventory validation, including mixed valid
  and missing required repositories. Do not broaden branch recreation.
- Audit workspace persistence and agentctl events for session/task mutation,
  empty provider token writes, stale callbacks, and subsequent explicit Resume.
- Reconcile handler terminal-state failure mapping with actual workspace
  eligibility. Keep bounded, sanitized errors and operation-accurate logs.

## Out of scope

Frontend composition, new schemas, automatic agent recovery, archive policy,
or provider protocol changes.

## Acceptance

- All workspace entry points restore eligible terminal sessions without agent
  startup; missing/foreign/archived/cleanup-owned resources fail without mutation.
- Concurrent restore, cleanup, and Resume preserve one winning execution and
  current ownership. Existing live siblings and their agent processes survive.
- Restoring and persisting workspace access preserves provider conversation
  identity for later explicit Resume, including after backend restart.

## TDD entry

Add `TestWorkspaceRestoreTerminalSessions` in `manager_execution_test.go` using
real retained inventory and the existing manager/runtime fakes. Assert workspace
success first; the baseline returns `ErrSessionTerminal`. Replace only the
workspace expectations in `TestEnsureExecutionRejectsTerminalSessionWithoutCreatingInstance`.
Retain agent-launch rejection tests. Add the named races and provider persistence
tests from the plan with channel barriers, not sleeps. The orchestrator test
must cross the real lifecycle manager rather than stub ensure success.

## Verification

Run the complete block from the repository root. These commands deliberately
cover both new workspace tests and the existing explicit-resume regressions.

```bash
(cd apps/backend && rtk go test -race ./internal/agent/runtime/lifecycle -run 'Test.*(WorkspaceRestore|EnsureExecution|EnsureWorkspaceExecution|GetOrEnsureExecution|CreateExecution|Coalesced|LaunchSession|TerminalSession|PersistExecutor)' -count=1)
(cd apps/backend && rtk go test -race ./internal/orchestrator -run 'Test.*(CompletedWorkspace|CompletedSession|LaunchRestoreWorkspace|RecoverSession|Startup|Reclaim)' -count=1)
(cd apps/backend && rtk go test -race ./internal/orchestrator/executor -run 'Test.*(Resume|TerminalSessionState)' -count=1)
(cd apps/backend && rtk go test -race ./internal/orchestrator/handlers -run 'Test.*(Launch|Recover|Workspace)' -count=1)
(cd apps/backend && rtk go test -race ./internal/agent/handlers -run 'Test.*(Workspace|Git|File|Shell|Terminal)' -count=1)
rtk git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/persistence.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go` only if admission
  identity must carry additional existing ownership metadata.
- Corresponding `manager_execution_test.go`, `manager_launch_test.go`, and `persistence_test.go`.
- `apps/backend/internal/orchestrator/session_launch.go` and `session_launch_test.go`.
- `apps/backend/internal/orchestrator/completed_workspace_restore_test.go` (new).
- `apps/backend/internal/orchestrator/handlers/handlers.go` and its tests.
- `apps/backend/internal/agent/handlers/git_handlers.go` and `git_handlers_test.go`.
- `apps/backend/internal/orchestrator/executor/executor_resume.go` only for
  restoration/Resume coordination if the existing path needs adaptation.

## Dependencies

None. Baseline includes merged PR #3564. Read scoped backend guidance before
implementation; do not replace that PR's explicit completed-resume permission.

## Risks

Authorization must precede cache hits. Cleanup admission must remain durable.
Registration and rollback must retain current execution identity. Do not hold
lifecycle locks while waiting for callbacks that acquire the same locks.
If a schema change becomes necessary, revise the design and add the required
SQLite/PostgreSQL conformance checks before proceeding.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), IDs above.
- [Design](../../specs/tasks/system-design/task-completion.md), Workspace sections.
- [Existing explicit-resume work](../task-completion/task-04-session-resume.md).
- Existing lifecycle execution, persistence, and cleanup race tests; `/tdd`.

## Results

Pending. Record the failing assertion, final commands, test counts, and any
unavailable fixture coverage when implemented.
