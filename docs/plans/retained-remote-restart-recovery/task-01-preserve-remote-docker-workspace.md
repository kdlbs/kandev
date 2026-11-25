---
id: "01-preserve-remote-docker-workspace"
title: "Preserve retained remote workspaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
  - AC-EXECUTORS-REMOTE-DOCKER-001.18
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-004.2
  - AC-EXECUTORS-PLUGIN-004.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
  - ../../specs/executors/system-design/remote-executor-plugins.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 01 Preserve retained remote workspaces

## Summary

Startup reconciliation must preserve an established Remote Docker container
and its runtime row. It must also keep the plugin runtime row needed to
reattach to a retained resource. The resume token and `Resumable` bit govern
native conversation recovery.

## In scope

- In `reconcileOneSessionOnStartup`, make one retention decision before the
  generic stop and prune paths for failed, completed, cancelled, created,
  and missing sessions. Preserve established or unresolved Remote Docker
  and plugin ownership. A failed ownership read leaves ownership uncertain.
- Protect the exact pending recovery turn from `abandonOpenTurnsOnStartup`.
  Work order 02 settles that turn after replay. Keep the failed-task REVIEW
  projection independent of workspace retention.
- Keep explicit archive, delete, reset, and fresh-allocation rollback on
  their authorized paths, including the plugin inventory rollback pass.

## Out of scope

Agent adoption, native restore classification, and cleanup for other runtimes.

## Acceptance

1. Reconcile failed, completed, cancelled, and missing sessions with recorded
   remote workspaces. Across two passes, Docker retains the original container
   ID, environment binding, and SSH metadata. Plugin inventory retains the
   original provider, resource handle, and checkpoint.
2. Cover an empty token, a token with `Resumable=false`, an unresolved owner,
   and a created session with recorded preparation. Startup preserves the
   ownership rows and any exact pending turn without requesting a forced stop.
3. A tokenless failed local session in the same inventory follows ordinary
   cleanup. Authorized teardown still reaches the recorded Docker container.

## Regression and verification

Add `TestReconcileRetainedRemoteWorkspaceOnStartup` to
`reconcile_restart_test.go` with a real SQLite repository and recording agent
manager. Capture the forced-stop request on the baseline. Reuse the Docker
Engine fixture controls for retention and authorized teardown.

Work order 02 lists the fixture and command for
`failed session without a resume token keeps its Remote Docker workspace`.
That case checks the original unpushed commit after restart and after Resume.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^(TestReconcileRetainedRemoteWorkspaceOnStartup$|TestReconcileSessionsOnStartup|TestReconcileFailedSessionOnStartupAbandonsOpenTurns$|TestStartupReconciliationPreservesOnlyExactPendingRemoteOwner$)' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRemoteDocker(ExecutorPreservesRetainedWorkspace|ExecutorRemovesFreshRollbackAndExplicitCleanup|FailedResumePreservesRuntime|RecoverableStopFailureRetainsOwner)$' -count=1)
```

## Likely files

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/reconcile_restart_test.go`

## Dependencies and risks

No prior work order. Recovery can overlap a startup cleanup pass.
Protect the recorded workspace before any call that can kill or remove a container.
Any unconfirmed agent state remains available for exact recovery or explicit cleanup.

## Lifecycle evidence

| Scenario | Acceptance criterion and assertion |
| --- | --- |
| Repeated restart | Docker `.18` and plugin `.1` and `.3` preserve the original owner and reattachment record. |
| Pending recovery mixed with terminal cleanup | Platform `.2` preserves the exact pending turn. Ordinary local cleanup proceeds. |
| Authorized cleanup | Docker `.12` and plugin `.2` preserve the existing archive, delete, and reset paths. |

## Results

RED: `go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^TestReconcileRetainedRemoteWorkspaceOnStartup$' -count=1` failed because startup removed every tokenless Remote Docker and plugin owner on the first pass.

GREEN:

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^(TestReconcileRetainedRemoteWorkspaceOnStartup$|TestReconcileSessionsOnStartup|TestReconcileFailedSessionOnStartupAbandonsOpenTurns$|TestStartupReconciliationPreservesOnlyExactPendingRemoteOwner$)' -count=1)` passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRemoteDocker(ExecutorPreservesRetainedWorkspace|ExecutorRemovesFreshRollbackAndExplicitCleanup|FailedResumePreservesRuntime|RecoverableStopFailureRetainsOwner)$' -count=1)` passed.

The new SQLite-backed test covers two reconciliation passes for failed,
completed, cancelled, created, and missing sessions on both remote runtimes. It
also covers a plugin row with a non-resumable token, incomplete Remote Docker
resource identity, failed-task REVIEW projection, retained open turns, and
tokenless local cleanup.
