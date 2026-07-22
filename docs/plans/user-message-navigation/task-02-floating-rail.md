---
id: "02-floating-rail"
title: "Floating user message rail"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/user-message-navigation.md"
---

# Task 02: Floating User Message Rail

## Acceptance

- A shared vertical up/down rail exposes accessible previous/next controls, busy and boundary states, desktop hover/focus disclosure, and persistent coarse-pointer controls at least 44px square.
- Safe-area positioning and the mobile content-clearance contract prevent overlap and horizontal overflow.
- Existing per-message left/right navigation controls and props are removed without changing other message actions.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/user-message-navigation-rail.test.tsx components/task/chat/messages/chat-message.test.tsx components/task/chat/messages/message-actions.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/task/chat/user-message-navigation-rail.tsx`
- `apps/web/components/task/chat/user-message-navigation-rail.test.tsx`
- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/components/task/chat/messages/chat-message.test.tsx`
- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-actions.test.tsx`

## Dependencies

None. Keep the rail presentational so Task 01 can define the controller independently.

## Inputs

- Spec `Mobile Contract` and disclosure/removal scenarios.
- `apps/web/hooks/use-responsive-breakpoint.ts` for fine/coarse pointer behavior.
- `apps/web/components/task/chat/voice-input-button.tsx` for coarse-pointer target sizing.
- Existing `MessageActions` hover/focus behavior.

## Output Contract

Report component API, files changed, rendered tests/typecheck run, mobile geometry decisions, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
