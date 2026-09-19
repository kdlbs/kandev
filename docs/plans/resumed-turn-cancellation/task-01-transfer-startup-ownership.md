---
id: "01-transfer-startup-ownership"
title: "Transfer startup ownership at prompt acceptance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.9
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 01: Transfer Startup Ownership at Prompt Acceptance

## Summary

End startup cancellation authority when the provider accepts the resumed prompt.
Keep the execution identity valid so pause and follow-up use the same healthy process.

## In scope

- Implement the accepted phase and guarded authority transfer in the existing attempt registry.
- Integrate lazy and compound resume, prompt acceptance, final result handling, and exact startup cleanup.
- Audit handler retries and model switches for the same accepted boundary.
- Propagate model-switch startup acceptance and pre-acceptance failure callbacks through lifecycle.
- Add the named service regressions from the plan and desktop/phone process-reuse scenarios.
- Preserve pre-acceptance cancellation, stale-callback rejection, and unrelated queued work.

## Out of scope

New public contracts, UI components, provider policies, schema, workflow parking changes,
commits or PR delivery, and mutation of the reported live instance.

## Acceptance

1. Pausing an accepted resumed turn preserves the healthy execution and provider identity. Follow-up dispatches exactly once on that execution.
2. Cancellation before acceptance still fences startup. Accepted events remain valid while stale callbacks cannot affect a replacement execution.
3. Desktop and phone regressions pass through real cancellation and follow-up controls. Accepted publication failure causes no replay or startup cleanup.

## TDD and reproduction

First add `TestPromptTask_ResumedTurnCancelPreservesExecution` in the new test file.
Hold the resumed provider turn after its acceptance callback, then call `CancelAgent`.
Release normal provider cancellation and assert zero forced stops and unchanged execution identity.
Send another prompt and assert one delivery on that execution.
Run it before production edits. The expected failure is startup teardown of the accepted execution.
Do not accept unrelated setup failures as the red result.

Add the remaining named barrier regressions from the plan before each corresponding correction.
Use channels and bounded waits, not arbitrary sleeps.
Read the backend TDD reference before creating fixtures or subprocesses.

Write the desktop and phone scenarios before the correction and record the failing process-reuse assertion.
Use existing mock scenarios and isolated fixtures. Disable passive resume for the reproduction.
The first new message must trigger lazy resume and remain active after provider acceptance.
Use existing authorized test API reads for execution/provider identity and durable message IDs.
If the current helper lacks those fields, extend its typed read helper without adding a production endpoint.
Do not rely on the deduplicated visible resume label as the only evidence.

## Verification

Run the complete block from the repository root after implementation.
Install workspace dependencies once before the first frontend command in a fresh worktree.
The managed E2E runner rebuilds the binaries and serves the production web build.
Run the two browser suites sequentially without worker overrides.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test(PromptTask_(ResumedTurnCancel|QueuedAcceptedTurnIdentityReadFailure)|ResumeTaskSessionAndPrompt_AcceptedTurnCancel|ResumeAttempt_)' -count=1 -timeout=10m)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/task/handlers -count=1 -timeout=20m)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/resume_attempt.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_resumed_turn_cancellation_test.go` (new)
- `apps/backend/internal/orchestrator/task_operations_resume_cancellation_test.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go` (only if acceptance propagation needs correction)
- `apps/backend/internal/agent/runtime/lifecycle/{types,manager_interaction,session}.go` (model-switch initial-prompt callbacks)
- `apps/backend/internal/task/handlers/message_handlers_resume_readiness_test.go`
- `apps/web/e2e/tests/session/session-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- `apps/web/e2e/helpers/session-resume-prompt-queue.ts`
- `apps/web/e2e/helpers/api-client.ts` (only if an existing identity read needs typing)
- This plan, work order, and the linked requirement/design amendment for result updates.

## Dependencies

None. Existing resume cancellation and provider acceptance callbacks are in this checkout.

## Risks

Keep transfer inside the cancellation guard, but never hold that guard during turn settlement.
Retain event provenance without keeping startup teardown authority alive.
Preserve accepted outcomes when publication fails. Do not replay ambiguous provider work.
Resume without a prompt, dispatch-only calls, and model-switch branches must retain their lifecycle rules.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/session-recovery-failures.md#recovery-attempt-isolation).
- [Accepted-turn design](../../specs/agents/system-design/session-recovery-failures.md#accepted-turn-ownership-requirement-007).
- [Evidence and test matrix](plan.md).
- [Cancellation ownership ADR](../../decisions/2026-08-03-backend-owned-cancellation-progress.md).
- Existing `TestPromptTask_ResumeAttemptKeepsCancellationOutOfAcceptanceCallback` and same-execution replacement tests.
- Existing desktop and phone delayed-startup cancellation fixtures.

## Results

The first RED run of `TestPromptTask_ResumedTurnCancelPreservesExecution`
failed with `resume attempt cancelled: resume-1` after provider acceptance.
The fix adds an accepted phase to the resume-attempt registry, transfers startup
ownership at the ordinary dispatch callback and model-switch replacement boundary,
and retains the attempt identity for valid late provider events. Accepted cleanup
and invalidation are fenced while pre-acceptance cancellation remains cancellable.
The review correction makes the transfer use the captured execution identity and
occur before `afterDispatch`. It does not depend on a second session identity read,
so a queued dispatch with a nonempty incarnation cannot leave accepted work startup-
cancellable when that read fails. The new queued regression injects that failure,
observes the hook ordering, pauses the accepted turn, and verifies follow-up reuse
of the same execution.

The final verification block passed:

- `pnpm install --frozen-lockfile` from `apps`.
- Focused and full affected-package Go race suites.
- Chromium `session-recovery.spec.ts`: 7 passed.
- Mobile Chromium `mobile-session-resume-recovery.spec.ts`: 4 passed.
- Web typecheck.
- Specification catalog validation and full specification lint.
- `git diff --check`.

The accepted direct regression returns the provider's nil-error cancelled result.
The compound regression returns the wrapped `lifecycle.ErrCancelEscalated` form.
Publication failure and model-switch replacement regressions confirm that accepted
work does not trigger startup teardown or replay. The desktop and phone accepted-turn
scenarios capture runtime identity after the first accepted response, wait for a new
persisted response ID before the second pause, and compare that identity after every
pause and follow-up. The package is uncommitted and no live runtime state was changed.

The PR fixup review found that the model-switch fallback starts its initial prompt
from a lifecycle goroutine after `StartAgentProcess` returns. The implementation now
registers one-shot dispatch and pre-acceptance failure callbacks before startup,
keeps the resume attempt active until one of them settles, and transfers ownership
using the replacement execution ID captured by the launch response. The new
`TestResumeAttempt_ModelSwitchFallbackCancellationBeforeInitialPromptAcceptance`
barrier proves cancellation still force-cleans the replacement before provider
acceptance and that a delayed callback cannot revive the cancelled attempt.
Lifecycle coverage also verifies that the callback reaches the asynchronous initial
prompt acceptance point.
