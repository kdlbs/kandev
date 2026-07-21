---
id: agent-message-comments-01
title: Comment foundation
status: complete
wave: 1
depends_on: None
plan: plan.md
---

## Acceptance

- `AgentMessageComment` and `MessageTextAnchor` are part of the frontend union with type guards, sessionStorage persistence/hydration, and Markdown formatting.
- Unit tests cover round-trip persistence/hydration, anchor normalization, eligible-message filtering, and multiline Markdown.
- Existing comment types and formatting remain compatible.

## Files

- `apps/web/lib/state/slices/comments/types.ts`
- `apps/web/lib/state/slices/comments/comments-store.ts`
- `apps/web/lib/state/slices/comments/persistence.ts`
- `apps/web/lib/state/slices/comments/format.ts`
- `apps/web/lib/state/slices/comments/*.test.ts`
- `apps/web/lib/chat/agent-message-comments.ts`

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/comments lib/chat/agent-message-comments.test.ts`

## Output

Implemented the comment union, sessionStorage-backed hydration, message-local anchors, Markdown formatting, and focused persistence/format/anchor tests.

## Output contract

Summarize model, helpers, tests, touched files, blockers, and follow-up risks.
