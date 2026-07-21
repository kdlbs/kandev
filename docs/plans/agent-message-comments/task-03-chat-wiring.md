---
id: agent-message-comments-03
title: Chat wiring and delivery
status: complete
wave: 3
depends_on: agent-message-comments-01, agent-message-comments-02
plan: plan.md
---

## Acceptance

- Add persists pending context and exposes removable message-comment chips in Task Chat and Quick Chat composers.
- Composer send includes and clears message comments only on success; failed sends preserve context. Existing passthrough review-comment behavior remains intact.
- Run uses existing direct message and busy queue paths, removing the comment only after successful delivery.

## Files

- `apps/web/lib/types/context.ts`
- `apps/web/components/task/chat-context-items.ts`
- `apps/web/components/task/chat/context-items/*`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/hooks/domains/comments/use-run-comment.ts`
- `apps/web/components/task/passthrough-chat-composer.tsx`
- focused tests

## Verification

- focused composer/store/run tests
- passthrough regression tests

## Output

Wired Task Chat and Quick Chat context chips, composer inclusion/cleanup, direct and queued Run delivery, and passthrough preservation.

## Output contract

Summarize Task Chat/Quick Chat/passthrough wiring, direct/queue behavior, tests, and risks.
