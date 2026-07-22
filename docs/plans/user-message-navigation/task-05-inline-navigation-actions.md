---
id: "05-inline-navigation-actions"
title: "Inline user message navigation actions"
status: done
wave: 4
depends_on: ["01-navigation-controller", "03-renderer-integration", "04-e2e"]
plan: "plan.md"
spec: "../../specs/ui/user-message-navigation.md"
---

# Task 05: Inline User Message Navigation Actions

## Acceptance

- User prompts expose accessible up/down actions in the existing message action row; the floating viewport rail and its mobile content clearance are removed.
- Each action navigates relative to its owning prompt, preserves multi-page previous navigation and renderer parity, and uses correct busy/boundary states.
- Destination feedback is a subtle, layout-neutral outline with no background fill; desktop hover/focus and mobile 44px touch behavior pass rendered E2E coverage.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-message-navigation.test.ts components/task/chat/messages/message-actions.test.tsx components/task/chat/messages/chat-message.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/message-list-virtuoso.test.tsx
cd apps/web && pnpm run typecheck
make build-web
cd apps/web && pnpm e2e:run tests/chat/user-message-navigation.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-user-message-navigation.spec.ts
```

## Files Likely Touched

- `apps/web/hooks/use-message-navigation.ts`
- `apps/web/hooks/use-message-navigation.test.ts`
- `apps/web/components/task/chat/user-message-navigation-context.tsx`
- `apps/web/components/task/chat/user-message-navigation-rail.tsx` (remove)
- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-actions.test.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-virtuoso.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/chat/user-message-navigation.spec.ts`
- `apps/web/e2e/tests/chat/mobile-user-message-navigation.spec.ts`

## Mobile Design Contract

- Desktop navigation lives in the existing hover/focus message action row.
- Mobile uses that same inline row, which is already directly visible; only the two new navigation controls receive 44px touch geometry.
- `MessageActions` is the nearest shipped exemplar for disclosure, ordering, and icon styling.
- Inline controls fit a frequent contextual action better than a drawer or floating viewport overlay.
- The message list remains the single scroll owner, no safe-area overlay clearance is needed, and E2E proves no horizontal overflow.

## Dependencies

Tasks 01, 03, and 04 provide the controller, renderer adapters, and long-history fixtures being revised.

## Inputs

- Revised spec `What`, `Mobile Contract`, `Failure Modes`, and `Scenarios`.
- Existing `MessageActions` hover/focus and coarse-pointer behavior.
- Existing renderer-specific stable destination scrolling and cancellation.

## Output Contract

Report controller/context API changes, removed rail files, highlight CSS, desktop/mobile E2E results, screenshots, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
