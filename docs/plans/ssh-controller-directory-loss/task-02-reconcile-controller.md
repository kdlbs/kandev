---
id: "02-reconcile-controller"
title: "Reconcile the original SSH controller after directory loss"
status: done
wave: 2
depends_on:
  - "01-record-launch-proof"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.19
  - AC-EXECUTORS-SSH-EXECUTOR-001.20
  - AC-EXECUTORS-SSH-EXECUTOR-001.21
  - AC-EXECUTORS-SSH-EXECUTOR-001.22
  - AC-EXECUTORS-SSH-EXECUTOR-001.23
  - AC-EXECUTORS-SSH-EXECUTOR-001.24
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
  - ../../specs/executors/system-design/ssh-agentctl-reconciliation.md
---

# Task 02: Reconcile the original SSH controller after directory loss

## Summary

Add a bounded remote helper that can stop the receipt's original Linux process
after its session directory disappears. Preserve strict identity guards and
retain an unresolved result when the helper cannot prove safe progress.

## In scope

- Dispatch private `agentctl ssh-reconcile` before server startup. Accept bounded
  JSON from stdin and return a typed result. Resolve a compatible verified
  remote helper through the existing resolver/upload path.
- Open a pidfd before the final identity probes. Recheck the launch birth token,
  marker, UID, owner binding, command/workdir, and missing session path.
- Signal only through that pidfd with SIGTERM. Wait at most five seconds within
  `sshRemoteCleanupContext`. Close every descriptor on every result.
- Select this route through a typed missing-directory result, not error text.
  Preserve existing behavior for present matching pidfiles and confirmed absence.
- Recheck persisted owner identity before dispatch and settlement. Include the
  receipt in current-owner comparisons and recovery candidate checks.
- Return the design's fixed unresolved reasons without credentials or raw procfs
  output. Lost replies and malformed helper output cannot become success.

## Out of scope

PID-only fallback, fabricated pidfiles, SIGKILL escalation, group discovery,
directory removal, new API/UI, and unrelated executor changes.

## Acceptance

1. `TestSSHDirectoryLossReconciliationOriginalProcessAndRetry` fails against the
   current implementation, then passes through the real persisted-stop path.
   Repeating a successful stop observes absence and sends no replacement signal.
2. Owned-process tests and race hooks cover PID reuse, reboot, changed generation,
   and replacement paths. They cover replacement after handle opening, denied
   procfs/pidfd access, and an unresponsive controller. The helper
   never signals the replacement or removes any path. Existing identity and
   macOS-shaped absence regressions pass.
3. Legacy proof gaps, SSH/probe ambiguity, lost replies, stale SQL ownership,
   and deadline expiry retain the recorded target. Test exact-owner validation
   before signal dispatch and before settlement. Only an explicit matching
   same-owner success permits the caller to finalize cleanup.

## Verification

Run from the repository root. The Linux helper tests must exercise real pidfds
against owned subprocesses, plus injected errors. Mark unsupported native
coverage explicitly instead of treating a skip as proof.

```sh
(cd apps/backend && go test -trimpath -race ./internal/common/processidentity ./internal/agentctl/sshreconcile -count=1)
(cd apps/backend && GOOS=darwin GOARCH=arm64 go build -trimpath ./internal/common/processidentity ./internal/agentctl/sshreconcile)
(cd apps/backend && go test -trimpath -race -tags fts5 ./cmd/agentctl -run 'TestSSHReconcile' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHDirectoryLoss|TestVerifyRemoteAgentctlIdentity|TestRemoteProcessCommandLineCommand|TestSSHExecutorStopInstance|TestStopAgentWithReason.*PersistedSSH|TestSSHRecovery.*Receipt' -count=1)
```

Before completion, run `bash scripts/lint-go-changed` with every Go file changed
by this work order. Pass explicit paths from the repository root.

## Files likely touched

- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/cmd/agentctl/ssh_reconcile.go` and `ssh_reconcile_test.go` (new)
- `apps/backend/internal/agentctl/sshreconcile/reconcile.go` and `reconcile_linux_test.go` (new)
- `apps/backend/internal/agentctl/sshreconcile/reconcile_linux.go` and `reconcile_other.go` (new platform boundaries)
- `apps/backend/internal/common/processidentity/processidentity_linux.go`
- `apps/backend/internal/common/processidentity/processidentity_other.go`
- `apps/backend/internal/common/processidentity/processidentity_darwin.go`
- `apps/backend/internal/common/sshlaunch/receipt.go` and tests
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_ssh_restart_cleanup.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery_stop.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_directory_loss_test.go` (new)

## Dependencies

Task 01. The helper and persisted-stop caller share its receipt contract.

## Risks

A procfs path read can observe a replacement after the pidfd opens. Keep the
process-directory handle, compare immutable birth/generation evidence, and
deliver through the retained pidfd. Directory checks do not grant process proof.

## Parallelism

`sequential`

## Inputs

- [SSH requirements](../../specs/executors/requirements/ssh-executor.md), AC-001.19 through AC-001.24
- [Reconciliation design](../../specs/executors/system-design/ssh-agentctl-reconciliation.md), remote reconciliation and race boundaries
- `executor_ssh_persisted_stop_test.go` and `executor_ssh_liveness_test.go`
- `internal/common/processidentity` and `samePersistedRemoteStopOwner`

## Results

Implemented.

- `internal/agentctl/sshreconcile` implements the private Linux path: it admits
  only a complete receipt with a positively absent session path, opens the
  process-directory handle and a pidfd, re-proves birth identity through
  `processidentity.InspectProcStat`, checks the exact agentctl command/workdir
  and launch-generation marker, rechecks directory absence, sends SIGTERM only
  through the retained pidfd, and waits up to five seconds on that handle.
  Paths are never created, removed, or renamed.
- Non-Linux platforms return `unresolved: unsupported_identity`. Missing pidfd
  support, denied procfs access, malformed requests, and transport/probe
  failures return closed unresolved reasons. A bare numeric-PID signal is never
  attempted.
- `cmd/agentctl` dispatches `ssh-reconcile` before flag parsing or server
  startup. It reads bounded JSON from stdin and writes bounded JSON to stdout.
- `processidentity.InspectProcStat` is the narrow opened-handle identity
  check used by the helper.
- The lifecycle path now distinguishes a typed
  `errSSHRemoteSessionDirectoryMissing` result, requires a receipt whose PID
  and owner match the runtime record, dispatches the verified remote helper
  through the existing resolver/upload path, and preserves every other
  verification refusal. Success settles only `stopped` or `absent`; unresolved
  reasons keep the record.
- A present receipt is validated, including its PID, before verification,
  signalling, or directory removal. A null, malformed, foreign, PID-mismatched,
  or conflicting receipt never degrades to legacy evidence; only a genuinely
  absent key remains legacy. In the present-directory branch, a receipt-bearing
  record dispatches the helper's `verify` operation before the existing guarded
  pidfile stop. The unmatched-process branch refuses directory removal while
  the recorded PID is still alive.
- Recovery checks the receipt PID against the record's PID and re-proves the
  live birth identity and generation through the same `verify` operation
  before adoption. A recycled PID with the same command/workdir and a
  recreated pidfile is rejected.
- The helper reports `stopped` or `absent` only after every process in the
  recorded launch session is gone. `processidentity.SessionMembersRemain`
  supplies that post-exit evidence; a surviving child, including on a retry
  after the controller exited, keeps the result unresolved. No process-group
  kill is introduced.
- Helper response transport is bounded on both stdout and stderr while still
  draining the channel. An oversized stream is torn down and reported as
  `probe_unknown`; it cannot grow backend memory before the cleanup deadline.
- The pending remote-recovery snapshot exception is limited to the exact
  archive delta: the same workspace, environment ownership and generation,
  route, and delivery revision, with the session cancelled under a running
  task cleanup claim. Any unrelated ownership change stays fenced.
- `stopPersistedSSHRecord` revalidates the exact owner immediately before the
  external stop and again before settlement. The comparison covers
  identity-bearing fields (execution, task, session, executor, runtime,
  controller instance, PID, paths, target, and receipt) and ignores volatile
  runtime metadata that the manager rewrites between reads of the same owner.
  A successor execution, changed receipt, vanished row, or unreadable
  inventory fences the result. Recovery refuses a record whose present
  receipt does not describe it, while legacy records still recover through the
  guarded pidfile path.
- A pending remote-recovery snapshot that moved because archive cancelled the
  same session no longer blocks an explicit teardown. The stop re-reads the
  persisted record and proceeds only when that identity is unchanged; a
  successor or changed receipt still fails.

Tests (all passing):

- `TestSSHDirectoryLossReconciliationOriginalProcessAndRetry`
- `TestSSHDirectoryLossReconciliationPreservesUnknown`
- `TestSSHDirectoryLossReconciliationNotSelectedForOtherVerificationErrors`
- `TestReconcileStopsTheRecordedProcess`
- `TestReconcileRejectsIdentityChanges` (birth token, generation, workdir, UID,
  replacement directory)
- `TestReconcileReplacementBeforeSignalPreservesTargets` (a directory that
  appears after process admission, before the signal)
- `TestReconcileRejectsReusedPID`
- `TestReconcileAbsentProcessWithoutDirectoryCompletes`
- `TestReconcileCommandLineAndMarkerParsers`
- `TestDirectoryAbsenceFailure`
- `TestSSHStopRevalidatesExactOwnerBeforeEffects`
- `TestSSHStopRecordChecksOwnerBeforeExternalEffects`
- `TestSSHStopUsesExactPersistedOwnerAfterSnapshotFence`
- `TestSSHStopRejectsUnrelatedSnapshotChanges` (environment generation,
  workspace owner, route generation, recovery revision, non-archive state,
  missing cleanup claim)
- `TestSSHReceiptValidationPrecedesDestructiveBranches` (null, malformed,
  foreign, and PID-mismatched receipts)
- `TestSSHPresentDirectoryVerifiesBirthIdentityBeforeLegacyStop`
- `TestSSHRecoveryRequiresTheRecordsOwnLaunchReceipt` (PID mismatch)
- `TestSSHRecoveryRejectsAReplacementProcess`
- `TestSSHAbandonedReceiptRetainsSurvivingChildEvidence`
- `TestReconcileRetainsSurvivingChild` (including retry after leader exit)
- `TestReconcileStopsAfterChildDrainage`
- `TestReconcileVerifyModeChecksIdentityWithoutSignalling`
- `TestBoundedSSHOutputCapsRetentionAndDrains`,
  `TestRunSSHCommandStdinBoundedRejectsOversizedStdout`,
  `TestRunSSHCommandStdinBoundedRejectsOversizedStderr`,
  `TestRunRemoteSSHReconcileOverflowIsUnresolved`
- `TestSSHReconcileCommand*` utility-boundary tests

Verification:

```sh
(cd apps/backend && go test -trimpath -race ./internal/common/processidentity ./internal/agentctl/sshreconcile -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/agentctl -run 'TestSSHReconcile' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHDirectoryLoss|TestSSHStop|TestVerifyRemoteAgentctlIdentity|TestSSHExecutorStopInstance' -count=1)
(cd apps/backend && GOOS=darwin GOARCH=arm64 go build -trimpath ./internal/common/processidentity ./internal/agentctl/sshreconcile)
```

The Linux helper tests exercise real pidfds against owned subprocesses and
injected mismatches. `bash scripts/lint-go-changed` reported no issues.
