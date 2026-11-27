---
id: "01-conditional-parking"
title: "Shared ACP suspension primitive"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.2
  - AC-EXECUTORS-IDLE-PARKING-001.3
  - AC-EXECUTORS-IDLE-PARKING-001.7
  - AC-EXECUTORS-IDLE-PARKING-001.8
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 01: Shared ACP suspension primitive

## Summary

Suspend the exact idle ACP execution while preserving its conversation and task resources.
The operation uses Kandev-owned activity and generation checks, with no provider-specific quiescence gate.

## In scope

- Add a runtime facade suspension operation and immutable identity-bound outcomes.
- Persist suspension provenance and preserve the existing resume token with SQLite/PostgreSQL migration parity.
- Coordinate prompt/workspace admission and exact-process teardown using existing ownership and process-group cleanup.
- Adapt executor stop reasons to preserve task-owned containers, Pods, worktrees, and shared agentctl processes.
- Distinguish suspension outcomes from generic cancellation and ignore stale terminal events before side effects.
- Support all ACP provider families without an allowlist; check only normal restoration prerequisites.

## Out of scope

Workspace settings, expiry scheduling, and frontend recovery callers belong to Task 02.
Provider-internal API discovery and complete quiescence proofs are not required work.

## Acceptance

1. Tests with different ACP provider identities stop the correct process and preserve session, token, workspace, and task compute.
2. A concurrent admitted prompt, stale identity, or known active work prevents destructive teardown.
3. Stop/persistence failures retain retryable ownership; delayed terminal events never cancel or complete a successor session turn.

## Verification

Use TDD with `TestSuspendIdlePreservesRuntimeOwnershipWithoutAgentStopped` and `TestSuspendIdleRejectsStaleIdentityAndMissingRestoreData` in `apps/backend/internal/agent/runtime/lifecycle/manager_stop_persistence_test.go`.
Executor-specific tests cover retained task containers and workspaces; the repository CAS test covers stale lifecycle writes and successor recovery.

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/... ./internal/agentctl/server/instance ./internal/agentctl/server/process -run 'SuspendIdle|IdleParking|IsIdle|StopAgentWithReason' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'SuspendIdle|IdleParking' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/repository/... -run 'IdleSuspension' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/runtime.go`, `facade.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`, `manager_interaction.go`, `persistence.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_idle_parking.go` and suspension tests in `manager_stop_persistence_test.go`
- Executor implementations under `apps/backend/internal/agent/runtime/lifecycle/`
- Executor lifecycle implementations and their stop-contract tests
- `apps/backend/internal/task/models/models.go` and runtime-inventory repository/migration files

## Dependencies

None.

## Risks

Normal executor shutdown can remove task compute. The suspension reason must preserve it explicitly.
The operation cannot guarantee preservation of work a provider never exposes through ACP; the opt-in policy accepts that visibility limit.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/idle-runtime-parking.md)
- [Design](../../specs/executors/system-design/idle-runtime-parking.md)
- Existing stop persistence, retry, and generation tests

## Results

Implemented `Runtime.SuspendIdle` with an identity-bound CAS and a distinct suspension reason.
The lifecycle releases the exact ACP process while retaining its executor-owned workspace, task compute, and resume identity. Executor stop contracts cover Docker, SSH, Kubernetes, plugin, Sprites, and local runtime paths.
Durable `executors_running.idle_suspension_state` survives stale lifecycle upserts for the same execution and clears only after recovery readiness. Stale suspension identities and missing restore data are rejected.

Regression coverage: `TestSuspendIdlePreservesRuntimeOwnershipWithoutAgentStopped`, `TestSuspendIdleRejectsStaleIdentityAndMissingRestoreData`, and `TestExecutorRunningUpsertDoesNotRegressIdleSuspensionForSameExecution`.
The full backend suite and backend build passed in this implementation session.

## Superseded investigation evidence

The earlier OpenCode quiescence and macOS smoke gates were explicitly withdrawn by the user. The implemented opt-in policy uses Kandev-observed state and protects known work without claiming visibility into provider-internal activity.
