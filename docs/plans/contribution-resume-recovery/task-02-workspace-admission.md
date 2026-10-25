---
id: "02-workspace-admission"
title: "Restore terminal-session workspace access"
status: completed
wave: 2
depends_on:
  - "01-contribution-preflight"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.4
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 02: Restore terminal-session workspace access

## Summary

Distinguish authorized workspace-only registration from agent startup. Recheck task/session/environment ownership, archive, and cleanup at pre-registration, post-registration, and reuse boundaries. Preserve singleflight and terminal session state.

## In scope

Distinguish authorized workspace-only registration from agent startup. Recheck task/session/environment ownership, archive, and cleanup at pre-registration, post-registration, and reuse boundaries. Preserve singleflight and terminal session state.

## Out of scope

Agent restart, prompt dispatch, new credential scope, branch reconstruction policy, and blanket terminal admission.

## Acceptance

- FAILED, COMPLETED, and CANCELLED sessions can regain valid retained workspace access without launching an agent or changing session/token state.
- Foreign, archived, deleted, cleanup-active, missing, and ambiguous ownership cases fail without surviving newly created resources.
- Concurrent restore and cleanup/resume races retain one valid owner; rollback never tears down a pre-existing shared runtime.

## Regression and verification

Add TestWorkspaceRestoreTerminalAdmission through the real registration boundary, not a mocked EnsureWorkspaceExecutionForSession result. Block runtime creation to race archive/delete and active cleanup before and after registration. Exercise retained runtime reuse as well as fresh workspace-only creation. Assert state, token, runtime inventory, no agent subprocess, and no prompt.

Run from the repository root. Use the listed test names for new regressions.
If any existing test is changed beyond this list, add its exact command here
before marking results complete.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(WorkspaceRestoreTerminalAdmission|.*Workspace.*|.*Register.*|.*Cleanup.*)' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(RestoreWorkspace|Archive|RecoverSession)' -count=1)
(cd apps/backend && go test ./internal/orchestrator/handlers -run 'Test.*Archive' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution_workspace_validation_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_restore_admission_test.go (new)`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/handlers/session_archive_conflict_test.go`

## Dependencies

[Task 01](task-01-contribution-preflight.md).
Execution is sequential.

## Inputs

- [Package evidence and design](plan.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md).
- Applicable REQ and AC identifiers in frontmatter.
- Read scoped AGENTS.md and the existing adjacent tests before implementation.

## Risks

Terminal guards also close deletion races. Keep agent startup and credential-broker rejection intact; use an internal purpose instead of empty command inference.

## Parallelism

`sequential`

## Results

Completed. Workspace-only execution admission now distinguishes retained
terminal sessions from agent startup. It rechecks ownership, archive, cleanup,
and task/session state at registration and reuse boundaries while preserving
singleflight, rollback, credential, and agent-launch guards. Failed,
completed, and cancelled retained sessions can restore workspace access without
starting an agent or changing session state.

Verification passed:

- `go test -race ./internal/agent/runtime/lifecycle -run TestWorkspaceRestoreTerminalAdmission -count=1`
- `go test ./internal/agent/runtime/lifecycle -count=1` (the full lifecycle package)
- The planned desktop and mobile recovery suites passed, including archived
  session and workspace restoration flows.

Review remediation also rechecks session admission immediately before
promoting a workspace-only execution. The lifecycle regression confirms that
terminalization during that boundary prevents promotion and agent commands.
