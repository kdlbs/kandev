---
id: "04-replay-policy-and-admission"
title: "Close replay policy and admission gaps"
status: complete
wave: 4
depends_on:
  - 03-browser-evidence
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-002.1
  - AC-TASKS-PROMPT-ATTACHMENTS-002.3
  - AC-TASKS-PROMPT-ATTACHMENTS-002.5
  - AC-TASKS-PROMPT-ATTACHMENTS-002.6
  - AC-TASKS-PROMPT-ATTACHMENTS-002.7
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 04: Close replay policy and admission gaps

## Summary

Make fresh-start replay carry the current first-conversation Kandev policy,
remain the first admitted turn, settle safely when its runtime disappears, and
lose replay authority after any alternate conversation work is accepted.

## In scope

- Compose the replay prompt with the first-launch instructions generated from
  current trusted task/session state. Keep the receipt and transcript raw, and
  exclude stale provider context and hidden prompt expansion.
- Disable the generic missing-execution retry fallback inside the owned replay
  continuation. Settle its correlated failure without starting an untracked
  successor while the lifecycle lock is held.
- Enforce the initial-prompt hold at every shared guarded queue reservation
  boundary, including manual and enqueue-side drain paths.
- Release/reschedule the hold on replay acceptance or terminal failure so normal
  queue draining can continue.
- Persist accepted-work provenance from ordinary resume and interactive prompt
  paths. A later accepted prompt must retire pending replay authority; a visible
  initial user row is not acceptance evidence. Ambiguous provenance fails closed
  after restart.

## Out of scope

The original runtime startup timeout, changes to ordinary prompt content,
read-only restoration behavior, and replay of any prior conversation history.

## Acceptance

1. Captured provider replay contains current task/session identity and applicable
   completion-signal, user-question/autopilot, and title/tool policy, with one
   raw transcript row and no stale hidden context.
2. A runtime that disappears after readiness produces bounded terminal failure
   and no generic fallback or untracked successor launch.
3. While replay is paused before admission, manual and enqueue-side queue drains
   cannot reserve a later message. The initial replay wins and the queued entry
   remains intact; after acceptance or terminal failure the hold no longer blocks
   normal draining.
4. Ordinary resume and interactive accepted prompts retire the pending replay
   authority. A later bootstrap failure cannot replay the original input,
   including after repository/backend restart. The initial visible user row alone
   does not retire or authorize the receipt.

## Verification

From the repository root, run:

```bash
(cd apps/backend && go test -trimpath -timeout=90s ./internal/orchestrator -run '^(TestFreshStartReplayUsesCurrentFirstLaunchInstructions|TestFreshStartReplayRuntimeDisappearsAfterReady|TestFreshStartReplayHoldBlocksManualAndEnqueueQueueDrains|TestFreshStartPendingReceiptBlockedByLaterPromptAcrossRestart)$' -count=1)
(cd apps/backend && go test -trimpath -race -timeout=120s ./internal/orchestrator -run '^(TestFreshStartReplayRuntimeDisappearsAfterReady|TestFreshStartReplayHoldBlocksManualAndEnqueueQueueDrains|TestFreshStartPendingReceiptBlockedByLaterPromptAcrossRestart)$' -count=1)
```

Write each regression before its production correction and demonstrate the
expected behavioral failure. Use captured dispatch input, durable receipt state,
queue state, and launch count as assertions. Use barriers instead of sleeps for
the readiness-to-admission ordering test. Keep the original startup timeout out
of all fixtures and conclusions.

## Files likely touched

- `apps/backend/internal/orchestrator/initial_submission_recovery.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/fresh_start_submission_test.go`
- `apps/backend/internal/orchestrator/task_operations_resume_cancellation_test.go`
- `apps/backend/internal/task/models/initial_prompt_submission.go` if a terminal
  receipt state is needed
- `apps/backend/internal/task/repository/sqlite/session.go` only if accepted-work
  provenance needs an additional atomic metadata operation

## Dependencies

Task 03 and the replay-policy amendments in
`docs/specs/tasks/system-design/prompt-attachments.md`.

## Risks

The replay hold must be visible at the shared reservation boundary, not just at
one drain caller. Receipt changes must be conditional and must not infer provider
acceptance from transcript rows. Prompt composition must use the current policy
without saving generated hidden context into the original submission.

## Parallelism

Owns orchestrator and submission-provenance files. The independent SQLite JSON
member identity correction is task 05.

## Results

Complete on 2026-10-06. The replay prompt now receives current first-launch
instructions while the original receipt and single user row stay raw. Dispatch
retry is disabled inside the owned continuation; the session hold guards shared
queue reservations and is released or rescheduled on acceptance or failure.
Ordinary and interactive accepted prompts retire pending replay authority with
durable provenance, including after repository restart.

Validation passed:

- Focused orchestrator replay, runtime-disappearance, queue-hold, restart
  provenance, receipt, and legacy-preview tests passed.
- Runtime-disappearance, queue-hold, and restart-provenance tests passed with
  `-race`.
- Managed Playwright fresh-start recovery passed for desktop (2 tests) and phone
  (1 test) against the final implementation.
- `go build -trimpath ./...`, changed-code `golangci-lint`, and the complete
  backend test run passed. The exact full-suite command and final results are in
  `plan.md`.
- The original initial runtime timeout remains out of scope.

PR review follow-up on 2026-10-07 keeps later-prompt retirement tied to provider
acceptance. A rejected prompt restores the pending receipt; an interrupted
dispatch remains uncertain and fails closed. The original transcript row uses a
stable idempotency key and the accepted replay turn ID, and an explicit durable
identity check ignores unrelated session messages. Focused rejection,
interrupted-dispatch, legacy-CAS-conflict, and transcript-backfill regressions
passed with the full backend suite, targeted race checks, build, and
changed-scope lint. The attachment read remains unchanged because its total size
is bounded by the existing recovery limit.
