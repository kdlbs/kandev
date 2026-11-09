---
created: 2026-11-08
status: done
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
  - ../../specs/workspaces/system-design/worktree-base-refresh-recovery.md
  - ../../specs/platform/system-design/git-subprocess-execution.md
legacy_specs: []
---

# Implementation plan: Git refresh lock recovery

## Overview

Retry brief lock contention during backend worktree base refresh.
Let the existing fast-forward-only command finish a partial update after its lock becomes available.
Keep persistent failures visible without deleting locks or changing base selection.

One sequential work order delivers the helper, error propagation, regression tests, and public documentation.
The design-package documents, implementation, and permanent tests are recorded here.

## Evidence and repair

On November 8 at 22:01:04 UTC, task `60d1bcb5-e3a5-4d1d-83d2-fa1b62218e78` failed during base preparation.
The backend reported `lock_contention` for a pull of `main`, followed by exit code 128.
The retained evidence does not identify the exact lock or its owner.

Later inspection found no index or ref lock and no Git process using the repository.
Local `main` remained eight commits behind `origin/main`.
The index and files matched `origin/main`, with 167 staged entries and no unstaged changes.
The index timestamp, 22:01:04.003 UTC, coincided with the failed pull.
This state is consistent with a partial fast-forward, not proof of the original lock path.

A disposable fixture held `refs/heads/main.lock` during a fast-forward.
Git returned exit code 128 after updating the index and files while retaining the old branch ref.
After the fixture released its lock, a repeated fast-forward completed and left a clean checkout.
An additional fixture used the exact `git pull --ff-only origin main` command.
Both its clean case and its independent-edits case recovered after lock release.
The second case retained separate staged and unstaged local changes.

The user authorized live repair separately from product implementation.
At 23:12 UTC, a guarded normal fast-forward repaired the managed clone:

- Before: `36476ce8641da64df9e9bea509dbec2b6b818893`.
- Verified target and repaired head: `58cf68ddd2ea411df98e012e5a2e406393e78cbf`.
- Result: clean tracked checkout, with no index or branch-ref lock.

The original failed session was no longer in the database, so recovery did not restart it.
An old `commit-graph.lock` was not linked to the incident and remained untouched.
No deployment or automatic task restart formed part of the repair.

## Requirements and decisions

Workspaces owns this amendment because it owns base selection and worktree materialization.
The existing [requirement](../../specs/workspaces/requirements/worktree-base-refresh.md) gains criteria .29 through .34.
The platform [retry restriction](../../specs/platform/requirements/git-subprocess-admission.md) gains only the linked workspace exception.
The [decision](../../decisions/2026-11-08-worktree-refresh-lock-retries.md) preserves the boundary and rejected alternatives.

The user requested a simple repair and a pragmatic plan.
No material product choice remains unresolved for this scope.

## Scope

### In scope

- Four attempts at most for recognized lock failures in the two backend base-refresh commands.
- Fixed backoff within one execution budget, with prompt cancellation.
- Normal Git completion of a partial fast-forward and preservation of independent local changes.
- Fixed exhausted-lock errors through existing preparation and launch wrapping.
- Regression coverage and a short update to the existing public Git guidance.

### Out of scope

- Automatic lock deletion, force updates, resets, process killing, or a new repair engine.
- A global lock manager, background repair worker, new settings, metrics, or persistence.
- Replacing the refresh algorithm, skipping required freshness, or substituting another PR base.
- Generic retries for provider callbacks, qualified-base materialization, remote executors, push, or agent Git.
- New UI controls, event schemas, frontend copy, task creation, or session restarts.

## Technical approach

Add the private helper in `apps/backend/internal/worktree/refresh_lock_retry.go`.
Use the existing classified AfterAcquire runner separately for each attempt.
Capture its first post-admission deadline, then reuse that deadline for retries and backoff.
Keep the initial admission wait outside the execution budget.
Release the Git slot between attempts and wait with a cancellable timer.

Route only the base fetch in `pullBaseBranchWithPolicy` and the pull in
`pullCurrentBranchOrFallback` through this helper.
Use the existing diagnostic classifier with bounded recognition of actual Git lock errors.
Retain cancellation precedence and non-lock behavior.

Add a small private exhausted-lock error with fixed text and preserved causes.
Make `syncFailureCause` retain that safe error on required fetch failures.
Carry attempt count through the existing diagnostic logger.
The current lifecycle `%w` wrapping carries the result without a new API or UI path.

The recovery design's compatibility matrix is authoritative.
Backend fetch and pull gain retries regardless of their selected noninteractive transport.
`RemoteSyncHandled`, `materializeQualifiedPRBase`, and executor-owned materialization keep their existing behavior.
Unsupported output remains a single failed command under the existing policy.

During implementation, extend `docs/public/git-operations.md` with the bounded retry and persistent-lock behavior.
Keep active-lock, stale-lock, and permission failures distinct.
The page remains an explanation of Git behavior with its existing recovery guidance.
The implementation updated internal documents and that public page together.

## Tests

All new test names are proposed.

| Criteria | Regression evidence |
| --- | --- |
| .29, .32 | `refresh_lock_retry_test.go`: `TestRefreshLockRetryTransientFailures`, `TestRefreshLockRetryNonLockFailures`. Include fetch, pull, permission errors, authentication, invalid refs, and misleading lock text. |
| .30; platform 002.3, 002.4 | Same file: `TestRefreshLockRetryBudget`, `TestRefreshLockRetryCancellation`. Hold initial admission, then prove one deadline across attempts and backoff. Assert slot release and no command after cancellation. |
| .31, .34 | `manager_refresh_lock_test.go`: `TestCreateWorktree_RefreshLockReleased`, `TestCreateWorktree_RefreshLockPreservesEdits`. Use a real partial fast-forward and fixture-owned lock. |
| .33, .14; platform 002.5 | `refresh_lock_retry_test.go`: `TestRefreshLockRetryExhaustion`, `TestRefreshLockRetrySafeDiagnostics`. Assert attempt count, fixed text, cause identity, and credential exclusion. |
| .30, .32, .33 | `refresh_lock_retry_test.go` and `manager_refresh_lock_test.go`: `TestRefreshLockRetryAbortsOnCheckoutSwitch`, `TestPullCurrentBranch_RetryTimeoutPreservesDeadline`, `TestPullCurrentBranch_ExhaustedLockPreservedWithRefSelectionFailure`. Assert no pull on a switched checkout, preserved deadline identity, and combined selection plus lock failure. |
| .6, .10, .13, .32; platform 002.6 | `manager_refresh_lock_test.go`: `TestCreateWorktree_RefreshLockPolicy`. Include required PR and remote-only failure, best-effort warning, and cancellation. Retain the existing policy tests. |

The transient-retry and real partial-fast-forward tests must fail before implementation.
Before implementation, the refresh commands returned the first lock failure instead of attempting recovery.

## End-to-end evidence

The real Git tests enter `Manager.Create`, refresh the base, and validate the resulting usable worktree.
The persistent required-lock case must produce no successful worktree and must report failed preparation.
These tests exercise Git and materialization together, without a mocked successful refresh.

No rendered UI, navigation, or viewport-dependent behavior changes.
The mobile skill's backend-logic exception applies, so no new browser scenario is required.
The existing error surface receives the same backend result on desktop and phone.

## Companion package inventory

- `worktree-refresh-fail-closed`: retain strict PR freshness and its completed launch-error evidence.
- `recovery-diagnostics`: retain the fixed classifier and secret exclusion.
- `git-invalid-ref-diagnostics`: invalid refs remain distinct from lock contention and are not retried.
- `explicit-worktree-base-selection` and `fork-pr-base-resolution`: retain selection and repository identity.

Their recorded implementation results remain historical.
This package neither marks those work orders pending nor claims to rerun their original browser matrices.

## Work orders

- [x] [Task 01: Recover brief base-refresh lock contention](task-01-recover-refresh-locks.md)

## Verification results

Live repair: completed and verified on 2026-11-08 at 23:12 UTC.
Design-package checks passed:

- `python3 scripts/list-docs.py validate`: 440 decisions and 1,947 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Documentation coverage preflight: the current documentation-only diff was exempt.
  The planned implementation paths and linked work order reported `covered` with no errors.
- `git diff --check`: passed. Both new plan files remain unstaged and uncommitted.

Disposable Git proofs passed for a clean checkout and independent staged/unstaged edits.
Each initial pull returned 128 with a partial update. Each repeated pull completed after lock release.
Implementation and regression tests completed in the work order; see its results for exact commands.
The final implementation diff reported `covered` in the documentation coverage preflight.

## Risks

- Git output varies by version and locale. Unrecognized output must retain the existing single-attempt behavior.
- A partial update can survive exhausted retries. An error must not promise rollback or authorize destructive cleanup.
- Initial admission and later execution budgets must remain distinct. Reusing the canceled first-attempt context breaks recovery.
- A remote branch can change between attempts. Each normal command and existing required-ref checks remain authoritative.
