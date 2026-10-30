---
id: "01-bound-terminal-processing"
title: "Bound terminal turn-change processing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.2
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.7
  - AC-TASKS-TURN-CHANGES-007.1
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 01: Bound terminal turn-change processing

## Summary

Bound terminal capture/summary retries and stop proven missing-set or ownership
loss without mutating a successor's live interval. This work order records the
implemented source in PR #4785 and its review correction. The correction has focused verification recorded below; historical evidence
remains scoped to its original source.

## In scope

Lifecycle phase attempt accounting, typed permanent-error handling, exact-owner
unavailable abandonment, summary publication for an absent interval and the
existing terminal ownership-test expectation. Preserve the failed-start repair,
valid summary/checkpoints and normal completion/failure/route/queue authority.

## Out of scope

Production data repair, forced live errors or restarts, new settings/UI/schema,
retention policy, completion gates, provider routing and new standing test files.
The review correction changes only the existing lifecycle retry/summary paths
and existing documentation. Coordinator and canonical publisher APIs stay unchanged.

## Acceptance

1. Capture and summary each end within three logical attempts, counting inline work, with
   250/500 ms cancellable waits. Typed permanent loss stops immediately; joined
   storage failures remain visible and bounded.
2. Terminal processing validates the exact turn, execution, environment, startup
   attempt and generation. Abandonment uses revision/claim guards, preserves
   accepted refs/content and finalized summaries, and cannot settle or clean
   another live owner. Failed persistence does not authorize fence release.
   Once capture stops, manager-owned continuation retries only abandonment
   until its existing durable/no-op result, shutdown or replacement ownership.
   It uses cancellable backoff capped at the existing five seconds; it does not
   restart capture or expire the fence. A stored unavailable result enters the
   normal summary phase after safe fence completion, so publisher failure does
   not reopen the capture fence.
3. Existing versioned completion/failure owners still settle the originating
   turn and managed input. Ordinary route and queued-prompt admission continue
   through their existing guards rather than a synthesized success path.

## Files likely touched

These are the six actual implementation paths in the published change:

- `apps/backend/internal/agent/runtime/lifecycle/manager_turn_change_retry.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go`
- `apps/backend/internal/backendapp/turn_changes_runtime.go`
- `apps/backend/internal/task/changes/coordinator_abandon.go`
- `apps/backend/internal/task/changes/coordinator_finish.go`
- `apps/backend/internal/task/changes/coordinator_test.go`

## Requirement-to-evidence mapping

| Acceptance criterion | Evidence at this repair boundary |
| --- | --- |
| AC-TASKS-TURN-CHANGES-003.2 | Exact terminal-owner validation; original-only abandonment; startup/generation fence checks |
| AC-TASKS-TURN-CHANGES-003.3 | Delayed old terminal preserves newer live owner; duplicate completion does not create another retry job |
| AC-TASKS-TURN-CHANGES-003.6 | Real originating-turn Ready settlement, `try_next` and next queued `PromptAgent` through normal orchestration |
| AC-TASKS-TURN-CHANGES-004.1 | Valid summary/continuation controls; completed summaries and accepted endpoint/content retention during abandonment |
| AC-TASKS-TURN-CHANGES-004.7 | Owned exhausted capture persists unavailable/incomplete; missing history is not published as a ready zero result |
| AC-TASKS-TURN-CHANGES-007.1 | Persistent failure exhausts three attempts, exact 250/500 ms waits, permanent-loss early stop and shutdown cancellation |

## Verification

The following existing focused commands were executed during the original
implementation from `apps/backend`. Their results remain historical evidence
for unchanged code. The review correction requires only the affected lifecycle
controls and disposable fault cases recorded in the correction results below.

```text
go test -trimpath ./internal/task/changes -run 'TestCoordinator(SettlesUnacceptedStart|AcceptsTerminalEndpointsBeforeSummaryProcessing|RejectsTerminalFromMismatchedOwnership|KeepsBaselineForValidSameTurnContinuation|SettlesCaptureDeadlineAndPersistenceFailuresBeforeSuccessorAdmission)$' -count=1
go test -trimpath ./internal/agent/runtime/lifecycle -run 'Test(TerminalCaptureRetriesPersistenceAndReleasesFence|TerminalCaptureReleasesFenceBeforeSummaryProcessing|TerminalCaptureFenceBlocksSuccessorAdmission|TurnCaptureAdmissionWaitCancellationPreservesFence|TurnChangeCapturePreservesShutdownDeliveryOutcome)$' -count=1
go test -trimpath ./internal/orchestrator -run 'Test(ApplyRouteActionRejectsActiveTurn|ApplyRouteActionRunsAfterTurnSettles|AgentTurnFailedSettlesDurableErrorWithoutStoppingRuntime|RetainedFailureCompleteStreamDefersSettlementToTurnOwner|AgentTurnFailureStorageErrorKeepsSessionAdmissionClosed|DrainQueuedMessage_LifecycleEntryRemainsDurableUntilPromptAcceptance)$' -count=1
```

Run only the following documentation checks from the repository root for this
follow-up, plus the existing coverage module against the actual combined PR
paths and linked document contents. Require `ok: true`, `status: covered` and no
errors. Normal commit hooks remain required.

```text
python -X utf8 scripts/list-docs.py validate
python -X utf8 scripts/lint-spec-files.py --all
git diff --check -- docs/plans/turn-change-summary-retry
git status --short -- docs/plans/turn-change-summary-retry
```

## Dependencies and inputs

None. Reuse [requirements](../../specs/tasks/requirements/turn-changed-files.md),
[system design](../../specs/tasks/system-design/turn-changed-files.md),
[immutable intervals](../../decisions/2026-10-07-immutable-turn-change-intervals.md),
and [prompt generation ownership](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md).

## Parallelism

`sequential`

## Results

Original implementation on Windows/amd64 with Go 1.26.0:

- RED: persistent failures made six calls in ten seconds of virtual time,
  instead of the expected three; typed missing/relationship losses also retried.
  The real SQLite/task-service publisher returned a missing-set error, and a
  delayed terminal was accepted against a newer live generation.
- GREEN: the coordinator group, including unaccepted-start, accepted endpoints,
  continuation, ownership and abandonment cases, passed in 0.291 s. An initially
  over-broad claim guard was corrected to preserve unavailable settlement for
  the exact unaccepted-start owner without a competing terminal claim.
- Real SQLite/publisher, rejected relationship, normal task-service completion
  and joined-error preservation passed in 0.530 s.
- Lifecycle focused and temporary cases passed in 1.192 s. Additional inline
  budget, new-startup and duplicate-completion cases passed in 0.093 s.
- Orchestration evidence passed in 1.926 s, including missing/rejected admission,
  versioned Ready, persisted original `completed_at`, `try_next` and actual next
  queued prompt dispatch. No orchestration production code changed.
- Scoped stale-generation/shutdown race cases passed in 1.310 s; the existing
  stop/retried-capture join passed in 1.323 s. Changed-package lint reported zero
  issues and normal commit hooks passed.
- Additional fault cases used explicit temporary Go overlays outside automatic
  collection. They were executed and removed; they are not permanent test files.
  No live database/process mutation or forced production failure was performed.
- Independent formal review approved the original implementation. Its optional
  suggestion to release a fence despite failed abandonment persistence was not
  adopted: durable settlement remains required before successor admission.

The reported aggregate warning rate and exact production STARTING stall cause
were not reproduced locally. These focused results do not claim full backend,
PostgreSQL, live-provider or parent-feature qualification.

Documentation follow-up on 2026-10-26: catalog validation passed (403 decisions,
1,736 specifications), specification lint and whitespace checks passed. The
unchanged coverage module evaluated the actual eight changed paths and four
linked document bodies, returning `covered`, `ok: true`, `errors: []`, and the
expected work order and REQ-TASKS-TURN-CHANGES-003/004/007 design references.

Review correction prepared on 2026-10-27: both permanent inline capture loss and
exhausted background capture now retain manager-owned settlement continuation
after abandonment storage failure. Successful abandonment enters the existing
summary phase after original-owner fence/anchor cleanup. Capture and summary
keep their separate three-attempt budgets and all-leaves permanent-error rule;
settlement-only persistence retries do not ask for new checkpoints. Existing
coordinator no-op results and canonical row/revision publication are preserved.
No publisher owner parameter, new policy, schema, recurring service, TTL fence
release, or permanent test case was added.

Root verification for the correction:
- The disposable baseline failed on both storage-recovery paths and on skipped
  summary notification, establishing the reported defects.
- The candidate passed all five causal scenarios, including unchanged capture
  counts, settlement recovery, summary failure/retry after fence release,
  shutdown, and replacement ownership.
- The candidate and the six existing lifecycle controls passed again on the
  actual publication checkout. The same selected cases passed with the race
  detector, alongside the existing coordinator persistence/settlement and
  compact summary projection tests.
- The temporary cases use package-local fakes and virtual time. They establish
  lifecycle phase/fence behavior; no installed provider/browser observation is
  claimed. No permanent test case or verification framework was added.
- Formatting completed. Normal hooks, updated-head hosted CI and readback are
  recorded by the integrator after publication; the original CI result is not
  reinterpreted as a new pass.

## Risks

Keep unavailable settlement distinct from successful ready history. Preserve
the original completion/failure owner and never adopt another live interval
merely to stop a retry. Document-only delivery must not alter those contracts.
