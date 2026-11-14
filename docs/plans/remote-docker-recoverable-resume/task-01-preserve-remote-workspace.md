---
id: "01-preserve-remote-workspace"
title: "Preserve established remote workspaces during failed recovery"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
  - ../../specs/platform/system-design/durable-agent-journal-ownership.md
---

# Task 01: Preserve established remote workspaces during failed recovery

## Summary

Make recoverable cleanup preserve an established remote-Docker container and
its recovery evidence. Cover ordinary agent failure, resumed bootstrap failure,
and registered-launch rollback before journal-owner repair begins.

## In scope

- Extend the runtime retention policy for `remote_docker` with recorded workspace-reuse provenance.
- Cover reused compute without a previous execution ID. Keep conversation-resume intent separate.
- Protect registered-launch rollback and retained recovery inventory.
- Retain container and exact cleanup ownership when agent termination is unconfirmed.
- Preserve bounded cleanup contexts, stale-execution fences, and retryable cleanup claims.
- Preserve fresh-allocation rollback and explicit destructive cleanup.

## Out of scope

Journal acquisition, new UI, automatic Git checkpoints, unrelated teardown
policies, and permanent changes to the public runtime interface.

## Acceptance

1. An established remote runtime survives both recoverable stop reasons and
   resumed-launch rollback, with its environment, journal, credentials, and restart evidence intact.
2. Failed stop and delayed/duplicate cleanup cannot remove the container or a
   successor. Failed cleanup stays retryable and prevents workflow restart.
3. Fresh bootstrap rollback, reset, archive, and delete retain their exact
   destructive authority. Existing local-Docker and Kubernetes controls pass.

## Regression evidence

First create `manager_remote_docker_recoverable_failure_test.go`.
Reproduce the two `force=true` failures from [the investigation](reproduction.md).
Add manager-to-backend assertions for container identity and retained inventory.
Cover `AgentStopFailed`, a cancelled request context, registered rollback,
duplicate failure, a blocked stop followed by a successor, and explicit cleanup.
Include `WorkspaceReuseRequired=true` with an empty `PreviousExecutionID`.
Capture actual attachment provenance rather than treating every container ID
as reused compute. Preserve this evidence through workspace-only promotion.

Use permanent names `TestRemoteDockerRecoverableFailurePreservesRuntime`,
`TestRemoteDockerFailedResumePreservesRuntime`,
`TestRemoteDockerRecoverableStopFailureRetainsOwner`, and
`TestRemoteDockerResumeRollbackPreservesRuntime`.
Add fresh-bootstrap and explicit-cleanup controls in the same file.
Use the existing remote-Docker fake Engine fixture when asserting absence of
kill/remove requests. A force-flag assertion alone does not prove preservation.

## Verification

Run from the repository root. Run the new regressions before the production edit,
then run this block after the correction. Record actual test counts and results.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'Test(RemoteDocker|DockerRecoverableFailure|FailedKubernetesResume|FailedFreshKubernetesLaunch)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'Test.*(RecoverableFailure|Recoverable.*Cleanup|ExecutionCleanup|Bootstrap.*Cleanup)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/recovery_stop.go apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go apps/backend/internal/agent/runtime/lifecycle/manager_launch.go apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go apps/backend/internal/agent/runtime/lifecycle/manager_remote_docker_recoverable_failure_test.go
git diff --check
```

If an owned file does not change, omit it from the lint invocation. Include
every additional changed Go file. Confirm the orchestrator command discovers
the existing recovery tests and any new cleanup regression.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/recovery_stop.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`, if a private per-attempt reuse field is required.
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_remote_docker_recoverable_failure_test.go` (new)
- `apps/backend/internal/orchestrator/execution_failure_cleanup.go` and adjacent cleanup tests, only if claim behavior needs correction.

## Dependencies

None. Read the plan, both listed designs, and scoped backend guidance first.

## Risks

- `force=false` alone does not suppress `AgentStopFailed` container teardown.
- Removing inventory after failed rollback can leave an intact but unreachable workspace.
- A generic cleanup change can weaken explicit task deletion or affect another executor.
- Conversation resume and attachment to retained compute are different facts.

## Parallelism

`sequential`

## Inputs

- `AC-EXECUTORS-REMOTE-DOCKER-001.12` and delivery storage criterion `002.1`.
- Remote-Docker design: Recoverable failure and failed resume.
- Journal ownership design: Retained storage and failed attempts.
- Existing `manager_docker_recoverable_failure_test.go` and
  `manager_kubernetes_resume_bootstrap_cleanup_test.go`.
- Existing exact-execution cleanup ownership in `execution_failure_cleanup.go`.

## Results

Completed on 2026-10-29 against upstream `1b0d0881a4108697b8c346d17c5493fd34dd3f36`.

- **RED:** Temporarily restoring the previous `shouldPreserveRecoverableRuntime`
  predicate made `TestRemoteDockerRecoverableFailurePreservesRuntime/reused_bootstrap_failure`,
  `TestRemoteDockerFailedResumePreservesRuntime`, and
  `TestRemoteDockerRecoverableStopFailureRetainsOwner` fail. The first two
  observed `force=true`; the third observed a nil error instead of retained
  ownership. Restoring the correction made all three pass.
- **GREEN:** `go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'Test(RemoteDocker|DockerRecoverableFailure|FailedKubernetesResume|FailedFreshKubernetesLaunch)' -count=1`
  passed. `go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'Test.*(RecoverableFailure|Recoverable.*Cleanup|ExecutionCleanup|Bootstrap.*Cleanup)' -count=1`
  passed. `golangci-lint` with `--new-from-rev=HEAD` reported 0 issues for the
  changed packages.
- **Coverage added:** `manager_remote_docker_recoverable_failure_test.go`
  covers both stop reasons, reused versus fresh compute, conversation resume
  without attachment authority, archive/delete/force controls, an unconfirmed
  agent stop, and registered-launch rollback for both a resumed session and a
  workspace-only attach with no previous execution ID. `remote_docker_stop_preserves_test.go`
  uses a fake Docker Engine API and asserts that a retained workspace receives
  no kill or remove request, while fresh rollback and explicit cleanup do.
- **Rollback reason:** a retained remote-Docker rollback is recorded with
  `StopReasonAgentBootstrapFailed`; leaving `StopReasonLaunchRollback` on the
  instance would make the executor treat the retained container as fresh
  allocation cleanup.
- **All post-attachment rollbacks:** repository projection, command resolution,
  project MCP materialization, owner/session admission, and duplicate-execution
  race failures now route through `stopLaunchRuntimeOnRollback`. Before an
  `AgentExecution` exists it uses `ExecutorInstance.ReusedWorkspace`; afterwards
  it uses `attachedExistingCompute`. Kubernetes reconnect and recovery set the
  same provenance so their earlier rollback paths keep existing retention.
- **Regression coverage:** `manager_remote_docker_launch_rollback_test.go` runs
  each of those failure paths against a real `RemoteDockerExecutor` and fake
  Docker Engine, asserting no kill/remove for reused compute and destructive
  cleanup for a fresh allocation.
- **Provenance:** `ExecutorInstance.ReusedWorkspace` is set by Docker reconnect,
  remote-Docker recovery, and remote-Docker reconnect, then copied to the
  process-local `AgentExecution.attachedExistingCompute`. `promoteWorkspaceExecution`
  does not overwrite it. Conversation-resume state remains separate.
- **Not covered:** a live remote daemon was not exercised by this work order;
  real-container verification belongs to Task 03 and remains pending.
