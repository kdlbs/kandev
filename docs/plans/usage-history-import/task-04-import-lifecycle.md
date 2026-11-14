---
id: "04-import-lifecycle"
title: "Add import queue and restart recovery"
status: done
wave: 3
depends_on:
  - "03-import-engine"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-004
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-004.1
  - AC-COSTS-USAGE-IMPORT-004.2
  - AC-COSTS-USAGE-IMPORT-004.3
  - AC-COSTS-USAGE-IMPORT-004.4
  - AC-COSTS-USAGE-IMPORT-004.5
  - AC-COSTS-USAGE-IMPORT-004.6
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 04: Add import queue and restart recovery

## Summary

Make a long import survive restarts and stop workspaces interfering with each other. This fixes two reproduced defects: an import stays `running` after a restart with no worker, and starting a second workspace cancels the first.

## In scope

- The persisted `import-queue`, the single active-run record, and startup resume from `SetHost`.
- Separating shutdown from user cancellation.
- One deferred `settle` for every worker exit, and the three-attempt failure rule with Resume.

## Out of scope

- Running imports for several workspaces at once.

## Acceptance

- After a simulated restart, a `running` import resumes from `NextDay` without a user action and repeats no completed day.
- A second workspace's start leaves the first running and returns `queued`, and the second runs when the first finishes.
- Injected state and report failures never leave `running` or `preparing` behind, except on process exit.

## Verification

```bash
(cd "$PLUGIN_ROOT" && go test ./server/ -run 'TestImportLifecycle' -count=1)
(cd "$PLUGIN_ROOT" && go test -race ./server/ -run 'TestImportLifecycle|TestImportEngine|TestReviewImport' -count=20)
```

## Files likely touched

- `server/import.go`
- `server/plugin.go`
- `server/responsive.go`
- `server/usage_scope.go`
- `server/import_lifecycle_test.go` (new)

## Dependencies

Task 03.

## Risks

- `SetHost` returns early when state cleanup needs a retry. Resume must also start after `startUsageStateCleanup` succeeds.
- `Close` runs before `os.Exit`, so the deferred `settle` must check the shutdown flag first.
- Queue entries can outlive workspace access. `authorizedHistoricalImportState` must run before resuming each entry.
- Include the deterministic composer-waiter cancellation test from the design's Shared blocking work section.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/costs/system-design/usage-history-import.md), Worker lifecycle.
- `server/report.go`: `reportCoordinator.wait`.

## Results

Added the plugin-scope start-order queue and one active-run record.
A restart resumes running, preparing, or queued imports after Host state cleanup; `Close()` no longer writes a terminal status.
Every worker exit settles through one deferred path, three consecutive day failures stop with a resumable failed status, and disabling collection keeps the checkpoint with a disabled status.
The composer-waiter cancellation test waits for the composer to join before cancelling and confirms the shared report finishes for the remaining waiter. A failed config read retries instead of settling disabled, and a refused start leaves completed or cancelled checkpoints unchanged.

Validation: plugin `go test ./server/ -run TestImportLifecycle -count=1` and `go test -trimpath -race ./...` passed, including restart, queue, shutdown, disabled-resume, config-retry, and composer-waiter cases.
