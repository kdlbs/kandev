---
id: "01-capacity-policy-and-feedback"
title: "Extended capacity policy and feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-003.1
  - AC-PLATFORM-TURN-CONTINUITY-003.2
  - AC-PLATFORM-TURN-CONTINUITY-003.3
  - AC-PLATFORM-TURN-CONTINUITY-003.4
  - AC-PLATFORM-TURN-CONTINUITY-003.5
  - AC-PLATFORM-TURN-CONTINUITY-003.6
  - AC-PLATFORM-TURN-CONTINUITY-003.7
  - AC-PLATFORM-TURN-CONTINUITY-003.8
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 01: Extended capacity policy and feedback

## Summary

Deliver the extended capacity episode through the existing scheduler and Chat
notice. Use TDD to prove deadline, attempt accounting, cancellation, restart
retirement, and safe same-runtime admission before changing production behavior.

## In scope

- Capacity-only policy selection, immutable start/deadline, eleven-slot ladder,
  shared replay/continuation budget, terminal wake, and dispatch-boundary checks.
- Package-local deterministic clock/timer seam; barrier-controlled admission races.
- Single persisted notice with window deadline and selected maximum; distinct
  expiry, refusal, cancellation, and exhaustion feedback with actual started count.
- Startup retirement of extended notices without reconstructing retry authority.
- Localized desktop/phone inline feedback, semantic status announcements, and
  compatible rendering of old notices without new fields.
- The four exact Go tests and frontend test additions named in the plan.
  Update affected existing capacity budget tests to select the episode policy
  rather than assuming the global short limit; retain non-capacity assertions.

## Out of scope

- New provider support, altered effect evidence, model switching, persistence,
  settings, release flags, independent schedulers, and production time-control APIs.
- E2E mock changes and public prose, assigned to Task 02.

## Acceptance

1. All capacity deadline/eleven-attempt tests pass, including an accepted turn
   spanning the deadline, no reset after output/tools, and no twelfth dispatch.
2. Safety/owner barriers and restart retirement prevent stale or ambiguous work;
   non-capacity recovery retains the original five-attempt policy.
3. Shared desktop/phone rendering shows one accurate localized notice and stop
   reason, preserves old metadata rendering, and keeps Cancel/composer accessible.

## ASCII UI preview

UI-01, waiting in task Chat, excerpt from the [full preview](plan.md#ascii-ui-preview):

```text
Desktop:
Model at capacity. Retrying in 2:00.             [Cancel]
Codex / selected model
Attempt 6 of 11. Recovery window ends at 14:30.

Phone:
Model at capacity.
Retrying in 2:00.
Codex / selected model
Attempt 6 of 11.
Recovery window ends at 14:30.
[ Cancel ]

UI-02, terminal, both viewports:
Model at capacity.
Automatic recovery window ended.
11 attempts started. You can send another message.
[ normal composer ]
```

AC `.5` and `.7`: structure is required; sample copy/counts/time are illustrative.
Use the existing transcript scroll owner and composer; phone Cancel has at least
44px actual height, desktop retains compact controls. Wrap long provider names.
No overlay or focus transfer. Task 02 owns rendered Playwright proof.

## Verification

Run from repository root. Bootstrap once if this worktree lacks dependencies.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/orchestrator -run 'Test(Capacity|Transient|AgentTurnFailed|Retained|Continuation)' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run '^TestCapacityDelayedRetry' -count=1)
(cd apps/web && pnpm test components/task/chat/messages/interruption-recovery-feedback.test.ts components/task/chat/messages/transient-retry.test.ts components/task/chat/messages/action-message.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/messages/transient-retry-notice.tsx components/task/chat/messages/interruption-recovery-feedback.ts components/task/chat/messages/action-message-details.tsx components/task/chat/messages/action-message.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Run changed-Go-file lint using `bash scripts/lint-go-changed` with every changed
Go path, per backend guidance. Include any newly extracted frontend helper in
the targeted lint/test commands. Record failing Red tests and passing Green
results. Run no broad local review or audit.

## Files likely touched

- `apps/backend/internal/orchestrator/transient_retry_clock.go` (constructor timing seam)
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/provider_capacity_retry_policy.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_messages.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_retained_runtime.go`
- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go`
- `apps/backend/internal/orchestrator/provider_interruption_preparation.go`
- `apps/backend/internal/orchestrator/provider_interruption_failure.go`
- `apps/backend/internal/orchestrator/provider_interruption_restart.go`
- `apps/backend/internal/orchestrator/provider_interruption_capacity_delayed_test.go` (new)
- Existing capacity, transient, retained-runtime, budget, and restart tests beside these files.
- `apps/web/components/task/chat/messages/transient-retry-notice.tsx`
- `apps/web/components/task/chat/messages/action-message-details.tsx`
- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/interruption-recovery-feedback.ts`
- Associated tests listed above and `apps/web/src/locales/*/chat.json`.

## Dependencies

None. Read root/backend/web guidance plus `/tdd`, `/mobile-parity`, and existing
capacity evidence tests before implementation. No protocol support expansion.

## Risks

Multiple helpers assume the global maximum; updating scheduling alone produces
premature exhaustion or wrong feedback. Never weaken refusal to accommodate the
new schedule. Clock injection must cover waits and final admission coherently.

## Parallelism

`sequential`

## Inputs

- Owning requirement, REQ-PLATFORM-TURN-CONTINUITY-003, especially AC `.3` to `.8`.
- Owning design, Capacity episode timing; Capacity recovery admission; Desktop and phone.
- Existing `provider_interruption_capacity_test.go` and `transient-retry-notice.tsx`.

## Results

Implemented and locally verified. Regression Red covered the five-attempt maximum,
thirty-minute admission boundary, restart counts above five, replay accounting,
clean continuation failures, and initiating-user continuity. Corrections preserve
the generic five-attempt policy and all runtime/authorization/effect fences.

The affected orchestrator/backend composition race selection passed, including
`TestCapacityDelayedRetryPreservesInitiator`, the eleven-slot schedule, deadline,
terminal wake, final admission, restart retirement, ownership, active-turn expiry,
and replay-to-continuation cases. Changed-package Go lint reports zero issues.
All 65 focused frontend tests, localization checks, frontend typecheck, and focused
source/E2E lint passed. Desktop and phone rendered proof passed (8 tests each).
See the plan's verification results for exact selection and artifact freshness.

PR remediation adds `TestInterruptionContinuationFifthRestoreDispatches`,
`TestCapacityDelayedRetryChangedErrorSettlesEpisode`,
`TestCapacityDelayedRetryRejectedReplayDoesNotCount`, and
`TestCapacityDelayedRetryRestartNoticeUpdateFailure`. Red reproduced the final
restoration block, rejected-replay overcount, and skipped restart cleanup; the
corrected cases and full affected race selection pass on the reconciled tree.
The replay acceptance fixtures now use the real turn service. Immutable policy
reads remain intentionally unlocked; capacity episodes still never restore a
lost runtime. Fresh remote verification remains the PR workflow's final gate.
