---
id: "03-merge"
title: "Guarded merge"
status: pending
wave: 3
depends_on: ["02-policy"]
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-MERGE-002
  - REQ-COORDINATOR-MERGE-003
acceptance_criteria:
  - AC-COORDINATOR-MERGE-002.4
  - AC-COORDINATOR-MERGE-003.2
  - AC-COORDINATOR-MERGE-003.3
  - AC-COORDINATOR-MERGE-003.4
  - AC-COORDINATOR-MERGE-003.5
  - AC-COORDINATOR-MERGE-003.7
  - AC-COORDINATOR-MERGE-003.6
  - AC-COORDINATOR-MERGE-003.8
system_design:
  - ../../specs/coordinator/system-design/merge.md
---

# Task 03-merge: Guarded merge

## Summary

propose_merge_kandev tool and refusal codes, merge kind in executor and guard, merge table with unique open change key, ordered rechecks, dispatch record, GitLab MergeMR and MergeMRForAutomation expected SHA, uncertain reconciliation, lowering, revert detection, activity/ledger/outcome recording (backend). Update the phase 2 guard walk for the amended PERMISSIONS-001.4.

## In scope

- Deliver the listed acceptance criteria with the named tests, against the paired design.

## Out of scope

- Releases, branch pushes, provider bypass and the parallel answer proposal.

## Acceptance

- 003.2: TestProposeRefusalCodes, TestDedupeAcrossCardsAndCoordinators.
- 003.3: table test over each condition in order, TestNoMutationOnRefusal, GitHub and GitLab request carry expected SHA, deferred merge never used.
- 003.4: TestConcurrentApprovalsOneRequest, TestUncertainNotReplayed, TestReconcileMergedOpenClosedElsewhere, TestStaleApprovingRetry.
- 002.4/003.7: TestProviderRejectLowers, TestPreflightRefusalDoesNotLower, TestRevertLowers, TestStricterConcurrentWins.
- 003.2 additions: TestCreationRefusalOrder, TestDedupeSupersedesPendingWithOtherHead, TestDedupeReturnsApprovingUnchanged, TestOwningCoordinatorGoverns.
- 003.4 additions: TestStaleClaimNeverFailed (no dispatch to pending, dispatch to uncertain, deadline fired), TestLockedIsUncertain, TestReconcileMinimumAge, TestDispatchUniqueOnlyWhileUnsettled, TestManagerReleaseUncertain, TestBackstopVisitsAutonomyOffUncertain.
- 003.8: TestAutomaticWaitsOnPending, TestAutomaticRefusesRedAndConflict, TestAutomaticPausedManagerOnly, TestAutomaticDailyLimitManagerOnly, TestAutomaticExpiresAfter7Days, TestManagerApproveUnmetRefuses, TestManagerApproveIgnoresPause. Round-4 cases: TestProviderShaMismatchIsStaleNoLower, TestQueuedResponseStaysUncertain, TestCreationRefusalNullProposalID, TestPausedAtRecheckIsManagerOnly, TestDeniedPolicySettlesAuthority, TestRequiresApprovalExpiresAndGoesStale, TestUniqueIndexesIncludeWorkspace, TestReplacePrincipalMarkerRequired, TestReleaseWindowFromDispatch. Round-5 cases: TestGitLabDetailedStatusMapping, TestGitLabConflictClearsOnHasConflictsFalse, TestBackstopVisitsAutonomyOffOpenProposals, TestAutomaticDispatchRecheckWaitingReturnsPending, TestPrincipalCheckedOnlyAtClaimAndRecheck, TestCreationEvaluationSynchronousNoDispatch, TestChangeUnknownBeforePolicyDenied, TestDetailOpenAndApproveRunUncachedRead, TestProviderStaleHeadSettlesDispatchCountsLimit, TestLockedAtRecheckUnavailableMergeable, TestReplacePrincipalStaleRevisionConflictWins.
- 003.6 claim cases: TestFlagOffClaimRecovery (no dispatch to pending, dispatch to uncertain, no provider call), TestAutomaticPendingResumesWhenFlagReturns.
- 002.4 additions: TestNotMergedLowers, TestMergedElsewhereClosedUnconfirmedDoNotLower, TestRevertUnknownLowersNothing, TestPrincipalInvalidRefusesAuthorityNoLower, TestLoweringBoundToAutomaticRevision, TestLoweringOncePerDispatch, TestRevertScanBounds.
- GitLab: MergeMR and MergeMRForAutomation take expected SHA and squash flag (TestGitLabMergeSendsShaAndSquash).
- 003.5: activity, ledger and outcome rows identify change, decision and result; no Undo.

## Verification

```bash
(cd apps/backend && go test -race ./internal/coordinator/... ./internal/github/... ./internal/gitlab/...)
```

## Files likely touched

- apps/backend/internal/coordinator; internal/github; internal/gitlab

## Dependencies

Answer-proposals slice, 02-policy.

## Risks

Provider reads may fail or race; preserve the fail-closed and uncertain-outcome rules.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/coordinator/requirements/merge.md) and [design](../../specs/coordinator/system-design/merge.md).
- Existing coordinator and provider code and tests.

## Results

Pending.
