---
id: "03-recovery-feedback"
title: "Render truthful recovery feedback"
status: completed
wave: 3
depends_on:
  - "02-continuation-owner"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 03: Render truthful recovery feedback

## Summary

Drive recovery copy from actual mode, phase, and disposition rather than the
classifier. Render one localized, cancellable notice on desktop and phone,
including when the admitted continuation is running.

## In scope

- Backend status/error metadata from Task 02, started attempt count, neutral
  legacy fallback, and typed truthful refusal/exhaustion/cancellation copy.
- Preserve Task 00's independently delivered neutral zero-attempt behavior;
  extend it with actual disposition and started counts from Task 02.
- Shared ActionMeta and recovery view-model updates; waiting countdown,
  reconnecting and continuing labels; existing task/preview recovery surfaces.
- Audit current RUNNING warning suppression so the same owned notice, not a
  second card, stays reachable while continuation is admitted.
- Desktop row and phone stacked actions, touch targets, wrapped details, focus
  retention and polite phase announcements without per-tick announcements.
- Locale keys for all six languages and generated Traditional Chinese/pseudo.

## Out of scope

New settings page, client-owned dispatch, changes to provider classification or
safe admission, transcript retraction, and a second recovery store/banner.

## Acceptance

- No started retry renders exhaustion copy; actual exhaustion reports the
  started count. Missing legacy metadata renders a neutral connection failure.
  Truthful copy works when the continuation toggle is disabled.
- The same persisted notice reflects waiting, reconnecting, and continuing,
  keeps Cancel reachable during RUNNING, and disappears durably after retirement.
- Desktop/phone share semantics; phone actions are at least 44px, details wrap,
  no horizontal overflow appears, and phase changes preserve focus and use
  localized accessible announcements.

## ASCII UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview). Excerpts below
retain the required structure; copy is illustrative and must be localized.

UI-01, desktop waiting/active region in Chat:

```text
| Connection interrupted. Continuing in 0:05.              [Cancel] |
| Cursor | Attempt 1 of 5 | Previous conversation is preserved.      |
  Waiting -> Reconnecting... -> Continuing previous request...
```

UI-01, phone in the existing transcript scroll region:

```text
| Connection interrupted              |
| Continuing in 0:05                  |
| Cursor | Attempt 1 of 5             |
| Previous conversation is preserved. |
| [             Cancel             ] |
```

UI-02, manual unsafe failure, desktop then phone:

```text
| Connection interrupted. Tool work needs checking.               |
| [Resume session] [Start fresh session]                           |
| > Technical details                                             |

| Connection interrupted              |
| Tool work needs checking.           |
| [        Resume session          ] |
| [      Start fresh session       ] |
| > Technical details                 |
```

Map to criteria `.003.1`-`.003.4`. Chat retains one scroll owner and current
composer safe-area handling. Fine-pointer desktop controls are 28px; phone and
coarse-pointer actions are at least 44px. Details expand inline. No focus jump.
Task 04 owns rendered desktop/phone proof against these structural annotations.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'Test(InterruptionRecoveryDisposition|CreateRecoveryStatusMessage|TransientFailureLabelAndManualMessage)' -count=1)
(cd apps/web && pnpm test -- components/task/chat/messages/interruption-recovery-feedback.test.ts components/task/chat/messages/action-message.test.tsx components/task/chat/session-recovery-model.test.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
```

`session-recovery-model.test.ts` and the feedback test are new delivery targets.
Existing action-message tests retain replay-mode assertions alongside new
continuation-mode cases. If a test is split, update this exact command before
marking done.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- New `interruption_recovery_disposition_test.go`
- `apps/web/components/task/chat/messages/{action-message,action-message-details}.tsx`
- `apps/web/components/task/chat/types.ts`
- `apps/web/components/task/chat/session-recovery-model.ts`
- New `messages/interruption-recovery-feedback.test.ts` and existing recovery tests
- New `apps/web/components/task/chat/session-recovery-model.test.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,pseudo}/{chat,task}.json`

## Dependencies

Task 02 supplies authoritative disposition/mode/phase and cleanup behavior.

## Risks

RUNNING warning suppression can hide Cancel; missing counters after entry
retirement can recreate false exhaustion; localized long details can overflow.
Do not infer exhaustion from an error code or a browser countdown reaching zero.

## Parallelism

`sequential`

## Inputs

Design Feedback contract and Desktop and phone composition; mobile-parity,
control sizing, existing transient retry renderer and session recovery model.

## Results

Implemented waiting/reconnecting/continuing feedback and visible Cancel during
RUNNING. Phase announcements are separate from the ticking countdown. Phone
controls use the shared touch-size button path and wrapping layout. Neutral,
actual-count exhaustion, and cancellation copy are localized in all six
languages and pseudo. Pure feedback/recovery tests and existing action tests
passed (39 tests), with typecheck, full web lint, and i18n checks passing.
Rendered desktop and phone checks pass, including cancellation during RUNNING,
neutral manual recovery, 44px touch actions, details wrapping, and no horizontal
overflow. Task 04 records the integration matrix.

PR review remediation adds localized refusal explanations, restores legacy retry live-region semantics, and gates all retry rendering on terminal visibility. The final four-file web run passes 55 tests, including fixture cleanup regressions; typecheck, focused lint, and localization checks pass.
