---
id: "03-remove-host-model-gates"
title: "Remove host model launch gates"
status: done
wave: 3
depends_on: ["02-persist-model-warning"]
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 03: Remove host model launch gates

## Acceptance

- Host-catalog differences show advisory text but never disable task-create or Office profile options.
- Persisted model-selection metadata renders one localized warning with requested, effective, agent, executor, and remediation details.
- Desktop and mobile use the existing chat message flow, with no hover-only information or new scroll owner.

## TDD scenarios

1. RED: Change profile-option tests from disabled behavior to selectable warnings.
2. RED: Add status-message tests for known effective, unknown effective, and explicit fallback outcomes.
3. RED: Add WebSocket state tests for provider-default and explicit-fallback live decisions.
4. GREEN: Remove host-only gates and add the structured warning renderer.
5. REFACTOR: Share warning derivation between task-create and Office selectors.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-options app/office/setup/agent-profile-setup-controls components/task/chat/messages/status-message lib/ws/handlers/session-models`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check`
- `cd apps/web && pnpm run i18n:ratchet`
- `cd apps && pnpm --filter @kandev/web lint`
- `git diff --check`

## Files likely touched

- `apps/web/components/task-create-dialog-options.tsx`
- `apps/web/components/task-create-dialog-options.test.tsx`
- `apps/web/app/office/setup/agent-profile-setup-controls.tsx`
- `apps/web/app/office/setup/agent-profile-setup-controls.test.tsx`
- `apps/web/components/task/chat/messages/status-message.tsx`
- `apps/web/components/task/chat/messages/status-message.test.tsx`
- `apps/web/lib/ws/handlers/session-models.ts`
- `apps/web/lib/ws/handlers/session-models.test.ts`
- `apps/web/lib/types/session-events.ts`
- `apps/web/src/locales/en/chat.json`
- `apps/web/src/locales/pseudo/chat.json`
- `apps/web/src/locales/pt-pt/chat.json`
- `apps/web/src/locales/zh-cn/chat.json`
- `apps/web/src/locales/zh-hk/chat.json`
- `apps/web/src/locales/zh-tw/chat.json`

## Dependencies

- Task 02 supplies the stored message and live-event contracts.

## Parallelism

Sequential after Task 02.
This task owns frontend selector, message, state, and locale files.

## Inputs

- The warning metadata contract from Task 02.
- Existing disabled profile logic.
- Existing status-message and session-model handlers.
- Existing mobile chat composition.

## Output contract

Report selector behavior, warning copy, accessibility, translations, RED evidence, GREEN evidence, and test results.

## Risks

- Executor compatibility gates must remain active.
- A warning must not imply that the host catalog is authoritative.
- Missing optional metadata must not produce broken sentences.

## Results

Removed host-only launch gates, added advisory selector copy, and rendered the
localized persisted warning with optional metadata handling. Focused frontend
tests, typecheck, lint, i18n check, and i18n ratchet passed.
