---
id: "00-truthful-manual-recovery"
title: "Correct manual recovery copy independently of continuation"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 00: Correct manual recovery copy independently of continuation

## Summary

Deliver the baseline zero-attempt copy correction while native compatibility
work proceeds. A transient classification does not establish retry exhaustion.
Actual continuation phases, cancellation dispositions and exhausted counts
remain owned by Tasks 02 and 03.

## Scope and ownership

- Own the neutral manual branch in orchestrator recovery-message creation and
  its tests. Preserve classification, retry admission and raw-detail sanitizing.
- Reuse existing `failure_kind` metadata (`provider_interrupted`) to localize
  the manual summary in both transcript and composer recovery surfaces.
- Own the recovery-copy helper test, shared locale key in all six locales and
  pseudo, focused desktop/phone cancellation assertions, and the short public
  session-recovery clarification.
- Do not introduce continuation flags, dispatch, native fallback, new actions,
  or layout changes. Missing legacy metadata remains on the existing fallback.

## Acceptance

- Missing evidence, prior output and tool work do not claim that retries ran.
- Resume/fresh actions and bounded technical details remain available.
- Existing capacity and quota recovery still retain their specialized behavior.
- New manual summaries are localized, and desktop/phone recovery remains usable.

## ASCII UI preview

Reuse the shipped inline Chat recovery card and its scroll owner.

```text
Desktop: | Session recovery failed                                  |
         | The agent was interrupted. Resume to try again, or ...    |
         | [Resume session] [Start fresh session]                    |
         | > Technical details                                      |

Phone:   | Session recovery failed                  |
         | The agent was interrupted.               |
         | Resume to try again, or start a fresh     |
         | session.                                 |
         | [           Resume session             ] |
         | [         Start fresh session          ] |
         | > Technical details                      |
```

Copy wraps within the existing card. Existing phone actions retain their touch
sizes; no new focus behavior, surface or control is introduced.

## Verification

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'Test(InterruptionRecoveryDisposition|CreateRecoveryStatusMessage|TransientFailureLabelAndManualMessage|HandleTransientFailure_)' -count=1)
(cd apps/backend && golangci-lint run ./internal/orchestrator ./internal/agent/runtime/lifecycle --timeout=5m)
(cd apps/web && pnpm test -- components/task/chat/session-recovery-model.test.ts components/task/chat/messages/action-message.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/transient-retry.spec.ts --grep 'Cancel stops' --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-transient-retry.spec.ts --retries=0)
```

The second managed browser run reuses the first run's freshly built artifacts.
When the host Go cache is inaccessible, use a writable temporary `GOCACHE` for
backend commands; this changes cache placement, not product configuration.

## Files

`apps/backend/internal/orchestrator/{event_handlers_agent,event_handlers_transient}.go`
and recovery tests; `apps/web/components/task/chat/session-recovery-model.ts`
and its test; `messages/action-message.tsx`; localized `chat.json` catalogs;
existing desktop/mobile transient-retry E2E specs; public session guide.

## Dependencies and risks

Independent of native continuation support. Do not infer started counts from
scheduled attempt numbers or retained classification. Task 03 will replace the
neutral summary with authoritative dispositions when the retry owner exists.

## Parallelism

`sequential`

## Results

Completed. The backend regression first failed on false exhaustion in three
refusal cases, and the frontend regression first failed on the same claim.
Targeted race tests, feedback/action/recovery unit tests, typecheck, lint, and
complete locale checks pass. Desktop manual and cancellation paths render
neutral copy and retain technical details. Phone cancellation passes in the
focused mobile run. Docker browser verification replaces the earlier host
Chromium launch failure; the implementation does not depend on host browser
permissions.
