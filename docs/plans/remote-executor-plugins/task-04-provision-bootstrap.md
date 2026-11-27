---
id: "04-provision-bootstrap"
title: "Provision and bootstrap one session environment"
status: done
wave: 4
depends_on:
  - "02-authenticated-transport"
  - "03-profile-admission"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-EXECUTORS-PLUGIN-004
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-002.1
  - AC-EXECUTORS-PLUGIN-002.3
  - AC-EXECUTORS-PLUGIN-002.4
  - AC-EXECUTORS-PLUGIN-004.3
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 04: Provision and bootstrap one session environment

## Summary

Implement the generic backend adapter and provisional inventory. Prove one launch reaches authenticated agentctl with a fake provider and records enough state to recover every partial launch.

## In scope

- Persist allocating before provider calls. Add narrow execution-CAS checkpoint methods and a typed, bounded plugin_executor metadata envelope.
- Bridge operation-bound checkpoint/progress callbacks and artifact streaming to AgentctlResolver. Fence callbacks by installation, dispatch generation, execution and environment ownership.
- Perform bounded bootstrap callback probe, agentctl handshake/readiness, existing repository materialization and core agent start. Roll back with a fresh cleanup context after cancellation.

## Out of scope

- Startup reattachment, plugin uninstall policy, production cloud provider.

## Acceptance

- Duplicate launch requests use the same operation identity; crash injection at allocation, checkpoint, bootstrap and post-create failure leaves confirmed cleanup or retained inventory.
- A fixture runtime uses the selected artifact digest, scoped bootstrap and normal agentctl setup; no credential is written to checkpoint state or logs.
- SQLite and real PostgreSQL tests reject stale checkpoints and preserve resume-token fields under concurrent updates.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/plugins/... ./internal/task/repository/sqlite -run 'TestPluginExecutor' -count=1)
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL DSN}" && go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorPostgres' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
```

Required evidence:

- `internal/agent/runtime/lifecycle/executor_plugin_launch_test.go: TestPluginExecutorLaunch, TestPluginExecutorPartialLaunch, TestPluginExecutorBootstrapSecrets`
- `internal/task/repository/sqlite/executor_plugin_inventory_test.go: TestPluginExecutorInventoryCAS`
- `internal/task/repository/sqlite/executor_plugin_inventory_postgres_test.go: TestPluginExecutorPostgresInventoryCAS`

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/ (new executor_plugin.go, executor_plugin_launch_test.go)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go and persistence.go`
- `apps/backend/internal/agent/runtime/lifecycle/agentctl_resolver.go and workspace_materialization.go`
- `apps/backend/internal/task/repository/sqlite/ (new executor_plugin_inventory.go, executor_plugin_inventory_test.go, executor_plugin_inventory_postgres_test.go)`
- `apps/backend/internal/plugins/ (new host_executor.go and host_executor_test.go)`
- `apps/backend/cmd/kandev/ composition wiring`

## Dependencies

[Task 02](task-02-authenticated-transport.md), [Task 03](task-03-profile-admission.md).

## Risks

No DDL is planned. If implementation requires a schema change, update this package before coding a migration. Unknown allocation outcomes must not be overwritten.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Completed. The generic lifecycle backend now provisions one isolated provider environment, records the
allocation intent and provider resource in bounded `plugin_executor` inventory, stages host-resolved
agentctl artifacts through an operation-bound callback, performs the bootstrap handshake/readiness
probe, and rolls back with a fresh cleanup context. Host callbacks validate plugin installation,
dispatch generation, execution identity, session ownership, and task-environment ownership generation.
Inventory updates preserve resume-token columns and reject stale executions and ownership generations.
Profile cleartext secrets remain transient; inventory stores only vault references, and the input digest
does not incorporate secret values.

Required verification passed:

- `go test -race ./internal/agent/runtime/lifecycle ./internal/plugins/... ./internal/task/repository/sqlite -run 'TestPluginExecutor' -count=1`
- `go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorPostgres' -count=1` against a disposable loopback PostgreSQL 16 instance
- `go run ./cmd/sqlguard ./internal`
- `go test -race ./internal/persistence/storeconformance -count=1`

The PostgreSQL test exercises concurrent inventory and resume-token updates plus stale execution and
ownership-generation rejection. Focused non-race tests also passed for the lifecycle, plugin, task
service, and SQLite repository packages.

Review remediation: recovery now handles durable `artifact_staging`, `bootstrapping`, and `provisioned`
checkpoints. With a persisted handshake token it inspects and attaches the exact resource; without a token
it runs idempotent cleanup, marks confirmed absence, and retains `cleanup_pending` when deletion is unknown.
Tests cover every checkpoint phase before and after handshake-token persistence.
