---
id: "01-repair-run-terminalization"
title: "Repair automation run terminalization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-CONTINUITY-003
  - REQ-OFFICE-AUTOMATION-TARGETS-002
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-CONTINUITY-003.3
  - AC-OFFICE-AUTOMATION-CONTINUITY-003.4
  - AC-OFFICE-AUTOMATION-TARGETS-002.4
system_design:
  - ../../specs/office/system-design/automation-runs.md
---

# Task 01: Repair Automation Run Terminalization

## Summary

Make exact-run stop succeed for an already-loaded open run when its bound turn is stale. Make
restart reconciliation treat typed missing-execution errors as definitive absence. Use strict
red-green-refactor and keep transient errors retryable.

## In scope

- Add table-driven service regressions for stop and reconciliation state transitions.
- Add orchestrator boundary regressions for all required typed gone-execution errors.
- Correct `Service.StopRun` and `Service.AutomationRunLive` with the minimum behavior change.
- Make a completion race after the open-row check idempotent.
- Normalize deleted task/session repository bindings at the orchestrator boundary.
- Format changed Go files and run the requested backend test and lint gates.
- Create a verified `fix:` Conventional Commit without bypassing hooks.

## Out of scope

- Frontend or protocol changes.
- Database migration or direct repair of existing user data.
- String matching for missing executions.
- Changes to automation concurrency policy or run-status vocabulary.

## Acceptance

- A `triggered` or `task_created` run owned by the requested automation becomes failed with the
  stop reason. The call returns success when the stopper returns either `true` or `false`. A real
  miss, mismatch, or non-open run remains not-found. A hard error leaves the row open. If normal
  completion wins the terminal-write race, the terminal result is returned as success.
- Wrapped runtime, executor, lifecycle, and SQL not-found sentinels make `AutomationRunLive` return
  not live without error. Deleted task and session repository sentinels have the same result.
  Reconciliation then fails the exact run. A transient error preserves it.
- A live run remains open, an explicit not-live run settles, and an unbound admitted run settles
  through its existing recovery reason.

## Verification

```bash
go test -tags fts5 ./internal/automation -run 'Test(StopRun|ReconcileOpenRuns)$'
go test -tags fts5 ./internal/orchestrator -run '^TestAutomationRunLive'
make test
make lint
golangci-lint run ./... --new-from-rev=c2e2b3a430d0bd865a3d0bc159e9d8543bc1b02d
git diff --check
```

Run every command from `apps/backend`. Before the verification, run `gofmt -w` on every changed Go
file. After the verification passes, use the repository `/commit` workflow with a `fix:` header.
Do not use `--no-verify`.

## Files likely touched

- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/service_run_terminal_test.go`
- `apps/backend/internal/agent/runtime/facade.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/orchestrator/automation_run_liveness_test.go`

## Dependencies

None.

## Risks

- Incorrectly broad missing-execution classification can terminalize a run during a transient
  outage.
- Stop must preserve exact-run isolation when a shared session already has a successor turn.

## Parallelism

`sequential`

## Inputs

- `AC-OFFICE-AUTOMATION-CONTINUITY-003.3` and `.4`.
- `AC-OFFICE-AUTOMATION-TARGETS-002.4`.
- `docs/specs/office/system-design/automation-runs.md` API and failure behavior.
- The reported read-only runtime evidence and current `StopRun`, `ReconcileOpenRuns`,
  `StopAutomationRun`, and `AutomationRunLive` implementations.

## Results

- RED: `TestStopRun/stale_open_turn_settles` failed because `StopRun` returned
  `ErrAutomationNotFound`.
- RED: the runtime, executor, and lifecycle liveness cases failed because
  `AutomationRunLive` returned each wrapped error.
- GREEN: `StopRun` now settles an already-loaded open run after both true and false stop results.
- GREEN: `AutomationRunLive` now maps the four typed gone-execution errors to not live without an
  error. Other errors remain retryable.
- The focused automation command passed 13 cases. The focused orchestrator command passed 6 cases.
- The full backend test target passed with the task-runtime configuration variables removed.
- Both lint commands passed with zero issues.
- The architecture hook rejected direct lifecycle imports in the orchestrator layers. The runtime
  package now owns lifecycle-sentinel recognition.
- PR fixup review remediation added a concurrent completion regression and real-repository deleted
  task/session normalization coverage. Focused tests and formatting passed after the remediation.
