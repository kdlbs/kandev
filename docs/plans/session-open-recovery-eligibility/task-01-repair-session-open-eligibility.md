---
id: "01-repair-session-open-eligibility"
title: "Restore ordinary recovery for workflow-stopped sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.7
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.8
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.9
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.4
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 01: Restore ordinary recovery for workflow-stopped sessions

## Summary

Remove workflow parking as an automatic recovery restriction. Preserve workflow
recipient isolation, automatic capacity admission, and execution-correlated callbacks.

## In scope

- Add `session_open_recovery_test.go` with `TestSessionOpenRecoveryEligibility`,
  `TestSessionOpenRecoveryOwnership`, `TestSessionOpenRecoveryAfterRestart`, and
  `TestSessionOpenRecoveryDelayedCallbacks`.
- Record RED for modern parking, historical stop intent, empty deferral, and their combination.
- Cover primary/non-primary sessions and the full backend matrix in the plan.
- Remove parking and stop-intent suppression from `autoResumeEligibility`.
- Accept empty settled deferrals; preserve nonempty queue validation.
- Exercise status, `passiveLaunchResponse`, `EnsureSession`, and real automatic admission.
- Verify full-capacity sibling recovery cannot replace another destination's accepted work.
- Audit parking-only writers/readers and remove unused helpers with their obsolete tests.
- Keep stop-intent tombstones, callback parsing, and independent lifecycle safeguards.
- Update old tests that assert parked-session suppression to assert the new contract.

## Out of scope

UI removal and rendered tests belong to Task 02. No live-data edits, migration,
new queue, manual ceiling override, workflow promotion, or prompt replay.

## Acceptance

1. Selected stopped conversations, including non-primary ones, recover under normal
   rules regardless of workflow parking metadata. Empty settled records do not block them.
2. Actual queued destinations, guarded admission, capacity, archive, terminal, and
   authorization rules remain intact. Opening a sibling never changes the queued prompt or primary.
3. Persisted restart tests and delayed-event tests pass without deleting stop tombstones.
   Existing deadlock regressions remain green.

## Verification

Run from repository root. Add proposed tests before their commands.

```bash
(cd apps/backend && go test ./internal/orchestrator -run '^TestSessionOpenRecovery' -count=1 -timeout=120s)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestSessionOpenRecovery|TestAutoResumeEligibility|TestGetTaskSessionStatus|TestPassiveLaunchResponse|TestEnsureSession|Test.*WorkflowParking|Test.*ProfileSwitch|Test.*WorkflowRoute|TestCeilingReplay' -count=1 -timeout=300s)
(cd apps/backend && go test ./internal/task/models ./internal/task/repository/sqlite -run 'Test.*WorkflowParking|Test.*DeferredLaunch' -count=1 -timeout=120s)
git diff --check
```

Use real repository persistence and startup reconciliation for restart coverage.
Test queue/route replacement between status and launch with deterministic barriers.
Audit existing test names when removing policy-only code so remaining behavior
still has coverage. If dialect-sensitive repository behavior changes, add its
PostgreSQL conformance command before completion. No database migration is planned.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`.
- `apps/backend/internal/orchestrator/session_launch.go` and `session_ensure.go`.
- `apps/backend/internal/orchestrator/workflow_session_target.go`.
- `apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go`.
- New `apps/backend/internal/orchestrator/session_open_recovery_test.go`.
- Existing resume, launch, ensure, workflow-target, and profile-switch tests.
- Parking-only model/repository helpers and tests, only if the consumer audit proves them unused.

## Dependencies

None. This replaces the prior work order's current-primary-only exception.

## Risks

Stop-intent metadata is not disposable parking UI state. Preserve event identity
checks. A queue conflict must not silently redirect a selected conversation.

## Parallelism

`sequential`

## Inputs

- [Plan and matrix](plan.md).
- [Design](../../specs/tasks/system-design/queued-session-ownership.md#conversation-recovery-and-workflow-stop-history).
- [Decision](../../decisions/2026-09-18-session-open-resumes-conversation.md).
- Existing automatic admission, workflow reuse, and stop-event regression fixtures.

## Results

Implemented the backend recovery contract.

- `autoResumeEligibility` no longer treats workflow parking or historical stop-intent metadata as passive recovery ownership.
- Empty settled `deferred_launch` objects are accepted as having no pending launch.
- Nonempty deferred launches still require valid destination identity and retain the queued-destination guard.
- The guarded `session_open` admission boundary rechecks current queue ownership under the existing lifecycle and ceiling-entry lock order. A capacity conflict with another accepted destination returns a successful waiting disposition and preserves that record.
- Added real `LaunchSession` coverage for status-driven recovery, free/full capacity, unchanged queued recipient and prompt, primary preservation, no manual override, no runtime launch on refusal, and queue replacement after the initial eligibility read.
- Added restart, successor-execution delayed-callback, stale-ownership, status, passive-launch, `EnsureSession`, primary/non-primary, parking, stop-intent, and settled-record regressions in the split session-open recovery test files. The delayed-callback case keeps an active successor turn and verifies route, primary, execution, and accepted queue state across service reconstruction.
- Updated existing resume and passive-launch tests to preserve the new recovery contract while retaining stop tombstones and parking lifecycle behavior.

Verification:

- Passed `go test ./internal/orchestrator -run '^TestSessionOpenRecovery' -count=1 -timeout=120s`.
- Passed the prescribed orchestrator race matrix.
- Passed the workflow parking and deferred-launch model/SQLite checks.
- Passed `make -C apps/backend build` and `make -C apps/backend lint`.
- Passed the full `internal/orchestrator` package.
- `git diff --check` passed.
- The complete backend test target reported unrelated environment-sensitive failures in `internal/agentctl/server/process/probe`, `internal/common/config`, and `internal/launcher`; all other reported packages, including the changed package, passed.

Task 02 owns the frontend parking presentation removal and desktop/mobile E2E coverage.
