---
id: "06-classify-startup-runtime-cleanup"
title: "Classify startup runtime cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/runtime-cleanup.md"
---

# Task 06: Classify startup runtime cleanup

## Intent

Make startup reconciliation decisions and diagnostics precise: safely remove
already-absent confirmed-dead local runtimes, preserve uncertain rows, and stop
expected legacy preservation from producing an ambiguous warning flood.

## Acceptance

- Missing-session and terminal/failed-session paths use one explicit cleanup
  decision classifier.
- Typed runtime-not-found plus confirmed-dead local liveness removes or repairs
  the row according to the resume-safety invariant.
- Alive local, Unknown local, and remote rows remain preserved.
- Generic stop errors remain preserved and produce individual structured
  warnings.
- Runtime/lifecycle/executor adapters preserve typed not-found identity through
  the real composition boundary.
- Local PID liveness after backend restart is tested for alive, dead, and
  missing-handle cases.
- Expected fail-closed outcomes are aggregated into a bounded startup warning
  summary with safe classifications and counts.
- A second reconciliation does not encounter a safely removable row removed by
  the first pass; intentionally uncertain rows remain preserved and summarized.
- Diagnostics never contain resume tokens, credentials, or provider payloads.

## TDD sequence

1. Add focused tests for missing sessions and terminal/failed sessions with a
   typed not-found result and confirmed-dead local PID. Assert cleanup and no
   repeated warning on a second pass.
2. Add alive local, missing-PID/Unknown local, remote, and generic-failure cases.
   Assert preservation and the correct diagnostic disposition.
3. Add adapter-boundary tests proving lifecycle/executor not-found sentinels
   normalize to the runtime API sentinel, and liveness tests for post-restart PID
   classification.
4. Add observer-backed tests for one aggregate expected-outcome summary and
   individual unexpected-error warnings.
5. Extract the smallest shared decision classifier and summary collector. Reuse
   them in missing and terminal/failed startup paths without changing the
   deletion safety predicate.
6. Run focused packages under `-race`, then refactor only after GREEN.

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/reconcile_liveness.go`
- `apps/backend/internal/orchestrator/reconcile_restart_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/liveness.go`
- `apps/backend/internal/agent/runtime/lifecycle/liveness_test.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/backendapp/adapters_test.go`

## Dependencies

None. The safe confirmed-dead cleanup behavior already exists; this task adds
classification consistency, integration evidence, and bounded diagnostics.

## Parallelism

`parallel-safe` with Tasks 01, 02, and 05. It owns orchestrator reconciliation
and liveness tests, not launcher/runtime availability files.

## Verification

- `cd apps/backend && go test -race -run 'TestReconcileSessionsOnStartup|TestStopReportsRuntimeAbsent|TestRowProcessLiveness' ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp`
- `cd apps/backend && golangci-lint run ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp --timeout=5m`

## Inputs

- Startup warning clusters for missing and terminal/failed sessions.
- Pre-start database evidence that warned standalone rows had `local_pid=0`.
- Existing `stopReportsRuntimeAbsent`, `rowLiveness`,
  `pruneOrRepairExecutorRow`, and runtime error-normalization paths.
- Existing startup reconciliation tests from the confirmed-dead cleanup repair.

## Output contract

Record the exact decision matrix, sentinel-normalization evidence, first/second
reconciliation outcomes, structured log fields and counts, and focused
race/lint results.

## Results

Centralized startup stop-result classification in `reconcile_liveness.go`:
typed runtime absence is cleanup-safe only with confirmed-dead local liveness;
alive, Unknown, remote, missing-handle, and generic-failure outcomes preserve
the durable row. Rows without an execution stop handle proceed only when a
local process is independently confirmed dead. Expected fail-closed
preservation is emitted as one bounded structured startup summary with
liveness, stop-error class, disposition, and local-PID-presence counts. Generic
stop failures retain individual structured warnings without raw errors, tokens,
credentials, or provider payloads.

The existing lifecycle and backend adapter boundary tests confirm real
sentinel normalization, and the liveness tests cover alive, reaped/dead, SSH,
docker, empty-runtime, nil, and missing-local-handle rows. Focused race
verification passed:

```text
go test -race -run 'TestReconcileSessionsOnStartup|TestStopReportsRuntimeAbsent|TestRowProcessLiveness|TestStopRuntimeForStartupCleanupPreservesUnknownRowsWithoutHandle' ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp
42 passed
```
