---
created: 2026-09-16
status: done
requirements:
  - REQ-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001
system_design:
  - ../../specs/platform/system-design/shutdown-background-canceled-logs.md
legacy_specs: []
---

# Implementation Plan: Background subsystem context-cancellation log severity

## Overview

A 2026-09-16 shutdown log audit found that a clean backend shutdown printed 28
`ERROR` lines and several `WARN` lines, all `context canceled`, even though the
supervisor reported `error_count: 0`. The lines came from three background
subsystems whose in-flight work is bound to the root context: the GitHub
PR-watch service, the agent profile reconciler, and the host-utility profile
migration. This package demotes those expected teardown cancellations to
`DEBUG` while leaving genuine faults at their original severity. It is a single
work order because the change is one cohesive log-severity classification with a
shared verification boundary.

## Scope

### In scope

- Demote PR-watch single and batched sync failures that wrap `context.Canceled`
  from `ERROR` to `DEBUG` during shutdown.
- Demote agent profile reconciler store failures that wrap `context.Canceled`
  from `WARN` to `DEBUG` during shutdown.
- Demote the host-utility profile migration failure that wraps
  `context.Canceled` from `WARN` to `DEBUG` during shutdown.
- Keep every other error, including `context.DeadlineExceeded`, at its original
  severity.

### Out of scope

- The session-prompt, orchestrator-launch, go-plugin, and launcher-signal sites
  owned by the separate shutdown-log-noise package.
- Any control-flow, returned-error, retry, or lifecycle change.
- Any non-shutdown log-severity review.

## Technical approach

Follow the existing `github.Poller.logCleanupError` precedent. Add a
`Service.logSyncError` helper in the PR-watch service and a
`ProfileReconciler.logReconcileError` helper in the settings controller; each
appends `zap.Error(err)`, records `DEBUG` when `errors.Is(err, context.Canceled)`
matches, and otherwise records the site's original level (`ERROR` for PR-watch,
`WARN` for the reconciler). Route the single and batched PR-watch sync sites and
the reconcile, orphan-cleanup, and profile-heal sites through their helper. In
`backendapp`, classify the utility profile migration error inline with the same
`context.Canceled` check.

## Tests

- `AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.1` and `.4`: unit coverage
  in `apps/backend/internal/github/service_pr_watch_sync_log_test.go` for
  `logSyncError` level selection (generic and `context.DeadlineExceeded` stay
  `ERROR`; `context.Canceled` and a wrapped variant become `DEBUG`) and field
  forwarding.
- `AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.2` and `.4`: unit coverage
  in `apps/backend/internal/agent/settings/controller/reconciler_log_test.go`
  for `logReconcileError` level selection and field forwarding.
- `AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.3`: covered by the same
  inline `context.Canceled` check in `backendapp/main.go`, verified by build and
  package vet alongside the helper tests.

## E2E tests

The contract is backend log severity emitted only during process shutdown, not
browser behavior, so a Playwright project cannot observe it. Backend unit tests
using an in-memory zap observer are the correct boundary. No browser E2E test is
needed for these acceptance criteria.

## Work orders

- [x] [Task 01: Demote background context-cancellation logs](task-01-demote-background-canceled-logs.md) (done)

## Verification results

- `go build ./...` passed.
- `go test ./internal/github/ -run 'TestServiceLogSyncError|TestPollerLogCleanupError' -count=1` passed.
- `go test ./internal/agent/settings/controller/ -run 'TestProfileReconcilerLogReconcileError' -count=1` passed.
- `go test ./internal/github/... -count=1` passed.
- `go vet ./internal/github/... ./internal/agent/settings/controller/... ./internal/backendapp/...` passed.
- One pre-existing controller test,
  `TestHostRuntimeUpdaterInvalidatesOnlyManagedNPMExecutionTree`, fails
  identically with these changes reverted (`open npm cache root: not a
  directory`), so it is an unrelated environment-dependent failure.

## Risks

- A demoted level must be reachable only for `context.Canceled`, so
  `context.DeadlineExceeded` and other faults stay visible. The helpers make the
  branch explicit and the unit tests assert it.
