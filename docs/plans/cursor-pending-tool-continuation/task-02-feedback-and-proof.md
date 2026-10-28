---
id: "02-feedback-and-proof"
title: "Verify feedback and desktop/mobile behavior"
status: complete
wave: 2
depends_on: ["01-runtime-policy"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.8
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.9
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 02: Verify feedback and desktop/mobile behavior

## Summary

Preserve accurate retry progress and explain remaining refusals. Prove the
pending-tool outcome on desktop and phone, and document the chosen policy.

## In scope

Propagate an attested refusal reason into retained-turn failure metadata. Update
`retainedTurnRecoveryKey` and action details so zero attempts never imply retries
ran. Preserve historical unknown reasons with a neutral fallback. Translate all
new copy in the seven required locales; generate Traditional Chinese catalogs.
Extend mock continuation scenarios and shared helper for retained V3 mixed tools,
late updates, waiting cancellation, and remaining refusal controls. Keep explicit
legacy/unknown fixtures. Update the task/workflow public recovery guidance,
separating transport policy from capacity recovery and stating duplicate-effect
limits. Do not publish planned behavior in public docs before implementation.

## Out of scope

New dialogs, layout redesign, configuration, extra retry owners, native
idempotency claims, and a separate capacity-policy change.

## Acceptance

1. An eligible mixed-tool interruption continues automatically on desktop and
   phone, preserving native identity, hidden prompt, budget, and human priority.
2. Remaining refusal displays its attested reason and actual attempts. Historical
   missing metadata has neutral copy; genuine exhaustion remains distinguishable.
3. Public guidance describes agent-managed inspection and retries without claiming
   tools stopped or results are complete. Mobile controls remain reachable.

## ASCII UI preview

UI-01: Task Chat, retained Cursor transport interruption. Shared desktop/phone
content, reusing `TransientRetryNotice` and existing failure cards.

```text
Before: pending foreground tool
Automatic retry stopped. You can send another message.

After: eligible pending foreground tool
Cursor: Continuing in 5 seconds
Attempt 1 of 5                         [Cancel]

After: remaining refusal, zero attempts
Automatic continuation did not start.
Kandev cannot continue automatically because conversation or safety evidence is missing.
Review the work before sending another message.
```

The words are illustrative localization source copy. Phone keeps the existing
stacked Cancel target of at least 44px. Desktop retains compact inline controls.
Chat owns scrolling; diagnostics wrap without horizontal page overflow. No modal
or new confirmation is added. Existing task Chat is the mobile exemplar; its
continuation card supplies hierarchy, focus behavior, and status announcements.
Map UI-01 to `AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1` through `003.4`.

Full preview: [UI-01 in the plan](plan.md#ascii-ui-preview).

## Verification

From the repository root; install workspace dependencies once if absent.
Managed E2E commands build fresh artifacts. Run desktop and mobile sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./cmd/mock-agent -run 'Test.*Continuation' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(Refusal|RetainedTurn|Interruption)' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/interruption-recovery-feedback.test.ts e2e/helpers/provider-interruption-continuation.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-interruption-continuation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run ESLint on the changed TS/TSX paths, and changed-Go lint on changed Go paths;
record the final explicit commands. Each modified test suite must run.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go` and tests
- `apps/backend/internal/orchestrator/provider_interruption_refusal_test.go`
- `apps/backend/cmd/mock-agent/interruption_continuation.go` and tests
- `apps/web/components/task/chat/messages/interruption-recovery-feedback.ts` and tests
- `apps/web/components/task/chat/messages/action-message-details.tsx`
- `apps/web/src/locales/*/chat.json`
- `apps/web/e2e/helpers/provider-interruption-continuation.ts` and tests
- `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts`
- `apps/web/e2e/tests/session/mobile-provider-interruption-continuation.spec.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 01. Retained refusal reasons must use its final admission predicate.

## Risks

Generic retained-runtime rendering can mask more specific interruption reasons.
Old persisted records cannot supply evidence that was never recorded. Mock
success proves host behavior, not model-level duplicate prevention.

## Parallelism

`sequential`

## Inputs

- Owning requirements `001`, `002`, and `003`; ADR 2026-10-26.
- System design: Feedback and mobile composition; Agent-managed pending-tool policy.
- Existing continuation E2E fixtures and mobile Chat notice.

## Results

Completed. Retained refusal messages persist an attested reason and actual
attempt count. New feedback copy is localized in all required catalogs, and
legacy refusal metadata receives neutral copy. Mock fixtures and desktop/mobile
coverage prove mixed-tool continuation, human priority, cancellation, and V1/V2
refusal. Public guidance explains the transport-loss policy and its duplicate
effect limit.

Passed verification:

- `(cd apps/web && pnpm exec vitest run components/task/chat/messages/interruption-recovery-feedback.test.ts components/task/chat/messages/action-message.test.tsx e2e/helpers/provider-interruption-continuation.test.ts)` (74 tests).
- `(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)`.
- `(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)`.
- `(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-interruption-continuation.spec.ts)` (16 passed).
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts)` (4 passed).
- `(cd apps/web && pnpm exec eslint components/task/chat/messages/action-message.test.tsx components/task/chat/messages/action-message.tsx components/task/chat/messages/interruption-recovery-feedback.test.ts components/task/chat/messages/interruption-recovery-feedback.ts e2e/helpers/provider-interruption-continuation.test.ts e2e/helpers/provider-interruption-continuation.ts e2e/tests/session/provider-interruption-continuation.spec.ts e2e/tests/session/mobile-provider-interruption-continuation.spec.ts)`; the final mobile spec was rechecked after its synchronization fix.
- `node --test scripts/validate-public-docs.test.mjs` (63 passed) and
  `node scripts/validate-public-docs.mjs` (54 pages).
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

## Review follow-up results (2026-10-26)

Unsupported refusals now use generic wording that does not claim the retained
runtime is unavailable. Unknown or zero-attempt refusals use neutral unavailable
copy. Positive refused attempts and exhaustion retain their separate counts.
Rendered coverage includes an unsupported server-error category with a retained
runtime, an unknown zero-attempt refusal, absent historical attempt metadata,
positive attempts, and exhaustion. Updated required catalogs and regenerated
Traditional Chinese.

Passed:

- `(cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/action-message.test.tsx components/task/chat/messages/interruption-recovery-feedback.test.ts)` (70 tests).
- `(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)`.
- `(cd apps/web && pnpm run i18n:zh-hant)`.
- `(cd apps/web && pnpm run i18n:check)` and `(cd apps/web && pnpm run i18n:ratchet)`.
- `(cd apps/web && pnpm exec eslint components/task/chat/messages/action-message.test.tsx components/task/chat/messages/interruption-recovery-feedback.test.ts)`.
