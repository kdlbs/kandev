---
id: "02-policy"
title: "Policy, dependencies and flag"
status: pending
wave: 2
depends_on: ["01-state"]
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-MERGE-002
  - REQ-COORDINATOR-MERGE-003
acceptance_criteria:
  - AC-COORDINATOR-MERGE-002.1
  - AC-COORDINATOR-MERGE-002.2
  - AC-COORDINATOR-MERGE-002.3
  - AC-COORDINATOR-MERGE-002.5
  - AC-COORDINATOR-MERGE-003.1
  - AC-COORDINATOR-MERGE-003.6
system_design:
  - ../../specs/coordinator/system-design/merge.md
---

# Task 02-policy: Policy, dependencies and flag

## Summary

Repository policy map with selecting manager, expected-revision compare (400 revision_required, 409 revision_conflict), audit, dependency sets with own revision, protection observations per target branch, features.coordinatorMerge in runtimeflags registry and profiles.yaml (prod false, e2e true), requiredstores descriptors, PostgreSQL advisory lock for read-modify-write.

## In scope

- Deliver the listed acceptance criteria with the named tests, against the paired design.

## Out of scope

- Releases, branch pushes, provider bypass and the parallel answer proposal.

## Acceptance

- 002.1/002.3: TestPolicyDefaultDenied, TestPolicyAuthorization (reader/coordinator 403), TestAutomaticIndependentOfEligibility.
- 002.2: TestPolicyCASConflict, TestUnchangedSaveNoRevision, TestOmittedMapUnchanged, TestPartialMapAtomic, TestPrincipalReplacement (valid principal cannot be replaced), TestDependencyRevisionRequired, PostgreSQL concurrent save.
- 003.1: TestDependencyCycleBlocks, TestUnlinkedChangeRefBlocks, TestDependencyLockAtRecheck.
- Docs: wake.md, pause.md and README amendment pointers.
- 002.5: TestProtectionUnknownOnFailure, TestGitHubRulesetsAndClassic404.
- 003.1: TestDependencyValidation, TestDependencyRevisionConflict, TestDependencyAudit.
- 003.6 (flag only): runtimeflags completeness tests, TestFlagOffHidesAndBlocks. The `approving` claim cases of 003.6 belong to task 03, where the executor and dispatch record exist.

## Verification

```bash
(cd apps/backend && go test -race ./internal/coordinator/... ./internal/runtimeflags/... ./internal/persistence/storeconformance && go run ./cmd/sqlguard ./internal)
```

## Files likely touched

- apps/backend/internal/coordinator; internal/runtimeflags; profiles.yaml

## Dependencies

Answer-proposals slice.

## Risks

Provider reads may fail or race; preserve the fail-closed and uncertain-outcome rules.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/coordinator/requirements/merge.md) and [design](../../specs/coordinator/system-design/merge.md).
- Existing coordinator and provider code and tests.

## Results

Pending.
