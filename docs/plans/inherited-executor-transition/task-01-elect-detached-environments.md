---
id: "01-elect-detached-environments"
title: "Elect and preserve detached environments"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.5
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.6
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.7
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.8
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01: Elect and preserve detached environments

## Summary

Make inherited executor transitions use one durable child-owned environment
before allocating executor resources. Keep the parent and shared-group
environment unchanged, and make the child environment authoritative for every
later child session.

## In scope

- Add or extend the repository transaction that claims the child environment
  and binds the launching session before lifecycle allocation.
- Use the current session as the durable materialization owner. Ensure a losing
  session waits for or attaches to the winning environment without allocating
  another physical workspace.
- Apply the rule to both `inherit_parent` and `shared_group` admission,
  propagation, and launch paths.
- Classify eligible mismatches before inherited readiness waits or failed
  foreign-environment recovery.
- Retire a prepared session's old runtime without removing the inherited
  environment or its worktree. Drive retirement from the persisted runtime
  `task_environment_id` versus the selected child environment, after existing
  live-agent admission and executor-mismatch refusals. Retry retirement after
  an earlier claim commit, even when the current call did not perform the
  binding; refuse unknown runtime identity.
- Require workspace-only transitions to persist the child environment before
  success or agent admission.
- Preserve typed refusal for Kubernetes, resume of a session still bound to the
  inherited environment, and unsafe or unknown ownership.
- Add real-repository and barrier-based regression coverage for two different
  session IDs, later sessions, group members, creating/failed parents, and stale
  prepared runtimes.

## Out of scope

- Public documentation and MCP schema copy; Task 02 owns those files.
- New task workspace modes or schema migrations.
- Changing the group pointer, releasing membership, or transferring ownership
  of the inherited environment.
- Browser layout or transport changes.

## Acceptance

- An eligible different non-Kubernetes launch claims a child-owned environment
  before any executor resources are allocated. Same-executor, Kubernetes, and
  resume behavior retain their specified outcomes.
- Later child sessions use the child's canonical environment. The parent or
  group's environment reference and other members remain unchanged.
- A two-session race with distinct session IDs creates one canonical child
  environment and one physical workspace. A prepared session cannot reuse the
  inherited runtime or return workspace-only success before environment
  persistence.
- A same-executor race makes the losing session wait for READY and attach to
  the winner's environment. It may start its own session runtime, but only the
  owner performs full workspace preparation.
- Retirement retries after cleanup failure and after an executor mismatch
  refusal, without cleaning a live agent before the existing admission checks.
- An active-agent refusal before a transition leaves the inherited session
  binding, workspace path, child environment count, parent environment, and
  runtime inventory unchanged for agent-start and workspace-only launches,
  including when a persisted running row exists without an in-memory agent.
- If a materialization owner fails or its session is cancelled, a successor
  can safely take over only when the existing physical-evidence guard permits
  it; a former owner cannot publish completion after takeover.
- Concurrent sessions selecting different executor types still bind to one
  canonical child environment. The losing executor receives a clear mismatch
  refusal before it allocates another workspace.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/... ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle -run 'Test.*(Inherited|SharedGroup|ExecutorTransition|Materialization|Detached)' -count=1
```

## Verification results

- RED: the different-executor SQLite race test found that the refused session
  stayed bound to the inherited parent environment instead of the canonical
  child environment.
- GREEN: after the claim transaction persisted that binding before returning
  the mismatch, the SQLite election, cancellation-recovery, and executor-layer
  barrier tests passed.
- RED: strict resume of a session already bound to the child canonical
  environment accepted a different executor.
- GREEN: resume now returns `ErrWorkspaceExecutorMismatch`; the inherited
  workspace resume control remains covered.
- RED: cleanup failure followed by retry skipped retirement after the durable
  child binding had committed; executor mismatch refusal followed by retry
  also left the inherited runtime available.
- RED: real-SQLite agent-start and workspace-only refusals for a live inherited
  session left the session bound to a creating child environment with an empty
  workspace path.
- GREEN: persisted runtime `task_environment_id` now drives identity-aware
  retirement on every launch after admission and canonical-executor checks.
  The retry tests prove a committed claim does not suppress cleanup; unknown
  runtime identity fails closed and active-agent refusal happens first.
- GREEN: current agent-start admission now runs before the durable transition
  claim, and workspace-only launches check a live agent before any claim that
  would rebind the session. Both real-SQLite refusal cases preserve the
  inherited binding and path, create no child environment, and leave the parent
  and runtime inventory intact.
- RED/GREEN: a workspace-only launch with no in-memory runtime but a persisted
  `executors_running` row in `running` status previously proceeded into the
  transition; the pre-claim admission now uses the already-loaded row with the
  existing `executorHasActiveAgent` predicate and refuses without mutation.
- Passed the exact focused package command above after the final refactor.
- Passed focused retirement and lifecycle persistence tests:

  ```bash
  cd apps/backend && go test ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run '^(TestLaunchPreparedSession_InheritedRuntimeRetirementRetriesAfterFailure|TestLaunchPreparedSession_InheritedRuntimeRetirementFollowsExecutorRefusal|TestLaunchPreparedSession_RefusesInheritedRuntimeWithoutEnvironmentIdentity|TestLaunchPreparedSession_DoesNotRetireInheritedRuntimeBeforeAgentAdmission|TestBuildRunningFromExecutionPersistsTaskEnvironmentIdentity)$' -count=1
  ```

- Passed race coverage for the concurrent claims, both allocation barriers,
  runtime retirement retries, and lifecycle identity persistence:

  ```bash
  cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run '^(TestClaimInheritedTaskEnvironmentTransitionDifferentExecutorsShareOneCanonicalEnvironment|TestClaimInheritedTaskEnvironmentTransitionRecoversCancelledOwnerWithoutStalePublication|TestLaunchPreparedSession_InheritedExecutorRaceUsesOneSQLiteClaim|TestLaunchPreparedSession_InheritedSameExecutorRaceWaitsAndAttachesToOneSQLiteWorkspace|TestLaunchPreparedSession_InheritedRuntimeRetirementRetriesAfterFailure|TestLaunchPreparedSession_InheritedRuntimeRetirementFollowsExecutorRefusal|TestLaunchPreparedSession_RefusesInheritedRuntimeWithoutEnvironmentIdentity|TestLaunchPreparedSession_DoesNotRetireInheritedRuntimeBeforeAgentAdmission|TestBuildRunningFromExecutionPersistsTaskEnvironmentIdentity)$' -count=1
  ```
- Passed focused race coverage for live-agent refusal and the same-executor
  READY barrier:

  ```bash
  cd apps/backend && go test -race ./internal/orchestrator/executor -run '^(TestLaunchPreparedSession_LiveInheritedTransitionRefusalDoesNotCommitBinding|TestLaunchPreparedSession_InheritedSameExecutorRaceWaitsAndAttachesToOneSQLiteWorkspace|TestLaunchPreparedSession_DoesNotRetireInheritedRuntimeBeforeAgentAdmission)$' -count=1
  ```
- PostgreSQL transaction coverage compiled and was invoked with
  `go test ./internal/task/repository/sqlite -run '^TestPostgresInheritedTransition' -count=1 -v`;
  all three PostgreSQL-only tests explicitly skipped because
  `KANDEV_TEST_POSTGRES_DSN` is unset in this environment.
- Passed `gofmt -l` for all changed Go files and `bash scripts/lint-go-changed`
  over all changed Go files (`0 issues`).
- Passed public-doc tests (63/63), public-doc validation (55 pages), docs
  inventory validation (437 decisions and 1937 specifications), specification
  lint, and `git diff --check`.

## Regression tests

- `TestClaimInheritedTaskEnvironmentTransitionDifferentExecutorsShareOneCanonicalEnvironment`
  proves two distinct sessions bind to one child environment and the other
  executor receives a typed mismatch.
- `TestClaimInheritedTaskEnvironmentTransitionRecoversCancelledOwnerWithoutStalePublication`
  proves cancellation allows guarded takeover and rejects the former owner's
  stale finalizer.
- `TestLaunchPreparedSession_InheritedExecutorRaceUsesOneSQLiteClaim` uses a
  channel barrier at physical allocation to prove the losing executor does not
  launch a second workspace and workspace-only success follows persistence.
- `TestLaunchPreparedSession_InheritedSameExecutorRaceWaitsAndAttachesToOneSQLiteWorkspace`
  uses real SQLite and a channel barrier to prove the same-executor loser waits
  for READY, then attaches with `WorkspaceReuseRequired` to the single canonical
  environment.
- `TestLaunchPreparedSession_LiveInheritedTransitionRefusalDoesNotCommitBinding`
  uses real SQLite to prove active-agent refusals leave both the session binding
  and child environment inventory unchanged for agent-start and workspace-only
  launches. It covers an in-memory agent and a persisted running execution row
  with no in-memory agent.
- Runtime retirement tests prove cleanup retry after a committed claim,
  refusal before cleanup for another selected executor, active-agent admission
  before retirement, and fail-closed behavior when the runtime lacks durable
  environment identity.
- PostgreSQL-gated multi-connection tests prove task-row and advisory-lock
  rechecks plus successor-claim stale-finalizer fencing. They skip explicitly
  when `KANDEV_TEST_POSTGRES_DSN` is unset.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_inherited_env_transition.go`
- `apps/backend/internal/orchestrator/executor/executor_inherited_env_runtime_retirement_test.go`
- `apps/backend/internal/orchestrator/executor/executor_inherited_env_transition_integration_test.go`
- `apps/backend/internal/orchestrator/executor/inherited_workspace_admission.go`
- `apps/backend/internal/orchestrator/handoff_inheritance.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/persistence.go`
- `apps/backend/internal/agent/runtime/lifecycle/persistence_test.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_inherited_transition.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_inherited_transition_test.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_inherited_transition_postgres_test.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/orchestrator/executor/executor_inherited_env_detach_test.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_test.go`
- `apps/backend/internal/orchestrator/handoff_inheritance_test.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_test.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_materialization_claim_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` if the
  existing lifecycle cleanup seam needs a narrow correction.

## Dependencies

None.

## Risks

- The election must be enforced by repository transactions and the existing
  unique task-environment identity, not only by an in-process mutex.
- Cleanup must retire only the old session runtime. It must not delete a
  parent-owned or group-owned workspace.
- Existing failed-environment recovery must never transfer a foreign claim to
  the child. The transition creates or claims a separate child environment.
