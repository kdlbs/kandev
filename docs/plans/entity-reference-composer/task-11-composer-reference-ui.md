---
id: "11-composer-reference-ui"
title: "Composer entity reference interaction"
status: completed
wave: 4
depends_on: ["10-frontend-search-client"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 11: Composer Entity Reference Interaction

## Acceptance

- Valid `#` triggers in task/Quick Chat open descriptor-driven grouped search, including a generic unknown-provider fallback; arrows + Enter/Tab and touch insert a non-sending atom with trailing space.
- Passthrough stays literal; Kandev task discovery remains under `@`, while `#` defensively omits any Kandev-task group returned by the search service.
- Draft/history and reverse-search entries round-trip generated Markdown plus metadata, without converting ordinary links; popup stays visual-viewport-contained with one scroll owner and 44 px rows.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/chat/tiptap-helpers.test.ts components/task/chat/use-tiptap-editor.test.ts components/task/chat/tiptap-editor-history.test.ts components/task/chat/popup-menu.test.tsx components/task/chat/entity-reference-menu.test.tsx
cd apps && pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/task/chat/entity-reference-types.ts`
- `apps/web/components/task/chat/tiptap-entity-reference-extension.tsx`
- `apps/web/components/task/chat/tiptap-entity-reference-suggestion.ts`
- `apps/web/components/task/chat/entity-reference-menu.tsx`
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/use-tiptap-editor.ts`
- `apps/web/components/task/chat/tiptap-helpers.ts`
- `apps/web/components/task/chat/message-history.ts`
- `apps/web/components/task/chat/tiptap-editor-history.ts`
- `apps/web/components/task/chat/popup-menu.tsx`
- `apps/web/components/task/chat/chat-input-body.tsx`
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/passthrough-chat-composer.tsx`
- `apps/web/components/task/chat/tiptap-helpers.test.ts`
- `apps/web/components/task/chat/use-tiptap-editor.test.ts`
- `apps/web/components/task/chat/tiptap-editor-history.test.ts`
- `apps/web/components/task/chat/popup-menu.test.tsx`
- `apps/web/components/task/chat/entity-reference-menu.test.tsx`

## Dependencies

Task 10.

## Inputs

Spec interaction/state/mobile sections; existing slash suggestion/menu; Mobile UI Language; current legacy mention tests.

## Output contract

Report trigger/gating/serialization/mobile geometry, files changed, exact tests/typecheck, rendered mobile check, blockers, risks, and mark task/plan done.
