---
id: "02-bound-deletion-coordination"
title: "Bound deletion coordination and prove compatibility"
status: done
wave: 2
depends_on:
  - "01-release-settlement-guards"
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.1
  - AC-TASKS-RUNTIME-CLEANUP-001.5
  - AC-TASKS-RUNTIME-CLEANUP-001.6
  - AC-TASKS-RUNTIME-CLEANUP-001.25
  - AC-TASKS-RUNTIME-CLEANUP-001.42
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.2
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
---

# Task 02: Bound deletion coordination and prove compatibility

## Summary

Make deletion coordination observe the original deadline without a waiter that acts after timeout.
Propagate failed stop admission before destructive task deletion and preserve exact recovery evidence.
Prove the existing desktop and phone cancellation outcomes against a fresh managed build.

## In scope

- Add a zero-value context-aware mutex over the existing weighted semaphore dependency.
- Cover exclusion, `TryLock`, cancellation before acquisition, cancellation during contention, and retry after release.
- Replace the shared cascade mutex and the execution mutexes used for remote stop and pre-stop checkpoint admission.
- Bound the cancellation prompt-identity read by its supplied context while it waits for the same prompt mutex.
- Preserve ordinary `Lock` and `Unlock` semantics for existing callers.
- Bound the whole checkpoint phase by the existing capture budget and earlier caller deadline.
- Return preservation errors before runtime deregistration, provider teardown, or destructive resource cleanup.
- Propagate normal verified-delete stop errors and compensate only this attempt's uncommitted prepared jobs within the original budget.
- Preserve retain-mode and accepted-unknown-change deletion with pending shutdown.
- Use a bounded compensation capability without changing ordinary detached cleanup transitions.
- Preserve archive policy and the existing partial-cascade committed-member behavior.
- Adapt fixture construction and stack-name assertions affected by the mutex type change.
- Run the existing desktop and phone user-flow specs after the backend changes.

## Out of scope

Do not change timeout values, schemas, provider stop semantics, UI composition, or UI copy.
Do not retry the live operator deletion, restart the live backend, or remove its runtime inventory or files.
Do not promote the entire draft runtime-cleanup specification after this focused work order.

## Acceptance

1. The held root, remote-stop, prompt-identity, and prompt-checkpoint lock cases return by their supplied deadlines.
   An expired waiter never later becomes an owner or mutates task state.
   Releasing the original owner permits a fresh authorized request to proceed.
2. For normal verified deletion, failed synchronous stop admission preserves the task, runtime handles, and prepared snapshot without deletion or physical removal.
   Compensation cancels only this attempt when its remaining budget permits it.
   An unavailable store preserves an unknown outcome for existing reconciliation.
   A mixed cascade retains obligations for every committed member and preserves each uncommitted blocked member.
3. Task 01's regressions, focused stop and cascade tests, the backend build, and the named desktop/phone E2E flows pass.
   Record real discovery counts and results. Do not treat an empty test selection as evidence.

## Verification

Run from the repository root. Run each command sequentially.
Install workspace dependencies once if this worktree lacks them.
The managed E2E runner builds the production web/backend fixtures and enforces resource limits.

```bash
(cd apps/backend && go test -trimpath -race ./internal/common/ctxmutex -count=1 -timeout=90s)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test(CancellationPromptIdentityReadHonorsDeadline|StopAgentCheckpointAdmissionHonorsDeadline|.*StopAgent|.*TurnChange|WithPromptLifecycleLock|LateCompletionCoalescer|TransientTurnFailure|StreamCoalescer|.*DispatchCompletion|StopWaitsForRetriedTerminalCaptureBeforeReturning|StopPreservesActiveTurnBeforeExecutorCleanup|PassthroughResumeReadinessDefersContendedProcessLocks|MarkReadyDoesNotDeadlockWithPromptCompletion|TerminalCapture|StreamDisconnectFinishesAdmittedTurnBeforeErrorPublication)' -count=1 -timeout=10m)
(cd apps/backend && go test -trimpath -race ./internal/task/service -run 'Test(DeleteTaskTreeLockDeadlineAndRetry|DeleteTaskTreeStopFailurePreservesPreparedIntent|DeleteTaskRecoveryModesRetainUnconfirmedShutdown|CancelPreparedCleanupPreservesOriginalDeadline|.*DeleteTask|.*Cleanup.*|.*Archive.*)' -count=1 -timeout=15m)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator ./internal/backendapp -run 'Test(CancelAgent|RetainedTurnFailure|.*Cancellation|.*Cancel.*Review|.*RetainedPrompt)' -count=1 -timeout=10m)
make -C apps/backend build
(scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/workflow/workflow-cancel-completion.spec.ts tests/chat/queued-prompt-cancel.spec.ts tests/task/tasks-page-delete-cleanup.spec.ts)
(scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/workflow/mobile-workflow-cancel-completion.spec.ts tests/chat/mobile-queued-prompt-cancel.spec.ts tests/task/mobile-tasks-page-delete-cleanup.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Before the production changes, run each new deadline regression and record its expected failure.
After the final production edit, rerun the required work-order commands.
Run changed-file Go lint from the repository root with the actual changed Go paths.

## Files likely touched

- `apps/backend/internal/common/ctxmutex/mutex.go`
- `apps/backend/internal/common/ctxmutex/mutex_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_stop_deadline_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/execution_store_prompt_lock_test.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/handoff_delete_deadline_test.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lock_order_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_resume_reuse_test.go`
- Existing fixture literals affected by the private mutex type change.
- The two amended system designs and this plan's results.
- `docs/public/tasks-and-workflows.md` for the normal-deletion failure outcome.

The named E2E files provide existing coverage. Change them only if the backend regression exposes a missing assertion.
Any changed E2E file must receive its focused managed run.

## Dependencies

Task 01 must pass first. Both work orders change cancellation/deletion coordination and run sequentially.

## Risks

- `preserveTurnChangesBeforeStop` currently returns no error. Every caller must preserve stop safety after its signature changes.
- A deadline must cover mutex admission and checkpoint work without creating a fresh budget for compensation.
- The shared root cascade lock protects archive, unarchive, and deletion. Preserve the mutex identity for each root.
- Existing tests that inspect `sync.Mutex` stack frames require equivalent ownership assertions after the type changes.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md) and Task 01 results.
- [Bounded cleanup coordination design](../../specs/tasks/system-design/runtime-cleanup-preparation.md#bounded-coordination-before-deletion).
- [Cleanup requirement](../../specs/tasks/requirements/runtime-cleanup.md) and [cancelled-turn requirement](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md).
- `cancel_in_flight_mutex.go` for existing cancellation semantics and `golang.org/x/sync/semaphore` for bounded acquisition.
- Existing retained-stop, turn-change capture, cascade compensation, and prepared-cleanup recovery tests.

## Results

Implementation followed the completed design handoff.
The root-lock waiter, three runtime-stop admission cases, bounded cleanup compensation, and prompt-identity read each failed before their corresponding production repair.
The prompt-identity regression reported an ignored deadline while its owner retained the prompt mutex.
The recovery-mode regressions exposed an overbroad stop-error policy. Restricting that policy to normal verified deletion preserves their existing behavior.
The cleanup compatibility suite also required retaining detached transition semantics outside the new request-owned compensation method.
After the final production edit, the race-enabled selections passed:

- Context-aware mutex: 3 top-level tests.
- Lifecycle: 79 top-level tests, including the prompt-identity, stop-admission, capture, coalescer, and adapted lock-order cases.
- Task service: 277 top-level tests, including the mixed partial cascade, original detached cleanup transition, and both explicit recovery modes.
- Orchestrator: 112 top-level tests, including both actual acknowledgement callers and the existing cancellation contracts.
- Backend application: 11 top-level tests.

The selections discovered and passed 482 top-level tests. No selection was empty.
`make -C apps/backend build` passed after the final production edit.
The fresh managed Chromium run passed four tests in 52.5 seconds.
The subsequent mobile-chrome run reused that build and passed three tests in 48.1 seconds.
These browser cases cover workflow cancellation policy, queued-prompt pause, and deletion with pending cleanup.
No production or E2E source changed between these browser runs.

The detailed race logs, build record, and browser summaries are retained privately outside the repository.
The task and its session remain absent in the final read-only database check.
The investigation task, failed cleanup intent, backup, and deletion receipt remain intact.

All four changed Go packages passed `scripts/lint-go-changed` with zero issues.
The lint record lists every supplied changed or new Go path.
The final catalog check validated 431 decisions and 1920 specifications.
Specification lint and local documentation coverage passed, covering both work orders with three accepted requirement references.
The public-documentation tests passed all 63 cases, and all 55 published pages validated.
`git diff --check` passed. Changes remain uncommitted.
