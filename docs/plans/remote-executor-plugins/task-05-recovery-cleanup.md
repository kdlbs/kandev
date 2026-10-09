---
id: "05-recovery-cleanup"
title: "Recover exact resources and fence cleanup"
status: done
wave: 5
depends_on:
  - "04-provision-bootstrap"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-004
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-004.2
  - AC-EXECUTORS-PLUGIN-004.3
  - AC-EXECUTORS-PLUGIN-004.4
  - AC-EXECUTORS-PLUGIN-004.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 05: Recover exact resources and fence cleanup

## Summary

Integrate plugin resources with startup recovery, ordinary Stop, archive/delete and reset. Keep uncertain resources and resumable conversation information durable.

## In scope

- Recover unresolved operations by original identity; attach exact resources using recorded profile snapshots and compatible state versions.
- Read retained rows including stopped sessions, keep runtime metadata session-scoped, and distinguish provider outage from confirmed absence.
- Use environment cleanup claims and execution CAS for all external teardown; do not provision reset replacements until prior absence is confirmed.

## Out of scope

- Durable transcript journal, shared session workspaces, automatic replacement after workspace loss.

## Acceptance

- Backend and plugin restart reattach the recorded environment; found/absent/unknown operation recovery cannot allocate duplicates.
- Stop and shutdown preserve compute; archive/delete/reset remove only the claimed resource and retain failed cleanup for retry.
- Late results and ownership transfers cannot destroy successors; compute loss preserves conversation resume data without claiming workspace recovery.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/task/service ./internal/task/repository/sqlite -run 'TestPluginExecutor' -count=1)
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL DSN}" && go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorPostgres' -count=1)
```

Required evidence:

- `internal/agent/runtime/lifecycle/executor_plugin_recovery_test.go: TestPluginExecutorRestartRecovery, TestPluginExecutorUnknownOperation`
- `internal/agent/runtime/lifecycle/executor_plugin_cleanup_test.go: TestPluginExecutorStopMatrix, TestPluginExecutorOwnershipFence`
- `internal/task/repository/sqlite/executor_plugin_inventory_postgres_test.go: TestPluginExecutorPostgresCleanupClaim`

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/internal/agent/runtime/lifecycle/ (new executor_plugin_recovery_test.go, executor_plugin_cleanup_test.go)`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_stop.go and recovery_guard.go`
- `apps/backend/internal/task/repository/sqlite/executor_plugin_inventory.go`
- `apps/backend/internal/task/service/ existing environment cleanup services`

## Dependencies

[Task 04](task-04-provision-bootstrap.md).

## Risks

Status=stopped is not proof of resource absence. Use durable ownership generation throughout cleanup, not a check followed by an unguarded provider call.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Implemented provider operation recovery by original identity, exact resource inspection and attachment, transient agentctl token recovery, and stopped-row retention. Shutdown detaches plugin-backed executions without stopping remote compute. Archive, delete, and environment reset use the admitted task cleanup job plus an environment ownership claim; cleanup checkpoints absence or retains retry inventory, and row deletion remains guarded by resume-safety and execution/generation CAS.

Added cleanup-job validation to environment recovery claims so provider teardown serializes within its already admitted archive/delete/reset barrier. Claims permit the stopped session's own inventory row while still rejecting other sessions or executions. Inventory reads now include stopped rows for cleanup and plugin lifecycle checks.

Validation passed:

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/task/service ./internal/task/repository/sqlite -run 'TestPluginExecutor' -count=1)
# Set KANDEV_TEST_POSTGRES_DSN to a disposable local PostgreSQL instance before this command.
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorPostgres' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/task/service ./internal/task/repository/sqlite ./internal/task/recoveryclaim ./internal/backendapp -run 'TestPluginExecutor|TestResetTaskEnvironment|TestTaskEnvironmentRecoveryClaim' -count=1)
```

The PostgreSQL case ran against a disposable local PostgreSQL 16 container. SQLite, lifecycle, task service, and backend reset coverage passed with the additional cleanup-claim checks. No database migration was required.

Review remediation: normal `GetOrEnsureExecution` reuse now loads the stopped session's durable provider
identity and attaches the recorded resource. It retains the original runtime identity and auth token,
preserves conversation state, and never provisions a replacement implicitly. Inventory updates now compare
the caller-observed envelope revision, reject illegal phase changes, and merge the envelope into the latest
metadata snapshot. A barrier-controlled inspection/cleanup test proves that a stale ready observation cannot
restore a resource after cleanup records absence.
