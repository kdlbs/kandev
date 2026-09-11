---
id: "03-bootstrap-projection"
title: "Persist correlated bootstrap failures"
status: completed
wave: 3
depends_on:
  - "02-workspace-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.11
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 03: Persist correlated bootstrap failures

## Summary

Persist safe typed asynchronous bootstrap failures with current execution and attempt stamps before failed-state publication. Carry optional phase and correlation through session metadata, status summary, synthetic messages, and recovery responses. Retain separate bounded resume/restore causes.

## In scope

Persist safe typed asynchronous bootstrap failures with current execution and attempt stamps before failed-state publication. Carry optional phase and correlation through session metadata, status summary, synthetic messages, and recovery responses. Retain separate bounded resume/restore causes.

## Out of scope

Reclassifying post-start provider failures, replacing existing auth/runtime recovery policy, a second error store, or database migrations.

## Acceptance

- Current asynchronous startup failure produces one safe durable bootstrap record before visible FAILED state; reload retains its stamp and causes.
- A stale execution or fallback cannot overwrite a successor record or clear its state.
- Legacy/malformed optional fields remain compatible, and credentials, local paths, raw Git hints, and empty repository labels do not enter the user-facing projection.

## Regression and verification

Add TestBootstrapFailureProjection and TestBootstrapFailureSuccessorFence. Exercise actual asynchronous failure callback, both event orders, failed persistence, stale fallback, and sanitized dual causes. Preserve managed-runtime/auth route tests. Projection tests assert byte bounds, invalid optional fields, same stamp, boot/live equivalence, and absence of raw error text.

Run from the repository root. Use the listed test names for new regressions.
If any existing test is changed beyond this list, add its exact command here
before marking results complete.

```bash
(cd apps/backend && go test ./internal/task/models ./internal/task/statussummary -count=1)
(cd apps/backend && go test -race ./internal/orchestrator/executor -run 'Test.*(StartFailure|StartFailed|LaunchFailure|Bootstrap)' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test.*(BootstrapFailure|AgentStartFailed|RecoverSession|LaunchRecovery)' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'Test.*LaunchError' -count=1)
(cd apps/backend && go test ./internal/orchestrator/handlers -run 'Test.*(Launch|Recover|Archive)' -count=1)
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/launch_failure.go`
- `apps/backend/internal/orchestrator/executor/executor_launch_failure_classification_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/task/models/launch_errors.go`
- `apps/backend/internal/task/models/ (LastAgentError model and tests)`
- `apps/backend/internal/task/statussummary/ (active-error projection and tests)`
- `apps/backend/internal/task/repository/sqlite/ (existing error compare-and-set helpers if needed)`
- `apps/backend/internal/orchestrator/task_bootstrap_failure_test.go (new)`

## Dependencies

[Task 02](task-02-workspace-admission.md).
Execution is sequential.

## Inputs

- [Package evidence and design](plan.md).
- [System design](../../specs/tasks/system-design/task-launch-failure-recovery.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md).
- Applicable REQ and AC identifiers in frontmatter.
- Read scoped AGENTS.md and the existing adjacent tests before implementation.

## Risks

Failure callbacks race terminal transitions. Reuse existing current-execution and CAS rules. Safe generic copy is required when a reason cannot be established.

## Parallelism

`sequential`

## Results

Completed. Asynchronous bootstrap failures now persist safe, typed, correlated
records before the visible failed transition. Execution and attempt fences
prevent stale callbacks or fallback work from overwriting successor failures;
bounded cause data and legacy-compatible optional fields flow through the task
status projection without persisting raw launch output.

Verification passed:

- `go test ./internal/task/models ./internal/task/statussummary -count=1`
- `go test -race ./internal/orchestrator/executor -run 'Test.*(StartFailure|StartFailed|LaunchFailure|Bootstrap)' -count=1`
- `go test -race ./internal/orchestrator -run 'Test.*(BootstrapFailure|AgentStartFailed|RecoverSession|LaunchRecovery)' -count=1`
- `go test ./internal/task/repository/sqlite -run 'Test.*LaunchError' -count=1`
- `go test ./internal/orchestrator/handlers -run 'Test.*(Launch|Recover|Archive)' -count=1`

Review remediation also verifies the atomic repository boundary and its
successor fence:

- `go test ./internal/task/repository/sqlite -run TestCommitBootstrapFailureIfCurrentExecutionGuardsStateExecutionAndStamp -count=1`
- `go test ./internal/orchestrator/executor -run TestBootstrapFailure -count=1`
- `go test ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/orchestrator/executor ./internal/orchestrator ./internal/task/repository/sqlite -count=1`
- `make -C apps/backend build` and `make -C apps/backend lint`
