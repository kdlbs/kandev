---
id: "01-demote-background-canceled-logs"
title: "Demote background context-cancellation logs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001
acceptance_criteria:
  - AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.1
  - AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.2
  - AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.3
  - AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.4
system_design:
  - ../../specs/platform/system-design/shutdown-background-canceled-logs.md
---

# Task 01: Demote background context-cancellation logs

## Summary

Demote the expected `context.Canceled` failures emitted during graceful
shutdown by the GitHub PR-watch service, the agent profile reconciler, and the
host-utility profile migration from `ERROR`/`WARN` to `DEBUG`, while keeping all
other errors at their original severity.

## In scope

- Add `github.Service.logSyncError` and route the single and batched PR-watch
  sync store-error sites through it.
- Add `settings/controller.ProfileReconciler.logReconcileError` and route the
  reconcile, orphan-cleanup, and profile-heal store-error sites through it.
- Classify the `backendapp` utility profile migration error inline with the same
  `context.Canceled` check.

## Out of scope

- The session, orchestrator, plugin, and launcher shutdown-log-noise sites.
- Any control-flow, returned-error, retry, or lifecycle change.

## Acceptance

- PR-watch sync failures wrapping `context.Canceled` log at `DEBUG`, not
  `ERROR`; reconciler and utility-migration failures wrapping `context.Canceled`
  log at `DEBUG`, not `WARN`.
- Any error that does not wrap `context.Canceled`, including
  `context.DeadlineExceeded`, keeps its original `ERROR`/`WARN` level.

## Verification

```bash
cd apps/backend && go build ./...
cd apps/backend && go test ./internal/github/ -run 'TestServiceLogSyncError|TestPollerLogCleanupError' -count=1
cd apps/backend && go test ./internal/agent/settings/controller/ -run 'TestProfileReconcilerLogReconcileError' -count=1
cd apps/backend && go vet ./internal/github/... ./internal/agent/settings/controller/... ./internal/backendapp/...
```

## Files likely touched

- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/agent/settings/controller/reconciler.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/github/service_pr_watch_sync_log_test.go`
- `apps/backend/internal/agent/settings/controller/reconciler_log_test.go`

## Dependencies

None.

## Risks

- The demoted branch must trigger only for `context.Canceled` so real faults,
  including `context.DeadlineExceeded`, stay visible. The unit tests assert the
  level selection explicitly.

## Parallelism

`sequential`

## Inputs

- `docs/specs/platform/requirements/shutdown-background-canceled-logs.md`,
  AC `.1` through `.4`.
- `docs/specs/platform/system-design/shutdown-background-canceled-logs.md`.
- The existing `github.Poller.logCleanupError` precedent and its
  `poller_cleanup_log_test.go` test pattern.

## Results

Added `logSyncError` and `logReconcileError` mirroring `logCleanupError`, routed
the PR-watch single and batched sync sites and the reconcile, orphan-cleanup,
and profile-heal sites through them, and classified the utility profile
migration error inline. Genuine faults, including `context.DeadlineExceeded`,
retain their original severity.

Verification:

- `go build ./...` passed.
- `go test ./internal/github/ -run 'TestServiceLogSyncError|TestPollerLogCleanupError' -count=1` passed.
- `go test ./internal/agent/settings/controller/ -run 'TestProfileReconcilerLogReconcileError' -count=1` passed.
- `go test ./internal/github/... -count=1` passed.
- `go vet ./internal/github/... ./internal/agent/settings/controller/... ./internal/backendapp/...` passed.
- The pre-existing `TestHostRuntimeUpdaterInvalidatesOnlyManagedNPMExecutionTree`
  failure reproduces with these changes reverted and is unrelated.
