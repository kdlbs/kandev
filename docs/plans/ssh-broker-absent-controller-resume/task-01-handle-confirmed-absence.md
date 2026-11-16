---
id: "01-handle-confirmed-absence"
title: "Correct SSH resume after controller exit or host reboot"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.9
  - AC-EXECUTORS-SSH-EXECUTOR-001.10
  - AC-EXECUTORS-SSH-EXECUTOR-001.18
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

# Task 01: Correct SSH resume after controller exit or host reboot

## Summary

Correct the existing helper and its resume callers as one recovery fix.
Accept confirmed absence and a verified boot change without signalling a
current PID holder. Reuse the normal runtime reset and launch path.

## In scope

- Write and run the helper and lifecycle RED tests before production changes.
- In verify mode, use `BirthBootMatches` after receipt/UID validation and
  before PID inspection. Return existing `OutcomeAbsent` for a verified boot change.
  Preserve unresolved results for invalid or unreadable evidence.
- Keep matching-boot checks, child-drainage proof, and terminate-mode boot refusal.
- Correct tracked and untracked broker resume.
  Absence skips stop and clears only existing runtime keys.
  A verified live result keeps stop followed by independent absence proof.
- Let ordinary receipt-bearing SSH resume accept confirmed absence.
  Verify receipt-backed identity before numeric PID checks, and cover a reused
  current PID after reboot when `kill -0` returns permission denied. Keep the
  numeric liveness probe for legacy records.
- Keep `verifyRecordedControllerIdentity` strict for stop and adoption callers.
  Preserve the existing receipt/target, authentication, preflight, and owner checks.
- In the receipt-bearing already-absent persisted-stop branch, preserve the
  session directory. A numeric pidfile match is insufficient removal proof.
  Adjust `TestSSHAbandonedReceiptRequiresPidfileProofBeforeRemoval` to the new
  preservation rule rather than leaving its old removal assertion active.
- Include outcomes in broker errors and preserve typed errors.
  Keep tracked-resource teardown and legacy behavior.
  Check the existing startup-recovery absence mapping.

## Out of scope

New wire outcomes or fields, common process-identity changes, SQL changes,
new recovery services, runtime-secret repair, and rendered UI.

## Acceptance

1. `TestReconcileVerifyPreviousBootController` covers absent and reused PIDs.
   An owned live replacement and session-directory markers survive verification.
   `TestReconcileVerifyInvalidBootEvidence` refuses invalid proof.
   `TestReconcileTerminatePreviousBootRefuses` preserves the terminate contract.
   Existing same-boot PID, generation, UID, and directory guards pass.
2. `TestSSHBrokerResumeConfirmedAbsentController` covers both broker paths.
   Each absence reset performs one verify and zero stop calls.
   `TestSSHResumePreviousBootController` covers ordinary SSH with a reused PID.
   `TestSSHResumePreviousBootSkipsPermissionDeniedPIDProbe` proves a previous-boot
   receipt is checked before a reused PID can fail numeric liveness with permission denied.
   Workspace/target metadata survives, stale runtime keys clear, and no old process is adopted.
3. Verified-live, failed-stop, unresolved, malformed, and child-survival cases
   preserve their existing result and resource-lifecycle rules.
   `TestSSHAbsentReceiptPreservesDirectory` proves that absence causes no
   directory removal, including a pidfile with the old numeric PID.
   `TestSSHVerifyRecordedControllerIdentityRequiresLiveController` still rejects `absent`.
   Diagnostics expose the actual outcome without credentials.

## Verification

Run from the repository root. Add relevant `@covers` annotations.
The first two commands must record behavioral RED before the correction.

```sh
(cd apps/backend && go test -trimpath ./internal/agentctl/sshreconcile -run '^TestReconcileVerifyPreviousBootController$' -count=1 -v)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^(TestSSHBrokerResumeConfirmedAbsentController|TestSSHResumePreviousBootController)$' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/sshreconcile -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run '^(TestSSHBrokerResume.*|TestSSHVerifyRecordedControllerIdentity.*|TestSSHResume.*|TestSSHAbsentReceipt.*|TestSSHAbandonedReceipt.*|TestSSHDirectoryLoss.*|TestSSHRecovery.*Receipt.*|TestSSHRecoveryReportsAbsentController|TestSSHExecutor(ResumeRemoteInstance|ResetManagedBrokerResume|ResetTrackedManagedBrokerResume.*)|TestSSHLaunchReceipt.*|TestClearSSHResumeRuntimeMetadata)$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/sshreconcile/reconcile_linux.go apps/backend/internal/agentctl/sshreconcile/reconcile_boot_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reconcile.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_broker_resume_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_directory_loss_test.go
git diff --check
```

New helper tests need the Linux build constraint.
Use the existing owned-process fixtures and small local test tables.
Keep syscall and process infrastructure as it is.
Record actual results and add any other changed Go files to the lint command.

## Files likely touched

- `apps/backend/internal/agentctl/sshreconcile/reconcile_linux.go`
- `apps/backend/internal/agentctl/sshreconcile/reconcile_boot_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reconcile.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_broker_resume_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_directory_loss_test.go`

## Dependencies

None. Receipt production, `BirthBootMatches`, and the reset/create path already exist.

## Risks

A shared verify result must not authorize destructive follow-up.
Preserve absent receipt directories instead of adding another proof protocol.
Apply the boot shortcut only to verify mode.
Keep existing execution ownership and resource teardown.

## Parallelism

`sequential`

## Inputs

- [SSH requirements](../../specs/executors/requirements/ssh-executor.md), especially AC-001.9
- [Boot-aware verification](../../specs/executors/system-design/ssh-agentctl-reconciliation.md#boot-aware-verification)
- [Broker resume](../../specs/executors/system-design/ssh-agentctl-reconciliation.md#managed-credential-broker-resume)
- [Amended launch-proof decision](../../decisions/2026-11-07-ssh-controller-launch-proof.md)
- Existing `reconcile_linux_test.go`, `executor_ssh_resume_receipt_test.go`,
  `executor_ssh_stop_resume_test.go`, `executor_ssh_keepalive_test.go`,
  and `executor_ssh_directory_loss_test.go`

## Results

Completed on 2026-11-16.

- RED: `TestReconcileVerifyPreviousBootController` failed for both absent and
  reused PIDs with the expected previous-boot conflict. The lifecycle RED run
  failed for tracked and untracked broker resume and ordinary resume after the
  PID probe reported a live reused holder.
- The PR review found that a different-user reused PID could return permission
  denied before receipt verification. `TestSSHResumePreviousBootSkipsPermissionDeniedPIDProbe`
  reproduces that ordering failure.
- GREEN: `TestSSHResumePreviousBootSkipsPermissionDeniedPIDProbe` passes, along
  with both focused regressions. Invalid boot evidence and terminate-mode
  previous-boot refusal also pass. Verified absence skips broker stop, clears
  only runtime metadata, and preserves workspace and target metadata.
- `go test -trimpath -race ./internal/agentctl/sshreconcile -count=1`: passed.
- The focused lifecycle race command in this work order: passed.
- `bash scripts/lint-go-changed` with every changed Go file: passed, 0 issues.
- `git diff --check`: passed.

Receipt-backed absent controller directories are preserved even when the
numeric pidfile matches. `verifyRecordedControllerIdentity` remains live-only.
