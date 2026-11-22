---
created: 2026-11-05
status: done
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
legacy_specs: []
---

# Fix plan: Cancellation completion and task deletion deadlock

## Overview

Remove the circular wait between cancellation acknowledgement, late completion, and ordered stream publication.
Then make the relevant deletion lock waits observe the existing operation deadline.
Two sequential work orders cover the regression and implementation.
Both work orders are complete. The changes remain uncommitted for review.

## Ownership and requirement conformance

Tasks own this repair because Tasks own cancellation settlement, runtime stop, and deletion admission.
The existing [cancelled-turn requirement](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md)
already requires bounded cancellation and ordered terminal persistence under criterion 001.1.
Criterion 001.2 retains runtime reconciliation and sibling precedence.
No cancellation policy or workflow setting changes.

The existing [cleanup requirement](../../specs/tasks/requirements/runtime-cleanup.md)
covers stop confirmation, recovery evidence, and the original work deadline.
This requirement remains draft. This package does not promote its unrelated archive-cascade work.
Its applicable criteria are 001.1, .5, .6, .25, and .42.

The paired designs record the implemented amendments.
The existing explicit-cancellation and backend-owned-progress ADRs remain authoritative.
This repair restores their boundaries. It introduces no new persistence or public contract.

## Evidence and root cause

The private goroutine capture showed cancellation escalation, a late stream completion, and deletion waiting for runtime cleanup.
Cancellation and completion formed this cycle:

| Goroutine | Holds | Waits for |
| --- | --- | --- |
| Cancellation acknowledgement | Session guard | `promptLifecycleMu`, in `AcknowledgeRetainedPromptFailure` |
| Late completion | `promptLifecycleMu` | Coalescer `emitMu`, through `flushMessageBuffer` |
| Timer publication | Coalescer `emitMu` | Session guard, through synchronous `handleAgentStreamEvent` |
| Deletion | Root cascade mutex | Prompt mutex, in `preserveTurnChangesBeforeStop` |
| Delete retries | Request context | The same root cascade mutex |

The two-minute context could not interrupt these `sync.Mutex.Lock` calls.
The separate start-checkpoint timeout is supporting context, not the proven lock cause.

The supporting logs and database evidence remain private and are not part of this change.

## Completed operator action

The reported task was removed from local SQLite after a consistent backup and a guarded, foreign-key-enabled transaction.
The transaction removed exactly one task and its session, messages, environment, search entries, queue state, and PR watch.
It retained delivery retirement and turn-history cleanup obligations.
It retained the unconfirmed runtime inventory and marked the prepared resource job failed with an explicit operator diagnostic.
The runtime was not reported stopped, and no workspace files were removed directly.

The deletion receipt is retained with the private evidence.
This action does not prove the permanent fix or release the existing in-memory deadlock.
Do not repeat it during implementation or use the live database for regression tests.

## Scope

### In scope

- Release physical session guards before both retained-settlement acknowledgement calls.
- Preserve logical cancellation ownership, exact generation checks, ordered output, and successor fencing.
- Bound root cascade, cancellation prompt-identity reads, runtime-stop admission, and pre-stop checkpoint waits.
- Propagate delete stop failures and preserve matching cleanup evidence.
- Add deterministic backend regressions and run existing desktop and phone cancellation flows.

### Out of scope

- New UI, translated copy, runtime flags, schemas, provider adapters, or public endpoints.
- Asynchronous event-bus delivery, discarded stream frames, or removal of serialization.
- Changed cancellation escalation durations or automatic queued-message replay.
- Restarting the live backend, removing its workspace files, or implementing an operator deletion command.
- Broad archive-cascade redesign or promotion of unrelated draft requirements.

## Technical approach

### Work order 01: Release the settlement guard

Use `lockedCancelInFlightGuard.unlock` before lifecycle acknowledgement in `runExplicitCancellationOwned`.
Keep the cancellation registry entry and projection reference until the owning operation settles.
Reacquire with the original operation context before any remaining guarded mutation, then revalidate its captured identity.

Release the retained-failure handler's guard after durable settlement and before `acknowledgeRetainedTurnFailure`.
Keep `Manager.AcknowledgeRetainedPromptFailure` execution and generation checks.
Do not clear a replacement execution's gate or admit a successor before settlement.

Prove both actual orchestrator callers release their guards.
Separately exercise the real lifecycle completion handler, coalescer, and synchronous bus with the three-way schedule.
Do not claim that a mock acknowledgement alone reproduces the stream cycle.

### Work order 02: Bound deletion coordination

Add a small context-aware mutex over the existing weighted semaphore dependency.
Preserve zero-value construction and ordinary lock methods for unaffected callers.
Use it at the shared cascade lock and execution locks needed by runtime stop.
Use context-aware acquisition when cancellation reads the prompt generation under its session guard.
Adapt existing tests that inspect mutex stack names without weakening their ownership assertions.

Make checkpoint preservation return its admission or capture error to `StopAgentWithReason`.
Apply the existing capture budget to the whole checkpoint phase.
Keep the caller's earlier deadline and preserve execution registration after failure.

Make normal verified deletion return synchronous stop errors before membership or task deletion commits.
Retain mode and accepted unknown changes preserve their existing committed-deletion and pending-shutdown behavior.
Retain archive's existing behavior through a separate call policy.
Compensate only the failed attempt's prepared intents within the original budget.
Use the bounded compensation capability. Keep ordinary durable cleanup transitions independent from expired callers.
Preserve unknown outcomes for existing reconciliation when persistence itself is unavailable.
Do not claim a stop or workspace removal after an admission timeout.

### Compatibility matrix

| Path | Identity | Required behavior | Evidence |
| --- | --- | --- | --- |
| Explicit and joined cancellation | Operation, execution, generation, turn | One owner, ordered output, pending clears | Service tests and real lifecycle cycle regression |
| Retained turn failure | Captured execution and generation | Durable settlement precedes acknowledgement outside the guard | Retained-failure guard regression |
| Worktree/local deletion | Exact task, execution, prepared intent | Complete or return a bounded error before unsafe removal | Lifecycle and cascade regressions |
| Retain mode and accepted unknown changes | Explicit options and durable cleanup inventory | Preserve committed deletion with pending shutdown | Recovery-mode compatibility regression |
| Remote executors | Same shared manager locks and provider stop contract | Preserve remote handles when admission expires | Shared-manager tests, existing stop compatibility tests |
| Archive and unarchive | Shared root mutex and existing cascade identity | Deadline-aware admission preserves serialization | Mixed archive/delete waiter test |
| Desktop and phone | Existing cancellation projection and controls | Existing pause and completion policy remains | Existing Chromium and mobile-chrome specs |

These tests establish shared coordination behavior. They do not certify live remote providers.

## Tests

| Regression | Location | Coverage |
| --- | --- | --- |
| `TestCancelAgent_SettlementAcknowledgementReleasesGuard` | `orchestrator/cancellation_completion_lock_order_test.go` | Cancellation 001.1 and .2 |
| `TestRetainedTurnFailure_AcknowledgementReleasesGuard` | Same file | Cancellation 001.1 and .2 |
| `TestLateCompletionCoalescerAndSettlementAcknowledgementProgress` | `agent/runtime/lifecycle/completion_cancel_coalescer_test.go` | Cancellation 001.1, ordered output and successor isolation |
| `TestContextMutexDeadlineDoesNotAcquireLater` | `common/ctxmutex/mutex_test.go` | Cleanup 001.42 |
| `TestStopAgentCheckpointAdmissionHonorsDeadline` | `agent/runtime/lifecycle/manager_stop_deadline_test.go` | Cleanup 001.1, .5, .25, .42 |
| `TestCancellationPromptIdentityReadHonorsDeadline` | Same file | Cancellation 001.1 |
| `TestStopAgentCaptureWorkHonorsDeadline` | Same file | Cleanup 001.5, .25, .42 |
| `TestDeleteTaskTreeLockDeadlineAndRetry` | `task/service/handoff_delete_deadline_test.go` | Cleanup 001.6 and .42 |
| `TestDeleteTaskTreeStopFailurePreservesPreparedIntent` | Same file | Cleanup 001.1, .6, .25, .42 |
| `TestDeleteTaskTreePartialFailureRetainsCommittedCleanup` | Same file | Cleanup 001.6 and .25 |
| `TestDeleteTaskRecoveryModesRetainUnconfirmedShutdown` | Same file | Existing deletion recovery contract |
| `TestCancelPreparedCleanupPreservesOriginalDeadline` | Same file | Cleanup 001.42 |

Use channels or `testing/synctest` to establish ownership.
Tests must drain their goroutines after the expected RED failure.
A bounded subprocess is suitable when the original production mutex cycle cannot unwind safely.
Cover stale execution, stale generation, duplicate cancel, one working sibling, and unrelated-task progress.
Cover a mixed cascade where one member committed and a later member remains blocked.

## E2E tests

The shared cancellation controls keep their existing composition and copy.
No rendered UI change or new mobile interaction requires a preview.
Run the workflow cancellation, queued cancellation, and tasks-page deletion specs in Chromium.
Run their `mobile-` counterparts in mobile-chrome, sequentially through the managed host runner.
The phone run reuses the fresh desktop build while production source remains unchanged.
These flows prove existing user outcomes after the deterministic backend regression passes.
Do not use browser timing as proof of the three-lock schedule.

## Work orders

- [x] [Task 01: Release settlement acknowledgement guards](task-01-release-settlement-guards.md)
- [x] [Task 02: Bound deletion coordination and prove compatibility](task-02-bound-deletion-coordination.md)

Execute Task 01, then Task 02 in the primary conversation.
No subagents are authorized.

## Verification results

Design checks passed: the catalog validated 431 decisions and 1920 specifications.
Specification lint passed, and all 36 specification-linter tests passed.
Local documentation coverage accepted both work orders against the planned implementation paths.
Local links, untracked work-order presence, and whitespace checks passed.
After the final production edit, all 482 selected backend tests passed with the race detector.
The backend build passed. The fresh managed browser run passed four Chromium tests and three mobile-chrome tests.
All four changed Go packages passed lint with zero issues.
The public-documentation tests passed all 63 cases, and all 55 published pages validated.
Final catalog validation, specification lint, and local documentation coverage passed.
Task 02 records the exact selections and private log paths.
The direct database deletion has its separate committed receipt and read-back evidence.

## Related delivery records

The completed [queued-prompt pause package](../queued-prompt-pause-deadlock/plan.md)
owns earlier admission and guard-deadline repairs.
The [dispatch completion package](../dispatch-completion-wakeup/plan.md)
owns completion wakeup and failure-publication ordering.
Their historical results remain unchanged. This package owns the newly observed acknowledgement cycle and deletion waits.

## Risks

- A guard release without identity revalidation can change a successor's state.
- A semaphore migration can invalidate stack-name-based tests without changing their intended invariant.
- A stop timeout must retain exact cleanup evidence and cannot authorize destructive resource removal.
- An unavailable database can prevent compensation. Preserve the prepared snapshot for reconciliation and report the error.
- The already-deadlocked live process remains outside this code package's operational scope.
