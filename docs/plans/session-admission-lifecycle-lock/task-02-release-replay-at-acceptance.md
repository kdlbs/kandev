---
id: "session-admission-lifecycle-lock-02"
title: "Release fresh-start replay at acceptance"
status: done
wave: 2
depends_on: ["session-admission-lifecycle-lock-01"]
plan: "plan.md"
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
acceptance_criteria:
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.8
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Work Order 02: Release fresh-start replay at acceptance

## Outcome

Fresh-start recovery (Retry launch, `RecoverSession(..., "fresh_start")`)
returns once the provider accepts the replayed original submission. The resume
continuation, and the session lifecycle and route-operation locks it holds, no
longer stay held for the whole agent turn.

## Scope

- `replayInitialPromptSubmission` (`initial_submission_recovery.go`): call
  `promptTask` with `dispatchOnly=true`. Keep every existing option (`onAccepted`,
  `beforeProviderAdmission`, `disableDispatchRetry`, `retryPrompt`, the
  initial-prompt hold) unchanged.
- Confirm that no caller of the replay result depends on turn completion. The
  replay discards `PromptResult`; `RecoverSessionWithOptions` uses only the
  launch response.

## Exclusions

`ResumeTaskSessionAndPrompt` and the context-continuation prompt are unchanged.
No change to submission receipt states or transcript backfill.

## Acceptance

1. With a provider prompt that has accepted but whose turn has not finished,
   `RecoverSessionWithOptions(..., "fresh_start", ...)` returns without error.
2. During that in-flight turn, with the session in `WAITING_FOR_INPUT`,
   `focusTaskSession` returns and `messageQueue.WithSessionAdmission` with a 1 s
   budget succeeds.
3. All existing `TestFreshStart*` and `TestInitialTaskSubmission*` tests pass.

## Regression test (fails before the fix)

New file `apps/backend/internal/orchestrator/fresh_start_replay_lifecycle_release_test.go`,
`TestFreshStartReplayReleasesLifecycleAtAcceptance`. It follows the setup in
`assertFreshStartSubmissionReplay`. The mock `promptAgentFunc` treats
`dispatchOnly=false` as "block until the test releases the turn" and returns at
once for `dispatchOnly=true`. The test asserts that recovery completes while the
turn is still unreleased, then that focus plus a 1 s admission probe succeed.
Before the fix, recovery blocks and the probe times out (the reproduction
recorded in the plan).

## Files

`apps/backend/internal/orchestrator/initial_submission_recovery.go` and the new
test file.

## Verification

```text
cd apps/backend
go test -trimpath -race -count=1 ./internal/orchestrator/ -run 'TestFreshStartReplayReleasesLifecycleAtAcceptance|TestFreshStart|TestInitialTaskSubmission'
bash ../../scripts/lint-go-changed internal/orchestrator/initial_submission_recovery.go internal/orchestrator/fresh_start_replay_lifecycle_release_test.go
```

## Results

Implemented. `replayInitialPromptSubmission` dispatches with
`dispatchOnly=true`. The regression test failed first (recovery did not return
within 3 s while the turn was open) and now passes. `TestFreshStart*`,
`TestInitialTaskSubmission*`, `TestMissingCheckout*`, and
`TestWorkflowStartPrompt*` pass under `-race`. Work order 04 gives
dispatch-only prompts, including this replay, the runtime stall watchdog.
