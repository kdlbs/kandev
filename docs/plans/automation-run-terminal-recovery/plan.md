---
created: 2026-08-27
status: completed
requirements:
  - REQ-OFFICE-AUTOMATION-CONTINUITY-003
  - REQ-OFFICE-AUTOMATION-TARGETS-002
system_design:
  - ../../specs/office/system-design/automation-runs.md
legacy_specs: []
---

# Implementation Plan: Automation Run Terminal Recovery

## Overview

Repair exact-run stopping and restart reconciliation. A stale runtime binding must not leave an
open automation run that consumes its concurrency slot. First add focused service and orchestrator
regressions. Then make the smallest boundary corrections. Run the requested backend verification
before the `fix:` Conventional Commit.

## Scope

### In scope

- Terminalize a genuinely open run when its exact-turn stopper reports that no active turn remains.
- Treat a concurrent normal completion after the open-row check as an idempotent stop success.
- Preserve not-found behavior for a missing, foreign, or already terminal run.
- Normalize typed gone-execution liveness errors to not live at the orchestrator boundary.
- Normalize typed task and session repository disappearance at the same boundary.
- Preserve open runs when liveness inspection fails with a transient error.
- Cover exact stop and recovery outcomes with table-driven Go tests.

### Out of scope

- Changing automation admission, concurrency limits, run statuses, API payloads, or frontend copy.
- Cancelling a successor turn when a stored exact-turn binding is stale.
- Treating untyped error strings as missing executions.
- Repairing or deleting the supplied local database row.

## Technical approach

### Exact-run stop

Update `automation.Service.StopRun` in `apps/backend/internal/automation/service.go`. Keep the loaded
open row as the authority after automation ownership and status validation. If
`RunStopper.StopAutomationRun` returns either `true` or `false` without an error, settle that exact
row. Use `Store.MarkRunTerminal(..., RunStatusFailed, "stopped by user")`, and return success. Return
`ErrAutomationNotFound` only for a missing row, automation mismatch, or non-open status. Propagate a
stopper error without settling the row. If the terminal write loses a race to normal completion,
reload the row and return its terminal state as an idempotent success; propagate the write error only
when the row is still open or cannot be read.

Remove or correct the unused `ErrAutomationRunNotLive` documentation because stale exact-turn state
no longer maps an already-loaded open run to not-found.

### Restart liveness

Update `orchestrator.Service.AutomationRunLive` in
`apps/backend/internal/orchestrator/event_handlers_automation.go` so its public boundary maps a
wrapped `runtime.ErrNotFound`, `executor.ErrExecutionNotFound`,
`lifecycle.ErrExecutionNotFound`, `sql.ErrNoRows`, task-repository task-not-found, or task-session
not-found to `(false, nil)` with `errors.Is`. Keep every other error unchanged. The runtime package
owns lifecycle-sentinel recognition. The orchestrator owns runtime, executor, repository, and SQL
classification. `automation.Service.ReconcileOpenRuns` keeps its conservative rule.
`live=false, err=nil` settles the run. A transient error leaves it open for retry. Do not add
runtime, executor, lifecycle, repository, or SQL imports to `internal/automation`.

## Tests

- `AC-OFFICE-AUTOMATION-CONTINUITY-003.3` and
  `AC-OFFICE-AUTOMATION-TARGETS-002.4`: add table-driven `TestStopRun` coverage for a successful
  active stop and a false-on-open stale stop. Cover real miss, mismatch, non-open state, and a hard
  stopper error, plus a concurrent completion race. Assert the stored row and returned error, not
  only fake calls.
- `AC-OFFICE-AUTOMATION-CONTINUITY-003.4`: add table-driven `TestReconcileOpenRuns` coverage for a
  normalized gone execution and a transient liveness error. Cover `live=false`, `live=true`, and an
  unbound admitted run. Assert terminal or preserved store state.
- Add orchestrator table coverage that injects each required wrapped sentinel through
  `AutomationRunLive`, proves `(false, nil)`, proves task/session deletion is normalized with a real
  repository, and proves a transient error still propagates.

New tests go in new files because the neighboring service and automation-handler test files already
exceed or approach the backend effective-line limit.

## Work orders

- [x] [Task 01: Repair automation run terminalization](task-01-repair-run-terminalization.md)

## Verification results

- The focused automation test command passed 13 cases.
- The focused orchestrator test command passed 6 cases.
- `make test` passed after removal of three task-runtime configuration variables from the test
  process. The first run exposed only configuration-discovery fixture conflicts.
- `make lint` passed with zero issues.
- `golangci-lint run ./... --new-from-rev=c2e2b3a430d0bd865a3d0bc159e9d8543bc1b02d`
  passed with zero issues.
- The specification linter and documentation diff validation passed.
- PR fixup added idempotent concurrent-stop handling and task/session repository not-found
  normalization, with focused regressions passing.

## Risks

- Settling before validation of the stored automation ID or open status can hide a real miss or alter
  another automation's run.
- Broad error normalization can turn a transient database or runtime error into a false stale
  result. Classification must use only the six typed runtime, executor, SQL, task, and session
  sentinels.
- A stale binding must never cause cancellation of a newer turn in the shared session.
