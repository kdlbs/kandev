---
id: "01-settle-registration-before-runtime-check"
title: "Settle registration before runtime comparison"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-EXECUTORS-RUNTIME-SELECTION-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.10
  - AC-EXECUTORS-RUNTIME-SELECTION-002.2
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
  - ../../specs/executors/system-design/runtime-selection.md
---

# Task 01: Settle registration before runtime comparison

## Summary

Make `Manager.ValidateExecutionRuntime` wait until a newly created execution's
registration has been persisted or rolled back before comparing it with the
retained `executors_running` row.

## Regression test (write first, must fail)

New file `apps/backend/internal/agent/runtime/lifecycle/manager_runtime_selection_registration_test.go`
(`manager_runtime_selection_test.go` is already 521 lines):

- `TestValidateExecutionRuntimeWaitsForRegistrationSettle`: real temporary
  SQLite inventory with a standalone row naming predecessor `exec-old`. Drive
  `registerAndPublishCreatedExecution` for `exec-new` with a running-writer
  wrapper that blocks inside `persistExecutorRunningResult` on a channel
  barrier. While blocked, call `ValidateExecutionRuntime(..., "local_pc")` in a
  goroutine. Today it returns `retained execution identity does not match`.
  After the fix it stays pending until the barrier releases, then returns nil.
- `TestValidateExecutionRuntimeAfterRegistrationRollback`: the same barrier,
  but persistence fails; the validator must fall through to the durable-row
  path for the predecessor, not compare the rolled-back execution.
- `TestValidateExecutionRuntimeRegistrationWaitHonorsContext`: barrier never
  released; a cancelled context returns the context error, not a mismatch.
- `TestValidateExecutionRuntimeWaitsForSuccessorRegistration`: replacing the
  settled execution with an unsettled successor makes validation wait for the
  successor before checking its runtime identity.
- `TestValidateExecutionRuntimeSettledMismatchStillRefused`: settled
  registration whose row names another execution still returns the
  identity-unverified error.

## Implementation

1. `AgentExecution` (`types.go`): add `registrationSettled chan struct{}` plus a
   `settleRegistration()` helper guarded by `sync.Once`, and
   `waitRegistrationSettled(ctx) error` that returns immediately when the
   channel is nil.
2. `initializeCreatedExecution` (`manager_execution.go`): allocate the channel before
   `executionStore.Add`.
3. `registerAndPublishCreatedExecution`: `defer execution.settleRegistration()`
   so every exit (race-lost Add, secret or row persistence failure, admission
   failure, publish failure, success) closes it after the row write or rollback.
4. `ValidateExecutionRuntime` (`manager_runtime_selection.go`): after
   `GetBySessionID` returns an execution, wait on its signal with `ctx`; then
   re-read the store and continue with whatever is current (nil goes to
   `validateDurableExecutionRuntime`). Do not lock `remoteInstanceLifecycleMu`.
5. Keep functions within lint limits; extract a small helper for the wait and
   re-read if needed.

## In scope

- `lifecycle/types.go` (`AgentExecution`),
  `lifecycle/manager_execution.go`, `lifecycle/manager_runtime_selection.go`,
  and the new test file.

## Out of scope

- Orchestrator, frontend, startup behavior, and predicate changes.

## Acceptance

- The five tests pass with `-race`; the first fails on current `main` for the
  expected reason.
- Existing lifecycle and orchestrator executor tests pass unchanged.

## Execution result

- RED confirmed: the registration-settle, rollback, and context tests failed
  before the fix. The first returned `retained execution identity does not
  match` while the inventory write was held.
- PASS: `go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'RegistrationSettle|ValidateExecutionRuntime' -count=1`
- PASS: `go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle/ ./internal/orchestrator/executor/ -count=1`
- PASS: changed-Go lint, including the new test file (`0 issues`).
- The package command without `-tags fts5` fails in an existing SQLite schema
  migration test with `no such module: fts5`; the documented command uses the
  repository's required tag.
- Review fixes: runtime validation now waits again if a successor execution
  replaces the registration it first observed. The regression test synchronizes
  on each registration wait instead of relying on a timing delay. The successor
  replacement regression failed with the old one-shot wait.

## Commands

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'RegistrationSettle|ValidateExecutionRuntime' -count=1
go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle/ ./internal/orchestrator/executor/ -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```
