---
id: "01-run-session-foundation"
title: "Run-owned runtime foundation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.1
  - AC-OFFICE-TASKLESS-001.2
  - AC-OFFICE-TASKLESS-001.4
  - AC-OFFICE-TASKLESS-001.7
  - AC-OFFICE-TASKLESS-001.8
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 01: Run-owned runtime foundation

## Summary

Add durable Office run-session records and a typed runtime owner/start contract. Keep the scheduler on its existing behavior until Task 02 wires the complete lifecycle.

## In scope

Own portable migrations, atomic reservation/registration, owner-aware runtime admission,
and the runtime start/prompt seam. Add exact owner identity to execution/events.
Existing task callers retain their preparation and startup contracts. Add Office
interfaces for durable admission and registration without runtime importing Office.
Update scoped backend/Office AGENTS guidance for the new ownership boundary.

## Out of scope

Other work orders, unrelated refactors, live-instance changes and publication.

## Acceptance

- A fresh run session is reserved atomically, bound to the claimed run and independently queryable; retries cannot overwrite predecessor identities.
- Run-owner admission is checked around allocation/registration/start and rejects mismatch, cancellation, paused workspace and read failure; task admission remains strict.
- Runtime start dispatches one initial prompt and cleans partial allocation on failure; persistence and runtime contract tests pass on supported database backends.

## Regression evidence

Add TestRunSessionReservationCAS, TestPostgresRunSessionReservationCAS, TestRunOwnerAdmissionRejectsStaleAttempt, TestRuntimeStartDispatchesInitialPromptOnce and TestRuntimeStartRollsBackRegistrationFailure. Include duplicate reservation, cross-workspace identity and task-session regression cases.

## Verification

Run from the repository root; every command is independently rooted. New test files
named below are outputs of this work order. Record red/green evidence.

```bash
(cd apps/backend && go test ./internal/office/repository/sqlite -run 'RunSession|PostgresRunSession' -count=1)
(cd apps/backend && go test ./internal/agent/runtime ./internal/agent/runtime/lifecycle -run 'RunOwner|RuntimeStart|Runtime_Launch|LaunchSession|LaunchError|LaunchMetadata|LaunchWorkspace' -count=1)
(cd apps/backend && go test ./internal/agent/runtime ./internal/agent/runtime/lifecycle -run 'RunOwner|RuntimeStart' -race -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/office/repository/sqlite -run PostgresRunSession -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/models/`
- `apps/backend/internal/office/repository/sqlite/base.go`
- `apps/backend/internal/office/repository/sqlite/run_sessions.go (new)`
- `apps/backend/internal/office/repository/sqlite/run_sessions_test.go (new)`
- `apps/backend/internal/office/repository/sqlite/run_sessions_postgres_test.go (new)`
- `apps/backend/internal/agent/runtime/runtime.go`
- `apps/backend/internal/agent/runtime/facade.go`
- `apps/backend/internal/agent/runtime/runtime_contract_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/AGENTS.md`
- `apps/backend/internal/office/AGENTS.md`

## Dependencies

None

## Risks

Every existing task runtime consumer must retain its task invariant. Do not turn unknown ownership into permissive sessionless startup. Postgres DSN must name an isolated test database.

## Parallelism

`sequential`

## Inputs

Read linked requirements/designs in full, the plan evidence, scoped AGENTS.md,
TDD guidance and the adjacent existing tests before changing code. UI tasks also
read mobile-parity and E2E fixture guidance.

## Results

Implemented the portable `office_run_sessions` reservation table and repository
CAS, including workspace ownership validation and immutable retry attempts.
Added `runtime.Start`, owner identity/admission propagation, startup rollback,
and exact run-owner fields on lifecycle events. Existing task launches retain
their prior contracts.

Verification on 2026-09-17:

- `(cd apps/backend && GOCACHE=/tmp/kandev-go-cache go test ./internal/office/repository/sqlite -run 'RunSession|PostgresRunSession' -count=1)` passed (Postgres case skipped when `KANDEV_TEST_POSTGRES_DSN` was unset).
- `(cd apps/backend && GOCACHE=/tmp/kandev-go-cache go test ./internal/agent/runtime ./internal/agent/runtime/lifecycle -run 'RunOwner|RuntimeStart|Runtime_Launch|LaunchSession|LaunchError|LaunchMetadata|LaunchWorkspace' -count=1)` passed.
- `(cd apps/backend && GOCACHE=/tmp/kandev-go-cache go test ./internal/agent/runtime ./internal/agent/runtime/lifecycle -run 'RunOwner|RuntimeStart' -race -count=1)` passed.
- `git diff --check` passed.
