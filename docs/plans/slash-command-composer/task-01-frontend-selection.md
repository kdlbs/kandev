---
id: "01-frontend-selection"
title: "Frontend slash command selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/slash-command-composer.md"
---

# Task 01: Frontend Slash Command Selection

## Acceptance

- Selecting a slash command in the shared TipTap chat composer inserts the selected slash command into the draft and never calls `onSubmit`.
- Focus stays in the composer with the caret after the inserted command text.
- The stale `handleAgentCommand -> onSubmit('/cmd')` plumbing is removed or made unreachable.

## Verification

```bash
cd apps
pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/task/chat/slash-command-types.ts`
- `apps/web/components/task/chat/tiptap-suggestion.tsx`
- `apps/web/components/task/chat/slash-command-menu.tsx`
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/chat-input-body.tsx`
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/use-chat-input-container.ts`
- `apps/web/components/task/chat/chat-input-body.test.tsx`
- `apps/web/hooks/use-inline-slash.ts`

## Dependencies

None.

## Inputs

- Spec scenarios for non-sending selection and explicit send.
- Existing `decideSubmitShortcut` tests in `apps/web/components/task/chat/use-tiptap-editor.test.ts`.
- Existing `session.available_commands` state in `apps/web/lib/state/slices/session-runtime`.

## Output contract

Report files changed, whether `use-inline-slash.ts` was deleted or updated, the typecheck result, and any remaining stale prop references.
