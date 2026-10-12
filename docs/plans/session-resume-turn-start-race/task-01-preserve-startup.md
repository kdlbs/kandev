---
id: "01-preserve-startup"
title: "Preserve startup during workflow turn-start"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
acceptance_criteria:
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.3
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.4
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.5
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.7
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.8
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.10
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.11
system_design:
  - ../../specs/tasks/system-design/resume-prompt-queue.md
---

# Task 01: Preserve startup during workflow turn-start

## Summary

Prevent workflow turn-start preparation from settling a recipient's active
startup or admitted turn. Prove the correction at the resume credential
boundary before adding browser evidence.

## In scope

- A workflow-specific recipient state-preparation helper with strict conditional publication.
- Engine, legacy, and turn-start WIP paths.
- Fresh recipient reads, stale snapshots, lost writes, terminal outcomes, and read errors.
- Deterministic resume-persistence and actual message-delivery regressions with TDD.
- Preservation of routing, startup-attempt identity, queue policy, and genuine failures.

## Out of scope

- Changes to provider adapters, credential authorization, persistence schema, or resume guards.
- Global replacement of `setSessionWaitingForInput` or changes to completion settlement.
- Layout, copy, public APIs, historical replay, and browser tests.

## Acceptance

1. The new credential-boundary regression fails on the current workflow writer.
   After correction, resume succeeds and one retained prompt reaches the resolved recipient after readiness.
2. The state matrix preserves working and terminal rows, including stale-read and lost-write orderings.
   Engine, legacy, WIP, profile-switch, and admitted FIFO/Send Now cases retain their intended behavior.
3. Cancellation and genuine launch failures retain their existing state, error, identity, and queue outcome.
   The correction creates no false waiting/Review event, duplicate transition, or competing launch.

## TDD sequence

1. Add `TestOnTurnStartDuringResumePreservesStarting` in the new workflow test file.
2. Run that test before production changes.
3. Add `TestResumeCredentialSnapshotSurvivesTurnStart` with a barrier after the early startup claim.
4. Execute the real workflow transition while credential-snapshot persistence waits.
5. Release the barrier and assert resume persistence and eventual dispatch.
6. Add the remaining state, identity, WIP, cancellation, and failure cases from the plan.
7. Implement the narrow workflow correction.
8. Run the required commands and record results.

Reuse the real repository and workflow service in the regression.
The credential-boundary harness must invoke the actual resume persistence path.
A helper-only assertion or a mock that always reports resume success is insufficient.
The test must record startup-attempt metadata, transition count, launch count,
user-message count, provider identity, and actual dispatched input.

## Verification

Run each command from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestOnTurnStartDuringResumePreservesStarting$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'TestOnTurnStartDuringResume|TestResumeCredentialSnapshotSurvivesTurnStart|TestTurnStartPreparation|TestResumeTurnStartGenuineFailure|TestSendNowWorkflowTransitionPreservesRunningState|TestOnTurnStartBeforeAdmissionKeepsSessionPromptable|TestResumePromptQueue|TestResumeTaskSession.*Cancel|TestResumeAttemptCancellation' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run 'TestResumeSession_PersistsStartingBefore|TestResumeSession_PersistsCredentialSnapshot|TestResumeSession_DoesNotLaunchWhenCredentialSnapshot|TestResumeSession_RollsBack|TestResumeSession_PreservesStarting|TestRollbackResumeStateAfterFailure|TestResumeSession_CancellationBeforeResumeLockWins' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run 'Test.*(Resume|RuntimeUnavailable|TurnStart)' -count=1)
git diff --check
```

The first command records Red before the correction and Green afterward.
The race command covers the new interleavings without timing sleeps.
If the credential harness requires an executor-local test, name it
`TestResumeCredentialSnapshotSurvivesTurnStart` and add that exact executor
command to Results. Do not omit its execution because the package changed.
Confirm each selector discovers its intended cases.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_turn_start_state.go` (new)
- `apps/backend/internal/orchestrator/workflow_turn_start_resume_test.go` (new)
- `apps/backend/internal/orchestrator/workflow_turn_start_profile_error_test.go` (new)
- `apps/backend/internal/orchestrator/workflow_turn_start_state_test.go` (new)
- `apps/backend/internal/orchestrator/workflow_turn_start_transition_error_test.go` (new during PR fixup)

Read `event_handlers_streaming.go`, `task_operations.go`, and
`executor/executor_resume.go` for state and admission boundaries.
Keep their existing contracts unless regression evidence requires a scoped correction.

## Dependencies

None.

## Risks

- A stale source row cannot authorize state changes for a new recipient.
- The conditional write must preserve a newly claimed startup or terminal successor.
- Lock order must remain compatible with cancellation and resume admission.
- Keeping `STARTING` exposes ordinary startup queue admission instead of inventing readiness.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/resume-prompt-queue.md).
- [Design](../../specs/tasks/system-design/resume-prompt-queue.md), Workflow transitions during resume.
- [Plan](plan.md), Confirmed defect and reproduction, Tests.
- `apps/backend/AGENTS.md` and `/tdd` backend test guidance.
- `queue_send_now_workflow_state_test.go` and `resume_prompt_queue_test.go`.
- `executor/executor_resume_test.go`: credential snapshot and terminal-race tests.
- `task_operations_resume_cancellation_test.go` and `message_handlers_resume_readiness_test.go`.

## Results

Implemented the workflow-specific recipient state preparation and conditional
state publication. The credential-boundary regression confirms that the resume
claim survives a turn-start transition, and a genuine launch failure still
retains its existing error and queued prompt.

Red was reproduced in an isolated pre-fix checkout at `c8273683132`:

```text
go test -trimpath -buildvcs=false -tags fts5 ./internal/orchestrator -run '^TestOnTurnStartDuringResumePreservesStarting$' -count=1
--- FAIL: TestOnTurnStartDuringResumePreservesStarting
    workflow_turn_start_red_test.go:48: session state = WAITING_FOR_INPUT, want STARTING
FAIL
```

The same selector passed after the correction. The remaining required backend
selectors also passed:

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestOnTurnStartDuringResumePreservesStarting$' -count=1)`
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'TestOnTurnStartDuringResume|TestResumeCredentialSnapshotSurvivesTurnStart|TestTurnStartPreparation|TestResumeTurnStartGenuineFailure|TestSendNowWorkflowTransitionPreservesRunningState|TestOnTurnStartBeforeAdmissionKeepsSessionPromptable|TestResumePromptQueue|TestResumeTaskSession.*Cancel|TestResumeAttemptCancellation' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run 'TestResumeSession_PersistsStartingBefore|TestResumeSession_PersistsCredentialSnapshot|TestResumeSession_DoesNotLaunchWhenCredentialSnapshot|TestResumeSession_RollsBack|TestResumeSession_PreservesStarting|TestRollbackResumeStateAfterFailure|TestResumeSession_CancellationBeforeResumeLockWins' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run 'Test.*(Resume|RuntimeUnavailable|TurnStart)' -count=1)`

Review follow-up on 2026-10-04 covered a route-persistence failure after the
turn-start transition commit, while resume was blocked at credential issuance.
The new `TestTurnStartProfilePreparationFailurePreservesResume` runs through
both legacy and engine workflow evaluation. Before the correction, the legacy
case returned no strict evaluation error and the engine case returned only a
generic preparation error. After the correction, both cases preserve
`STARTING`, the startup attempt and error fields, the committed single
transition, and the `IN_PROGRESS` task projection; neither publishes a false
waiting or Review event, strict evaluation returns the injected route error,
and resume completes after the credential barrier is released.

The Red command was:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestTurnStartProfilePreparationFailurePreservesResume$' -count=1)
```

It failed in both subtests as expected: legacy returned `<nil>`, and engine
returned `workflow on_turn_start session preparation failed` instead of the
injected route persistence error. The focused command passed after the fix.
The updated race selector passed with this regression included, and the
refactored regression passed again under `-race`:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^TestTurnStartProfilePreparationFailurePreservesResume$' -count=1 -v)
```

The executor and task-handler selectors above also passed.
`make -C apps/backend build` passed.

PR fixup on 2026-10-04 added a strict legacy-evaluation regression for a target
step lookup failure while the recipient remains `STARTING`. The first run was
red because strict evaluation returned no error even though the transition did
not commit:

```text
go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestLegacyTurnStartTransitionFailureSurfacesForStartingSession$' -count=1 -v
strict on_turn_start error = <nil>, want injected transition error
```

The correction records the original transition error before workflow-specific
state preparation. The focused regression then passed. The same follow-up fixed
the lost-write fake to invoke the embedded repository CAS; its targeted test
passed with the profile-preparation regression:

```text
go test -trimpath -tags fts5 ./internal/orchestrator -run '^(TestPrepareWorkflowTurnStartSessionStatePreservesLostWrite|TestLegacyTurnStartTransitionFailureSurfacesForStartingSession|TestTurnStartProfilePreparationFailurePreservesResume)$' -count=1 -v
```

The expanded race selector also passed, including the strict legacy transition
error and credential-boundary cases:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'TestOnTurnStartDuringResume|TestResumeCredentialSnapshotSurvivesTurnStart|TestTurnStartPreparation|TestResumeTurnStartGenuineFailure|TestLegacyTurnStartTransitionFailure|TestSendNowWorkflowTransitionPreservesRunningState|TestOnTurnStartBeforeAdmissionKeepsSessionPromptable|TestResumePromptQueue|TestResumeTaskSession.*Cancel|TestResumeAttemptCancellation|TestTurnStartProfilePreparationFailurePreservesResume|TestPrepareWorkflowTurnStartSessionState' -count=1)
```
