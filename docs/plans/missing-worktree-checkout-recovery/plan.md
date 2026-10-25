---
created: 2026-09-29
status: complete
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-004
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Fix plan: Missing canonical worktree checkout

## Overview

Resolve [issue #4052](https://github.com/kdlbs/kandev/issues/4052) through guarded
automatic recovery before read-only attachment. The user selected this behavior
on 2026-09-29. The tasks system owns this repair because it owns the environment
inventory, recovery authority, and launch lifecycle.

Implement the restoration operation first. Then prove its production launch and
resume integration and document its limits. Both work orders are sequential.

## Evidence and root cause

Inspected checkout: `a2346dec3af`. The issue reported `83a9ab93f`. The canonical
issue had no comments or image attachments when investigated.

`Manager.reuseRequiredWorktree` rejects an absent canonical checkout. This obeys
`AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2`. The defect is the gap before
attachment: `inspectRecoverySlot` returns no recovery work for `os.ErrNotExist`.
The ready environment retains active inventory, so repeated launches return to
the same attach-only refusal.

Smallest reproduction: create a managed worktree with a durable active row,
remove its checkout and Git registration, retain its branch, then run selected
recovery admission followed by attach-only `Create`. Admission does nothing and
attachment fails repeatedly with `ErrReuseWorktreeUnavailable`.

`TestCreate_ReuseRequiredRejectsRemovedTasksBase` passes on the current code.
It proves the low-level refusal and must remain passing after the fix. A
temporary Go overlay also exercises the surviving-branch reproduction without
adding a permanent test file. Its result is recorded below.

## Scope

### In scope

- Missing managed canonical checkout, including a removed managed tasks base.
- Surviving local branch, recorded remote branch, and applicable exact compaction identity.
- Exclusive environment claims, path safety, interrupted operations, and complete inventory validation.
- Existing session preparation, launch, resume, fresh-start, and workspace-restoration entry points.
- Existing typed refusal responses and public Git recovery guidance.

### Out of scope

- Mutation inside attach-only reuse or removal of its fail-closed tests.
- Lost uncommitted files, new branch selection, automatic runtime stops, or arbitrary external paths.
- Remote executor recovery, new UI controls, schema changes, and general worktree cleanup.
- Broad refactoring of metadata recovery or clone relocation.

## Technical approach

Extend `recovery_admission.go` with a missing-checkout classification. The
focused operation and its operation record live in
`missing_checkout_recovery.go`.

Preflight all selected slots before mutation. Acquire the existing durable claim
with `AllowCurrentSessionRuntime: false`, reload identity, and repeat inspection.
Restore only the recorded absent path and branch. Keep every durable slot identity.
Use safe directory handles and repository serialization. Handle only a proven
stale registration for this path. Do not call general `recreate` or broad prune.

Detect interrupted operations before the healthy fast path. Match operation ID,
owner generation, path, and branch head before continuation. Refuse unexpected
files. Retain completed slots after a later failure. Preserve claim authority
through the existing startup boundary. Hold a nonblocking OS operation lock for
each affected worktree from replay/adoption through admission release. Reconcile
a completed operation's retained claim only after rereading and validating every
matching record, checkout, owner, generation, session, and operation under that
lock.

Run Git creation through a pinned target identity and remove only exact proven
administrative metadata. If local and tracking refs are absent, probe origin for
the exact recorded branch. Keep probe failures distinct from confirmed branch
loss, and carry `resume_new_branch` authorization through the real preflight.

Use `executor_worktree_recovery.go` and lifecycle recovery admission as existing
integration points. Change callers only where tests prove a missing route or
lost claim. The specification and
[ADR](../../decisions/2026-09-29-missing-worktree-checkout-recovery.md) own the contract.

| Executor or input | Intended behavior | Evidence |
| --- | --- | --- |
| Worktree, selected active missing slot | Restore under exclusive authority | Real Git and SQLite regression |
| Worktree, healthy or metadata-damaged slot | Preserve existing attach or metadata recovery | Existing reuse and recovery suites |
| Worktree, shared or inherited environment | Include every environment consumer in busy checks | Borrowing-task and requester runtime tests |
| Local, Docker, SSH, Sprites, Kubernetes | No host missing-checkout recovery | Excluded-executor scope tests |
| Repo-free or executor transition | No inspection of unrelated host slots | Existing scope and transition tests |
| Unsafe, ambiguous, or unsupported path | Typed refusal before mutation | Negative path and registration tests |

## Tests

All short criterion references below use the worktree metadata recovery prefix.

| Acceptance | Test file and verification |
| --- | --- |
| 004.1, 004.7 | `internal/worktree/missing_checkout_recovery_test.go`: `TestMissingCheckoutRecoveryPreservesCanonicalIdentity` and `TestMissingCheckoutRecoveryUsesTheSurvivingBranchIdentity` |
| 003.1-003.3, 004.2 | `missing_checkout_recovery_test.go`: `TestMissingCheckoutRecoveryRefusesBusyEnvironmentBeforeMutation` covers requester-runtime, sibling-session, and borrower-session consumers; `TestMissingCheckoutRecoveryRefusesLivenessReadFailureBeforeMutation` fails closed |
| 004.3 | `missing_checkout_recovery_test.go`: `TestMissingCheckoutRecoveryRequiresAnActiveCanonicalRow`, stale-registration proof, task-root ownership checks, and invalid-branch preflight |
| 002.6-002.7, 004.4 | `missing_checkout_recovery_test.go`: `TestMissingCheckoutRecoveryPreflightsEverySlotBeforeMutation`, `TestMissingCheckoutRecoveryPreservesCompletedSlotWhenLaterSlotFails`, and `TestMissingCheckoutRecoveryLeavesHealthySlotUnchanged` |
| 003.2-003.4 | `missing_checkout_recovery_test.go`: `TestMissingCheckoutRecoveryRefusesCompetingClaimBeforeMutation` and `TestMissingCheckoutRecoveryResumesInterruptedOperation` |
| 004.5 | `missing_checkout_recovery_review_test.go`: `TestMissingCheckoutRecoveryPinsGitAddAcrossPathReplacement` and `TestMissingCheckoutRecoveryDoesNotDeleteCheckoutThatAppearsBeforeCleanup` assert pinned mutation and unchanged appeared trees |
| 004.8 | `missing_checkout_recovery_review_test.go`: `TestMissingCheckoutRecoverySameOperationReplayIsExcludedAcrossManagers`, `TestMissingCheckoutRecoveryLeavesCompletedMatchingClaimForRestartReconciliation`, and `TestMissingCheckoutRecoveryContinuesMultiSlotClaimFromCompletedRecord` |
| 004.9 | `missing_checkout_recovery_review_test.go`: `TestMissingCheckoutRecoveryUsesTheSurvivingBranchIdentity`, `TestMissingCheckoutRecoveryDistinguishesOriginProbeFailure`, `TestMissingCheckoutRecoveryPreservesConfirmedBranchLossContract`, and `TestMissingCheckoutRecoveryExplicitNewBranchUsesClaimedPreflight` |
| 001.1-001.5, 004.6 | `internal/orchestrator/executor/executor_missing_checkout_recovery_integration_test.go`: `TestMissingCheckoutRecoveryLaunchAndResume` covers existing-environment preparation, prepared launch, and resume with real Git, SQLite, and the recovery manager |
| Launch refusal and projection | `internal/orchestrator/missing_checkout_recovery_test.go` preserves provider state on preflight refusal; `internal/agent/runtime/lifecycle/missing_checkout_recovery_test.go` verifies restored path projection and attach |
| Executor exclusions | Existing `recovery_admission_scope_test.go` confirms repo-free and excluded executor requests do not inspect host checkouts |
| Attach immutability | Existing `TestCreate_ReuseRequiredRejectsRemovedTasksBase` and canonical-worktree Git-state test remain unchanged |

## End-to-end evidence

Task 02 owns a backend integration test with real Git, SQLite inventory, and
the real recovery manager. A runtime fake records the startup boundary. The test
must reach production executor admission and verify the working directory and
branch before agent startup. A recording admission callback is insufficient.

Exercise additional-session preparation, prepared launch, resume, fresh-start
preflight, and workspace-only restoration. Assert refusal prevents startup and
provider-state mutation. Existing desktop and phone error surfaces remain unchanged.
No rendered UI change or browser-only evidence is required.

## Companion packages

The [metadata-recovery package](../worktree-metadata-recovery/plan.md) contains
the existing metadata-recovery implementation and its historical verification
results. This package extends its eligibility boundary without changing those
historical work-order statuses. Existing metadata and clone-relocation
regressions remain in place.

## Work orders

- [x] [Task 01: Restore missing checkouts under exclusive authority](task-01-restore-checkout.md)
- [x] [Task 02: Prove launch recovery and document limits](task-02-prove-integration.md)

## Verification results

Task 01 and Task 02 completed on 2026-09-29.

Design-package checks on 2026-09-29:

- Existing `TestCreate_ReuseRequiredRejectsRemovedTasksBase`: passed.
- Temporary `TestIssue4052TemporaryTrace`, through a Go overlay: passed. Real
  Git retained the branch after checkout and admin removal. Recovery admission
  returned no work, and two attach attempts returned the expected refusal.
  The initial temporary fixture omitted `BaseBranch` and hit request validation.
  Adding `main` reached the intended path. No permanent test file was added.
- `python3 scripts/list-docs.py validate`: 330 decisions and 1246 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for both work orders
  with the planned runtime path included to force reference validation.
- `git diff --check`: passed.
- Package status check: plan and both work orders present and untracked.

Task 01 implementation checks on 2026-09-29:

- `go test -race ./internal/worktree -count=1`: passed.
- `go test -race ./internal/task/repository/sqlite -run 'TestTaskEnvironmentRecoveryClaim' -count=1`: passed.
- `go test -race ./internal/orchestrator/executor ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'TestMissingCheckoutRecovery|Test.*Recovery|TestRecoverSession_ResumeNewBranchPreservesSessionAndProviderIdentity' -count=1`: passed.
- Named review regressions for path pinning, completed-claim reconciliation, same-operation lock contention, remote probing, confirmed loss, and explicit branch replacement: passed.
- `golangci-lint run ./internal/worktree --timeout=5m` and `make -C apps/backend lint`: passed.
- `GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-worktree.test.exe ./internal/worktree`: passed.
- `make -C apps/backend build`: passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs` (62 tests) and `node scripts/validate-public-docs.mjs` (47 pages): passed.
- `git diff --check`: passed.
- PostgreSQL recovery-claim test: not run because
  `KANDEV_TEST_POSTGRES_DSN` is not configured.

Task 02 completed the public documentation and launch integration checks below.

Task 02 implementation checks on 2026-09-29:

- `go test -race ./internal/orchestrator/executor -run 'TestMissingCheckoutRecovery|TestWorktreeRecovery' -count=1`: passed, including preparation, prepared launch, and resume routes.
- `go test -race ./internal/orchestrator -run 'TestMissingCheckoutRecovery|Test.*Recovery' -count=1`: passed.
- `go test -race ./internal/agent/runtime/lifecycle -run 'TestMissingCheckoutRecovery|Test.*Recovery' -count=1`: passed.
- `go test -race ./internal/worktree -count=1`: passed with multi-slot canonical inventory assertions.
- `go test -race ./internal/task/repository/sqlite -run 'TestTaskEnvironmentRecoveryClaim' -count=1`: passed.
- `make -C apps/backend build`: passed.
- `python3 scripts/list-docs.py validate`: 330 decisions and 1246 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed; `node scripts/validate-public-docs.mjs`: 47 pages validated.
- PR-documentation reference coverage for both work orders: passed.
- `git diff --check`: passed.
- PostgreSQL recovery-claim test: not run because `KANDEV_TEST_POSTGRES_DSN` is not configured.

## Risks

- Directory absence does not prove that no process can use the environment.
- Generic recreation can remove a newly appeared path or change the branch head.
- Stale Git registrations can belong to another checkout or remain locked.
- A crash after materialization can leave a healthy path with an unfinished claim.
- PostgreSQL checks require a disposable DSN. A skipped test is not concurrency evidence.
- This recovery restores surviving committed content only.

## Current-main integration validation

Integrated `origin/main` at `ccaa7c0a8d80ef4f2f089b1f416bd1230417da0a` into
the PR branch after the base advanced. The three overlapping files were merged
additively: selected main checkouts keep main's health and provider-identity
checks, missing checkouts keep their record preflight and exclusive operation
lock, and the requirements and public guide retain both contracts. The
auto-merged launch tests retain the main-checkout fixtures and the explicit
`resume_new_branch` coverage.

On that merge candidate, `go test -race ./internal/worktree -count=1` and the
focused orchestrator, executor, and lifecycle recovery race tests passed.
`make -C apps/backend lint`, `make -C apps/backend build`, Windows/amd64 and
Darwin/arm64 worktree test cross-compiles, and the Chromium attachment E2E
passed. The targeted E2E ran three times with 3 passed and 0 failed. The docs
catalog validated 331 decisions and 1246 specifications; specification lint,
all 62 public-doc tests, all 47 published-page checks, and PR work-order
reference coverage passed. PostgreSQL claim verification remains outstanding
because `KANDEV_TEST_POSTGRES_DSN` is not configured.
