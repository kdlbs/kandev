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
  - AC-EXECUTORS-NATIVE-WORKER-006.8
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 03: Destroy- and recover-scoped checkpoints (R4)

## Summary

Let a provider checkpoint during cleanup and recovery under the existing claims, with claim, generation, dispatch, and revision fencing and monotonic phases.

## Size

M: ~250 source, ~500 test lines.

## In scope

- Use the claim's operation ID (already sent as `cleanup_claim`) to bind callbacks; add the additive `sequence` field; one advisory namespace `plugin_inventory:` for every envelope write, with `plugin_workspace:` taken first where both apply.
- Let a provider checkpoint during cleanup and recovery under the existing claims, with claim, generation, dispatch, and revision fencing and monotonic phases.

## Out of scope

- Drain receipt writer; bootstrap secrets.

## Acceptance

- A callback from a released claim, replaced generation, or superseded dispatch changes nothing.
- Two callbacks at one revision yield one commit on SQLite and PostgreSQL.

## Failing-first tests

- A callback carrying the cleanup claim's operation ID and a stale or equal `sequence`: stale rejected, equal with an equal digest an idempotent success, equal with a different digest `conflict`; a cleanup claim writing `recovery_progress` is rejected; malformed phase or missing sequence is `invalid_config`. No claim token is added.
- Delayed writer after a successor claim is rejected (fails: callbacks only match an active launch).
- Two checkpoints at one revision: one winner, one `conflict`, SQLite and PostgreSQL under -race.
- Phase regression rejected.
- Settlement requires confirmed absence and the latest checkpoint; a missing checkpoint is not absence.
- Crash between side effect and checkpoint: destroy re-issued idempotently from recorded state (unit with fake provider; the process-level proof lands in task 07).

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/task/repository/sqlite -run PluginExecutor -count=1)
```

```bash
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run PluginExecutor -count=1)   # PostgreSQL tests run when KANDEV_TEST_POSTGRES_DSN is exported
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_callbacks.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_recovery.go`
- `internal/task/repository/sqlite/executor_plugin_inventory.go`
- `internal/task/recoveryclaim`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
