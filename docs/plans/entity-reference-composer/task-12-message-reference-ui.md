---
id: "12-message-reference-ui"
title: "Reference submission and sent rendering"
status: completed
wave: 4
depends_on: ["08-message-reference-metadata", "11-composer-reference-ui"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 12: Reference Submission and Sent Rendering

## Acceptance

- Named composer submit payload carries references through direct send and queue add/update without stale metadata.
- Sent and queued generated links render as clickable chips only when matched by typed metadata; ordinary Markdown stays unchanged.
- Existing files/prompts/plan/task mentions, passthrough submission, attachments, comments, and queue behavior retain parity.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/use-message-handler.test.ts components/task/chat/use-chat-input-state.test.ts lib/api/domains/queue-api.test.ts components/task/chat/messages/chat-message.test.tsx components/task/chat/queued-ghost-message.test.tsx components/task/passthrough-chat-composer.test.ts
cd apps && pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/use-chat-input-container.ts`
- `apps/web/components/task/chat/use-chat-input-state.ts`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/components/task/chat/messages/entity-reference-chip.tsx`
- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/passthrough-chat-composer.tsx`
- `apps/web/hooks/use-message-handler.test.ts`
- `apps/web/components/task/chat/use-chat-input-state.test.ts`
- `apps/web/lib/api/domains/queue-api.test.ts`
- `apps/web/components/task/chat/messages/chat-message.test.tsx`
- `apps/web/components/task/chat/queued-ghost-message.test.tsx`
- `apps/web/components/task/passthrough-chat-composer.test.ts`

## Dependencies

Tasks 08 and 11.

## Inputs

Spec persistence/scenarios; backend metadata contract; existing prompt mention rendering and queue metadata shapes.

## Output contract

Report payload migration/render matching/queue edit semantics, files changed, exact tests/typecheck, blockers, risks, and mark task/plan done.
