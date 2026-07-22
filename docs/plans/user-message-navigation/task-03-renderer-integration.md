---
id: "03-renderer-integration"
title: "Message list renderer integration"
status: done
wave: 2
depends_on: ["01-navigation-controller", "02-floating-rail"]
plan: "plan.md"
spec: "../../specs/ui/user-message-navigation.md"
---

# Task 03: Message List Renderer Integration

## Acceptance

- Native and Virtuoso lists use the shared controller and rail with equivalent viewport-origin, pagination, center-or-boundary positioning, boundary, and highlight behavior.
- Prepending older pages retains the current stop by message ID; Virtuoso resolves the updated item index before scrolling and highlights only after mount.
- The explicit **Load older messages** button and automatic top pagination continue to work, and mobile content clears the visible rail.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/message-list-shared.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/message-list-virtuoso.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-shared.test.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/message-list-virtuoso.tsx`
- `apps/web/components/task/chat/message-list-virtuoso.test.tsx`
- `apps/web/components/task/chat/messages/turn-group-message.tsx`

## Dependencies

Tasks 01 and 02.

## Inputs

- Controller and rail APIs from Tasks 01 and 02.
- Native `useScrollPositionOnPrepend` and `useScrollToMessage` patterns.
- Virtuoso `useStableFirstItemIndex`, `rangeChanged`, and `scrollToIndex` patterns.
- Existing `.search-flash` reduced-motion styling in `apps/web/app/globals.css`.

## Output Contract

Report both renderer paths, files changed, focused tests/typecheck run, fallback preservation, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
