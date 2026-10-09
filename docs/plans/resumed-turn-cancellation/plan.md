---
created: 2026-09-18
status: complete
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Implementation Plan: Preserve Resumed Agents After Pause

## Overview

Preserve the healthy agent process when the user pauses an accepted resumed turn.
One sequential work order implements the ownership correction and its service and browser regressions.
Implementation is complete.

## Evidence and root cause

The reported session is `7c6a9ba2-e2be-4ff8-bf56-d0e339fee3df` in task
`742b1dca-39bc-4035-856b-39240f0b9340`. The running build was `81b59416ab`.
The provider session was `01a0b19b-c373-7853-9c0e-7c68e9c2b196`.
Backend diagnostics establish this September 18 timeline in Europe/Lisbon:

| Time | Event |
| --- | --- |
| 00:32:01 | Resumed execution `5e11de73-89a6-4fb6-8f0c-a53a892859b0` accepts work. |
| 00:37:26 | The user requests turn cancellation. |
| 00:37:30.341 | The provider completes cancellation normally. |
| 00:37:30.450 | Kandev force-stops the execution as `cancelled resume startup`. |
| 00:37:33 | Kandev removes the instance and execution tracking. |
| 00:37:35 | The next user message finds no agent and requests another resume. |
| 00:37:46 | Replacement execution `94b839f4-0c9e-4a91-ba87-74c557295d16` becomes ready. |

The source confirms that `promptTask` defers attempt completion until its blocking
prompt call returns. `runExplicitCancellationOwned` invalidates that attempt.
`finishPromptExecutorDispatch` then invokes startup cleanup after normal turn cancellation.
The same defect can recur during the first turn of each replacement execution.

Evidence came from diagnostic bundle `b581ffed368b708481e536590e5d5847`.
The archive byte limit truncated older history, but the incident window is present.
This package uses the existing trace and source inspection plus deterministic service and browser regressions.

## Requirement conformance

The agents system owns provider continuity and startup attempt outcomes.
The [existing requirement](../../specs/agents/requirements/session-recovery-failures.md)
adds criteria 007.7 through 007.9 for accepted-turn cancellation and process reuse.
Criteria 007.2 through 007.4 retain startup fencing, retry, and disconnect protection.
The [design amendment](../../specs/agents/system-design/session-recovery-failures.md#accepted-turn-ownership-requirement-007)
separates startup authority from execution identity.

The completed [resume cancellation package](../resume-cancellation/plan.md)
remains the baseline. Its recorded results remain historical evidence.
Its acceptance-barrier test permits `ErrResumeAttemptCancelled` after acceptance
and never asserts process survival. This package closes that coverage gap.
Other contribution recovery, error-history, and resume-queue packages retain their scope and results.

## Scope

### In scope

- Transfer startup cancellation authority at provider acceptance.
- Preserve accepted execution callbacks, runtime identity, and normal cancellation outcomes.
- Keep model-switch fallback startup cancellable until lifecycle reports initial-prompt acceptance or failure.
- Cover lazy resume, compound resume-and-prompt, handler retry, and relevant model-switch paths.
- Preserve cancellation before readiness, queued work, and exact replacement cleanup safety.
- Prove pause and follow-up behavior on desktop and phone.

### Out of scope

- Changes to workflow parking, explicit runtime stops, archive, or restart recovery policy.
- Removal of cancellation timeout escalation when a provider does not settle.
- New UI layout, labels, APIs, schema, flags, provider fallback, or automatic replay.
- Mutation of the reported live session or implementation during this design turn.

## Technical approach

Use the accepted phase described in the owning design. Integrate it into
`resume_attempt.go`, the prompt acceptance callback, and cancellation cleanup.
The existing cancellation guard serializes transfer and cancellation.
Registry completion and tombstones retain event provenance after the transfer.
Publication errors after provider acceptance remain accepted outcomes.
Audit the compound continuation and model-switch return paths for stale startup checks.
Do not fix only the final cleanup call while leaving valid events classified as stale.

## Tests

Add `task_operations_resumed_turn_cancellation_test.go` with these named regressions:

| Test | Criteria and evidence |
| --- | --- |
| `TestPromptTask_ResumedTurnCancelPreservesExecution` | 007.7-.9: first resumed turn accepts, pauses, preserves execution/token, and accepts the next message once. |
| `TestResumeTaskSessionAndPrompt_AcceptedTurnCancelPreservesExecution` | 007.7-.8: outer compound continuation cannot reclaim an accepted runtime. |
| `TestResumeAttempt_AcceptanceCancellationOrdering` | 007.2-.4, .8: barriers prove cancellation before acceptance and after acceptance, including duplicate acceptance callbacks. |
| `TestResumeAttempt_AcceptedExecutionEventsRemainValid` | 007.2, .9: token, stream, and completion events remain valid after pause; stale/replaced events remain rejected. |
| `TestResumeAttempt_AcceptedPublicationFailureDoesNotCleanup` | 007.8: accepted publication failure causes no startup teardown, rollback, or replay. |
| `TestResumeAttempt_ModelSwitchFallbackTransfersAcceptance` | 007.8: a replacement execution launched by model-switch fallback inherits accepted-turn ownership. |
| `TestResumeAttempt_ModelSwitchFallbackCancellationBeforeInitialPromptAcceptance` | 007.7-.8: cancellation after process startup but before asynchronous initial-prompt acceptance force-cleans the replacement and fences a delayed callback. |
| `TestPromptTask_QueuedAcceptedTurnIdentityReadFailurePreservesExecution` | 007.7-.9: a queued dispatch with a nonempty incarnation transfers ownership before an injected identity-read failure and preserves the same execution for pause and follow-up. |

Use real service paths with controlled provider barriers. Assert zero forced-stop
calls after successful turn cancellation. Include both nil-error cancelled results
and the existing provider cancellation error form. Preserve unrelated queued work.
The compound regression uses the wrapped `ErrCancelEscalated` provider cancellation form.
Add handler retry cases in their existing test files if those paths differ.
A mere successful return or unchanged boot-row count does not prove process survival.
The queued-dispatch regression also observes the ordinary `afterDispatch` hook and
proves that acceptance has already revoked startup cleanup authority before that
hook or any fallible identity publication read can run.

## E2E tests

Extend `session-recovery.spec.ts` in `chromium` and
`mobile-session-resume-recovery.spec.ts` in `mobile-chrome`.
Use the existing isolated backend and mock provider.
Make the first message perform lazy resume while passive resume is disabled.
Wait for actual accepted work before clicking or tapping Pause.
After cancellation settles, submit a follow-up through the composer.
Assert a response, unchanged execution/provider identity, and no added durable boot event.
Repeat pause and follow-up on that same execution to cover later turns.
Capture the initial runtime identity after the first message is accepted and before
the first pause. For the second pause, snapshot the existing persisted response
message IDs and wait for a newly persisted response ID before pausing, so the witness
cannot match the first turn's output. Compare the runtime identity after cancellation
and after each follow-up.
The phone case retains reachable touch controls and no horizontal page overflow.
Keep existing delayed-startup cancel/retry scenarios as the negative-boundary coverage.

No rendered structure changes are planned, so an ASCII UI preview is unnecessary.
The existing task chat and mobile session layout remain the interaction exemplars.

## Work orders

- [x] [Task 01: Transfer startup ownership at prompt acceptance](task-01-transfer-startup-ownership.md)

## Verification results

Implementation is complete. The first RED run of
`TestPromptTask_ResumedTurnCancelPreservesExecution` failed with
`resume attempt cancelled: resume-1` after provider acceptance, reproducing
startup cleanup of the accepted execution. The final verification block passed:

- `pnpm install --frozen-lockfile` from `apps`: passed.
- Focused orchestrator race suite for accepted cancellation, ordering, events,
  publication failure, and model-switch acceptance: passed.
- Queued accepted-turn regression with injected identity-read failure and
  same-execution follow-up: passed.
- Full affected-package race suite for orchestrator, executor, lifecycle, and
  task handlers: passed.
- Chromium `session-recovery.spec.ts`: 7 tests passed.
- Mobile Chromium `mobile-session-resume-recovery.spec.ts`: 4 tests passed.
- Web typecheck: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Model-switch lifecycle callback propagation and pre-acceptance cancellation barrier: passed.

The browser helper counts persisted `agent_boot` metadata because the boot row's
content is empty and the rendered resume label is deduplicated. The package is
uncommitted, as required by the work order.
Design validation on 2026-09-18:

- `python3 scripts/list-docs.py validate`: passed (288 decisions, 1004 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Package links and acceptance references: passed.
- `git diff --check`: passed.
- `git status --short -- docs/plans/resumed-turn-cancellation`: both new package files are present and untracked.

Implementation verification is complete. The package remains uncommitted for review.

## Documentation impact

Internal requirements, design, and delivery records change. Public documentation
has no new command, configuration, control, or recovery procedure to describe.
The existing cancellation ADR is sufficient for this local boundary correction.
No new ADR or system index inventory is necessary.

## Risks

- Early transfer can let cancelled startup dispatch. Late transfer can kill accepted work.
- Deleting identity evidence can reject valid events or admit stale callbacks.
- Callback publication failure can incorrectly restore startup cleanup authority.
- A guard held during provider settlement can deadlock cancellation.
- Passive browser recovery can hide the defect by launching a replacement before assertions.
