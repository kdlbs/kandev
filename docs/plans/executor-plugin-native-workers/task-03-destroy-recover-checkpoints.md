---
id: "03-destroy-recover-checkpoints"
title: "Destroy- and recover-scoped checkpoints (R4)"
status: pending
wave: 2
depends_on:
  - "02"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-006
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-006.1
  - AC-EXECUTORS-NATIVE-WORKER-006.2
  - AC-EXECUTORS-NATIVE-WORKER-006.3
  - AC-EXECUTORS-NATIVE-WORKER-006.4
  - AC-EXECUTORS-NATIVE-WORKER-006.5
  - AC-EXECUTORS-NATIVE-WORKER-006.6
  - AC-EXECUTORS-NATIVE-WORKER-006.7
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 03: Destroy- and recover-scoped checkpoints (R4)

## Summary

Let a provider checkpoint during cleanup and recovery under the existing claims, with claim, generation, dispatch, and revision fencing and monotonic phases.

## Size

M: ~250 source, ~500 test lines.

## In scope

- Let a provider checkpoint during cleanup and recovery under the existing claims, with claim, generation, dispatch, and revision fencing and monotonic phases.

## Out of scope

- Drain receipt writer; bootstrap secrets.

## Acceptance

- A callback from a released claim, replaced generation, or superseded dispatch changes nothing.
- Two callbacks at one revision yield one commit on SQLite and PostgreSQL.

## Failing-first tests

- Delayed writer after a successor claim is rejected (fails: callbacks only match an active launch).
- Two checkpoints at one revision: one winner, one `conflict`, SQLite and PostgreSQL under -race.
- Phase regression rejected.
- Settlement requires confirmed absence and the latest checkpoint; a missing checkpoint is not absence.
- Crash between side effect and checkpoint: destroy re-issued idempotently from recorded state (unit with fake provider; the process-level proof lands in task 07).

## Verification

Run from the repository root:

```bash
go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/task/repository/sqlite -run PluginExecutor -count=1)
```

```bash
KANDEV_TEST_POSTGRES_DSN=<disposable dsn> go test -race ./internal/task/repository/sqlite -run PluginExecutor -count=1  # from apps/backend
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_callbacks.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_recovery.go`
- `internal/task/repository/sqlite/executor_plugin_inventory.go`
- `internal/task/recoveryclaim`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
