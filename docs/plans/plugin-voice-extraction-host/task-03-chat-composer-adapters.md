---
id: "03-chat-composer-adapters"
title: "Adapt chat and Quick Chat composers"
status: done
wave: 2
depends_on: ["01-publish-composer-contract"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/voice-extraction-host.md"
---

# Task 03: Adapt Chat And Quick Chat Composers

## Acceptance

- Desktop/mobile task chat and Quick Chat pass a live composer capability through
  `chat-input-actions`, with correct surface, presentation, task, and session metadata.
- Insertion replaces the active TipTap selection with native spacing/focus semantics; submit delegates
  to the existing draft state machine and preserves native submission behavior.
- Unit tests cover blocked, stale, task/session-switch, and task-less Quick Chat cases.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat lib/plugins components/quick-chat
cd apps/web && pnpm run typecheck
```

Follow RED-GREEN-REFACTOR. Keep `useChatInputState` and `useSubmitHandler` as the only submit owners.

## Files Likely Touched

- `apps/web/components/task/chat/chat-input-plugin-actions.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-desktop.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/chat/use-chat-input-state.ts`
- `apps/web/components/task/chat/use-tiptap-editor.ts`
- focused chat and Quick Chat tests

## Mobile Design Contract

Reuse the native mobile toolbar and its scroll/overflow behavior. The plugin action remains secondary
to send/cancel, has a >=44px touch target, and receives the same capability semantics as desktop.

## Risks

- Fast insertion then submit must read the updated value ref before React effects.
- An old session capability must return `unavailable`, never insert into the newly active session.
