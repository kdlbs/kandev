---
id: "01-manual-admission"
title: "Admit manual recovery after inspection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 01: Admit manual recovery after inspection

## Summary

Let one manual Resume reach the current classification after a competing
workspace inspection releases its lock. Keep lifecycle admission non-blocking
and keep dirty relocation authorization separate from the inspection wait policy.

## In scope

- Add an inspection-wait policy to `RecoveryAdmissionRequest` and a typed internal
  contention result. Preserve the default immediate-refusal behavior.
- Select bounded waiting only in manual `PreflightSessionWorktreeRecovery`, outside
  lifecycle singleflight. The limit is 15 seconds or the caller's shorter deadline.
- Re-resolve canonical slots after acquisition. Preserve slot order and release
  every acquired lock on cancellation, timeout, or failed validation.
- Return a sanitized conflict for exhausted manual waiting. Keep durable claims,
  runtime liveness, owner generation, and explicit dirty authorization unchanged.
- Add deterministic barrier regressions with real Git and a persisted selected inventory.

## Out of scope

Durable relocation-error projection, UI rendering, new migrations, and live repair.
Do not broaden the existing `RelocateDirty` context as a way to permit waiting.

## Acceptance

1. One manual Resume waits for a released inspection lock, then returns the
   verified dirty-relocation result without moving files or clearing a provider token.
2. Cancellation, deadline expiry, changed inventory, live consumers, and competing
   durable authority cannot publish a replacement or leave a slot lock held.
3. Lifecycle/background callers still return promptly under contention. A
   concurrent lifecycle singleflight and manual preflight cannot deadlock or
   start duplicate executions. Scope exclusions and legacy reuse still pass.

## Regression first

Add `TestManualRecoveryPreflightRequestsInspectionWaitWithoutDirtyAuthorization`
in `executor_manual_recovery_contention_test.go`. Assert the manual preflight
requests bounded inspection waiting without enabling dirty relocation. The held
lock behavior and post-wait revalidation are covered by real-manager barriers in
`recovery_admission_wait_policy_test.go`.
Record that behavioral RED before production edits.

Add `TestRecoveryAdmissionWaitPolicyHonorsCancellationAndDeadline` and
`TestRecoveryAdmissionWaitPolicyRejectsChangedInventory` in a new focused worktree test file.
Add `TestManualRecoveryPreflightDoesNotDeadlockWorkspaceSingleflight` beside lifecycle
workspace restore tests. Use the real manager and channels, not wall-clock sleeps.
Include a mixed inventory with an unchanged valid slot and a dirty or invalid sibling.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run '^TestManualRecoveryPreflightRequestsInspectionWaitWithoutDirtyAuthorization$' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/worktree ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/handlers -run 'ManualRecoveryPreflight|RecoveryAdmissionWaitPolicy|LockRecoverySlots|DirtyRecoverySlotLock|AdmitRecovery|RegisteredLegacy|ManagedClone|ManagedMain|SessionRecoveryGuard|WorktreeRecovery(Launch|Resume)Integration' -count=1)
git diff --check
```

The first command is the RED gate and then the GREEN regression.
The second command covers all new tests and affected existing admission cases.
Do not add a generic broad verification pass.

## Files likely touched

- `apps/backend/internal/worktree/recovery_admission.go`
- `apps/backend/internal/worktree/errors.go`
- `apps/backend/internal/worktree/recovery_admission_wait_policy_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_worktree_recovery.go`
- `apps/backend/internal/orchestrator/executor/executor_manual_recovery_contention_test.go` (new)
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_restore_admission_test.go`

## Dependencies

None.

## Risks

Waiting in lifecycle can invert the lock order. Admission policy must be explicit
and confined to the outer manual preflight. Slow ignored-file inspection can exceed
the wait limit. Timeout must remain a refusal, not permission to skip proof.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md), criteria listed in frontmatter.
- [Design](../../specs/tasks/system-design/managed-clone-relocation.md#inspection-contention-and-explicit-preflight).
- `recovery_admission_context_test.go` and `executor_legacy_clone_recovery_test.go` for existing fixtures.
- `.agents/skills/tdd/references/backend-tests.md` for channels, cleanup, and filesystem isolation.

## Results

The regression first failed because `RecoveryAdmissionRequest` had no separate
inspection-wait policy. After adding the field, it failed behaviorally with a
zero wait duration. The green implementation keeps that wait separate from
`RelocateDirty`, caps it at 15 seconds, honors caller cancellation/deadlines,
and maps exhausted waits to a path-free conflict.

The stale-inventory correction captures the session binding, selected
environment, owner generation, executor identity, and complete active
environment-repository membership, including unmaterialized slots and their
registered repository identity and path. After the original worktree locks are
acquired, admission compares a fresh persisted snapshot before reloading or
inspecting worktrees. Drift rejects the preflight and releases the locks without
adding newly discovered slots to the operation. Lifecycle reconstruction builds
its lock set from the same complete snapshot.

`TestRecoveryAdmissionRejectsSelectionDriftAfterInspectionWait` covers a new
sibling slot, selected-environment drift, and owner-generation drift while the
original worktree lock is held. The test failed behaviorally when the complete
snapshot guard was removed: admission inspected the checkout and returned a Git
metadata error instead of refusing stale inventory. The fresh-start regression
also asserts that a stale-admission refusal preserves the provider resume token
and does not start an agent.

Passed:

- `go test -tags fts5 ./internal/orchestrator/executor -run '^TestManualRecoveryPreflightRequestsInspectionWaitWithoutDirtyAuthorization$' -count=1`
- `go test -tags fts5 ./internal/worktree -run 'RecoveryAdmissionWaitPolicy|RecoveryAdmissionRejectsSelectionDriftAfterInspectionWait|ReadRecoverySelectionSnapshotIncludesCompleteActiveInventory' -count=1`
- `go test -tags fts5 ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'ManualRecoveryPreflight|RecoveryAdmission|WorkspaceRecovery|WorkspaceRestore|FreshStartPreflightPreservesProviderState' -count=1`
- `go test -tags fts5 -race ./internal/worktree ./internal/task/repository/sqlite ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/executor ./internal/orchestrator/handlers ./internal/backendapp -run 'WorkspaceRecovery|CommitWorkspaceRecoveryError|RecoveryAdmissionWaitPolicy|RecoveryAdmissionRejectsSelectionDriftAfterInspectionWait|ReadRecoverySelectionSnapshot|ManualRecoveryPreflight|FreshStartPreflightPreservesProviderState|RestoreWorkspace|WorkspaceRestore|MainCheckoutLaunchIntegration|MissingCheckoutRecovery' -count=1`
- `git diff --check`

No checks remain blocked for this work order.

Review follow-up: The race-enabled regression set passed after correcting the
main-checkout integration fixture to reread the selected snapshot. The
production guard remains fail-closed when a store cannot reread inventory.
The snapshot reader also records whether a replacement session is already
persisted, so pre-insert preparation verifies continued absence while retaining
the selected environment, generation, and full inventory checks.
`TestMissingCheckoutRecoveryLaunchAndResume` and
`TestSQLiteStore_ReadRecoverySelectionSnapshotTracksProspectiveSessionAbsence`
cover that pre-insert path.
