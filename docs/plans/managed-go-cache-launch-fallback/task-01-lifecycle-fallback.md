---
id: "01-lifecycle-fallback"
title: "Lifecycle fallback and execution state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-006
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.8
system_design:
  - ../../specs/system-page/system-design/managed-go-cache-launch-fallback.md
---

# Task 01: Lifecycle fallback and execution state

## Summary

Allow host-local creation after optional cache preparation fails.
Preserve ordinary environment resolution, caller cancellation, and one cache decision across each execution.
Remove stale managed-only state during recovery without editing saved settings.

## In scope

- Change `prepareManagedGoCacheEnvironment` according to the design's preparation boundary.
- Apply managed override during final composition. Preserve request `Env` input, including an independent value equal to an old managed path.
- Move new cache selection after the existing-session lookup in `launchInternal`.
- Keep workspace promotion and coalesced joins on their established execution environment.
- Remove copied cache metadata in `prepareExecutionCreateRequest` before adding the current result.
- Add fixed warning fields with reasons `preparation_failed` and `invalid_output`.
- Cover legacy, strict, and production-finalized environment consumers.

## Out of scope

Provider safety relaxations, persistence changes, host cache repair, new flags, and UI changes.

## Acceptance

1. Real adopted-root and ancestor symlinks permit new execution creation with no managed override. Deterministic settings/filesystem errors and relative provider output use the same fallback.
2. Valid, disabled, remote, recovery, promotion, repeated-request, and coalesced-join cases preserve their environment contract. Recovered shell/build environments cannot restore rejected managed metadata.
3. Caller and wrapped provider cancellation/deadline errors prevent creation. Skip warnings contain fixed bounded text and no raw errors or sensitive values.

## Planned TDD sequence (completed)

The implementation followed this sequence. Actual regression coverage and command outcomes are recorded in Results.

First add `TestManagedGoCacheFallbackAdoptedRootSymlink` with a real `gocache.Provider` and a temporary sentinel target.
Record its expected red error from current lifecycle behavior before any production edit.
Then add the remaining plan test targets in focused files.

Use a provider fixture returning `errors.New` or `os.PathError` for deterministic failure.
Use a non-directory parent blocker for a real filesystem failure. Do not rely on `chmod` under root.
For disabled management, use the real provider with `GoCache.Enabled=false`.
For non-host execution, use a counted provider and independently configured executor-local cache.
For cancellation, cover pre-cancelled and expired contexts, wrapped provider errors, and cancellation while the provider returns success or nil.
Coordinate mid-call cancellation with channels. Assert that the executor creation effect is absent, with a matching successful positive control.

Exercise successful final composition rather than preserving helper-level eager input mutation in existing tests.
Retain `TestManagedGoCacheEnvironmentPropagatesToPrepareAndRuntime` and process-runner assertions.
For same-request reuse, change provider state from valid to failing and prove the independent request value survives.
For promotion and coalesced creation, change provider state after workspace creation and prove the provider is not consulted again.
Use copied old recovery metadata and assert `processEnvironment` honors the independent requested environment after fallback.
For warnings, inject oversized errors containing a token, URL, absolute path, and newline. Assert all are absent from message and fields.

## Verification

Run from repository root:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -run 'ManagedGoCache|ExecutionEnvironment|CleanupRejects|ValidateAdoption' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'ManagedGoCache' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -count=1)
git diff --check
```

## Results

Implemented the lifecycle fallback and execution-level cache decision. The adopted-root symlink, request-environment preservation, and strict-environment composition tests failed before the production changes.

Validation passed:

- `go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -run 'ManagedGoCache|ExecutionEnvironment|CleanupRejects|ValidateAdoption' -count=1`
- `go test -race ./internal/agent/runtime/lifecycle -run 'ManagedGoCache' -count=1`
- `go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -count=1`
- `git diff --check`

PR review follow-up (2026-10-02): fallback warnings now include `task_id` and `session_id` when available, and the bounded-warning regression asserts both fields. The disabled, absent-provider, and remote-executor cases now assert their expected provider-call counts.

Validation passed:

- `go test ./internal/agent/runtime/lifecycle -run '^Test(ManagedGoCacheFallbackWarningIsBounded|ManagedGoCacheDisabledAbsentAndRemote)$' -count=1`
- `go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -run 'ManagedGoCache|ExecutionEnvironment|CleanupRejects|ValidateAdoption' -count=1`
- `go test -race ./internal/agent/runtime/lifecycle -run 'ManagedGoCache' -count=1`

## Files touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go` (adjust existing assertions only)
- New `apps/backend/internal/agent/runtime/lifecycle/manager_managed_go_cache_test.go`

No changes were needed in `types.go`, `process_runner.go`, or existing process-runner tests.
Keep new test files within the 800-effective-line limit.

## Dependencies

None.

## Risks

Moving the selection point can affect promotion and duplicate-launch behavior. Retain their existing admission and singleflight contracts.
An inherited default cache can still fail in a later tool process.
Do not treat the caller's independent cache value as managed-only state.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md), active requirement 006.
- [Design](../../specs/system-page/system-design/managed-go-cache-launch-fallback.md).
- Existing static cache fixture and launch/environment-preparer tests in `manager_launch_test.go`.
- Existing recovery environment fixture in `manager_execution_test.go`.
- [Decision](../../decisions/2026-10-01-optional-managed-go-cache.md).
