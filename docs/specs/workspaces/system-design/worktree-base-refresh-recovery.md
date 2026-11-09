---
status: current
system: workspaces
created: 2026-11-08
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
owners:
  - kandev
---

# Worktree refresh lock recovery

## Purpose and boundaries

Workspaces owns base refresh and the resulting worktree selection.
This design extends the [base-refresh design](worktree-base-refresh.md).
It covers only backend base-fetch and fast-forward-pull commands in `internal/worktree`.
The command policy, not the executor's display name, determines coverage.

The existing design is near its size limit, so this part owns failure recovery.
It does not replace the existing selection or credential contracts.
The [decision](../../../decisions/2026-11-08-worktree-refresh-lock-retries.md) records the retry boundary.

## Requirement mapping

| Criteria under REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001 | Design section |
| --- | --- |
| .29, .30 | Execution and deadline |
| .31, .34 | Partial fast-forwards |
| .32 | Policy and compatibility |
| .33, .14 | Errors and diagnostics |

## Components

Add a private `runRefreshGitWithLockRetry` helper in
`apps/backend/internal/worktree/refresh_lock_retry.go`. This is a proposed symbol.
Call it from `pullBaseBranchWithPolicy` and `pullCurrentBranchOrFallback` in
`manager_git.go` instead of their single-attempt network runner.
Other `runGitCombinedAfterAcquire` callers remain unchanged.

Reuse `classifyRefreshDiagnostic` from `refresh_diagnostics.go` for known causes.
Only a failed Git command with `lock_contention` permits another attempt.
Cancellation and timeout take precedence over output classification.
Unknown output and an exit code of 128 alone do not permit a retry.
An incidental lock filename in a URL or warning must not identify retryable contention.
Keep permission-denied lock creation separate from file-exists or reference contention.

Reuse `subproc.RunGitCombinedAfterAcquire` with `subproc.GitLifecycle` for every attempt.
Retain `newNonInteractiveGitCmd`, the selected environment, and tokenized arguments.
Do not add a generic retry framework or change `internal/common/subproc`.

## Execution and deadline

Allow one initial attempt and at most three retries.
Use delays of 200 ms, 400 ms, and 800 ms, for 1.4 seconds of total backoff.
Keep these values private constants, without new configuration.

The first attempt waits for admission under the existing caller lifetime.
Inside its command builder, capture the execution deadline after admission.
After a lock failure, derive a retry context from the caller and that captured deadline.
Do not derive it from the completed attempt's canceled context.
Later commands, admission waits, and backoff share the captured deadline.
An earlier parent deadline remains authoritative.

Each attempt constructs a new command and completes managed cleanup before the next attempt.
The runner releases its admission slot before backoff.
Use a context-aware timer, not `time.Sleep`.
Return immediately after success, a non-lock failure, cancellation, or budget exhaustion.
Never restart the full session launch to retry one Git command.

Keep the existing repository lock across base preparation.
This mutex does not exclude external Git processes or every application Git writer.
Git's own locks remain authoritative. A new global lock manager is outside this package.

## Partial fast-forwards

A failed pull can update the index and files before its branch-ref update fails.
Repeating the existing fast-forward-only pull lets Git complete that update after contention ends.
Git performs its normal checks for local edits and incompatible history.
The normal base-ref and ancestry checks still govern a successful refresh.

Do not reject a retry solely because the index contains staged changes.
Do not clear the index, reset files, or directly update the branch ref.
Do not run a separate merge or repair command in production.
This design adds re-execution of existing commands, not a second reconciliation engine.

When contention remains, return the bounded failure without claiming that the checkout rolled back.
A later user retry can use the same task and original base selection.
Independent local changes, divergence, or a new non-lock error retain the existing failure policy.
No lock file age, empty process list, or missing open descriptor authorizes automatic lock removal.

## Policy and compatibility

| Route | Behavior |
| --- | --- |
| Backend `pullBaseBranchWithPolicy` fetch | Bounded lock retries, followed by the existing required or best-effort decision. |
| Backend `pullCurrentBranchOrFallback` pull | Same retry budget, including a partially completed fast-forward. |
| Provider callback with `RemoteSyncHandled` | Existing provider policy. No second refresh or new credential route. |
| Qualified PR base through `materializeQualifiedPRBase` | Existing `gitbase` policy and identity checks. No retry expansion in this package. |
| SSH, container, or Kubernetes materialization outside these two methods | Existing executor behavior. No claim of automatic recovery. |
| Worktree creation, ref repair, push, agent Git, and background refresh | Existing behavior. No general Git retries. |

Required PR refresh remains strict after exhausted retries.
Best-effort local refresh retains its warning and local-base behavior.
The `reason` used for fallback remains separate from the diagnostic category.
Lock contention does not qualify as a proven missing remote ref.
Each repository keeps its own preparation outcome before agent startup.

## Errors and diagnostics

Use a small private error for exhausted lock contention.
Its `Error()` text uses fixed wording, the operation, and the attempt count.
It identifies a busy repository and permits a later launch retry.
It never asserts that a lock is stale or names an unknown owner.
Its cause preserves `ErrGitCommandFailed` and the final process error for error inspection.
Context errors pass through without this replacement.

`syncFailureCause` must preserve this credential-safe error instead of discarding its detail.
Required fetch and pull failures must carry it through `failRequiredSync` and normal lifecycle wrapping.
Do not change error shapes, cancellation identities, or fallback selection.

Use the existing structured diagnostic with an additional attempt count for exhausted retries.
A debug retry entry can record operation, repository path, branch, and attempt count.
Reuse fixed diagnostic codes and explanations. Do not log complete argv, remote URLs, or raw output.
Do not persist failed-attempt buffers or introduce new metrics.

The current preparation and launch-error surfaces carry the result on desktop and phone.
No component, navigation, control, WebSocket schema, or frontend copy changes are planned.
The mobile skill's backend-logic exception applies. Backend tests verify the shared error payload.

## Verification and delivery

Use scripted command failures for exact attempts, deadline, admission, cancellation, and diagnostic assertions.
Use disposable real Git repositories for the partial-fast-forward regression.
Hold a fixture-owned branch-ref lock for the first pull, then release it before retry.
Prove that the index advances before the initial failure and that retry finishes the branch update.
Include independent edits and a required-refresh failure with no usable resulting worktree.

The [plan and work order](../../../plans/git-refresh-lock-recovery/plan.md) own exact checks and results.
No schema, settings, background repair job, or persistent recovery record is required.
