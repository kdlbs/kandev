---
id: "04-render-provider-quota-recovery"
title: "Render localized quota recovery"
status: done
wave: 4
depends_on: ["03-classify-provider-failure"]
plan: "plan.md"
spec: "../../specs/agent-stall-recovery/spec.md"
---

# Task 04: Render Localized Quota Recovery

## Acceptance

- `provider_quota_limited` metadata renders localized model/reset guidance,
  existing recovery actions, and sanitized technical details collapsed by
  default; no raw OpenCode URL or identifier is displayed.
- Unknown provider failures retain the generic settled-error card, and running
  inactivity notices retain their neutral compact behavior.
- Desktop and phone use the same logic; phone actions remain at least 44px,
  expanded details stay within the chat width, and no workflow depends on
  hover.

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/action-message.test.tsx`
- `cd apps/web && pnpm run typecheck`

Use TDD for provider metadata, missing model/reset fallbacks, details
disclosure, generic-error preservation, and pseudo-locale rendering.

## Files likely touched

- `apps/web/components/task/chat/types.ts`
- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/src/locales/en/chat.json`
- `apps/web/src/locales/pseudo/chat.json`

## Dependencies

Task 03.

## Parallelism

Sequential. Task 05 validates this presentation and its backend metadata.

## Inputs

- Spec provider-limit desktop/mobile scenarios
- Plan `Localized provider-limit recovery` and mobile design contract
- Existing `MissingBranchRecovery`, `TechnicalDetails`, `ActionButtons`, and
  `useTranslation("chat")` patterns

## Risks

- Reset timestamps must be formatted at render time and must not call `t()` at
  module scope.
- The safe technical string is display-only; control flow must use
  `failure_kind` rather than translated copy or substring matching.

## Output contract

Report RED assertions, translation keys, desktop/mobile hierarchy, technical
details behavior, exact test/typecheck results, files changed, blockers, and
risks. Mark this task `done` and update its plan checkbox in the same
conversation.

## Results

Added the localized `ProviderQuotaRecovery` inline card with model/reset
fallbacks, collapsed technical details, existing recovery actions, and shared
desktop/mobile layout. Unknown failures and running stall notices retain their
existing renderers. The focused component suite passed all 17 tests; web
typecheck, i18n key/pseudo checks, and the new-code ratchet passed.
