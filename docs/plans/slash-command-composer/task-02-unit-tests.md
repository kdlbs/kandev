---
id: "02-unit-tests"
title: "Unit tests for slash command insertion"
status: done
wave: 2
depends_on: ["01-frontend-selection"]
plan: "plan.md"
spec: "../../specs/ui/slash-command-composer.md"
---

# Task 02: Unit Tests for Slash Command Insertion

## Acceptance

- A unit test covers slash command insertion text normalization for normal command names, labels that already include `/`, and command names with punctuation such as `tool:read`.
- Existing submit-key tests still prove plain Enter defers while a suggestion menu is open.
- Chat input tests are updated for the removed auto-submit callback props.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- components/task/chat/slash-command-types.test.ts components/task/chat/use-tiptap-editor.test.ts components/task/chat/use-chat-input-container.test.ts components/task/chat/chat-input-body.test.tsx
```

## Files likely touched

- `apps/web/components/task/chat/slash-command-types.test.ts`
- `apps/web/components/task/chat/use-tiptap-editor.test.ts`
- `apps/web/components/task/chat/use-chat-input-container.test.ts`
- `apps/web/components/task/chat/chat-input-body.test.tsx`

## Dependencies

Task 01.

## Inputs

- `formatSlashCommandInsertion` or equivalent helper from Task 01.
- Spec scenarios covering Enter selection and no auto-submit.

## Output contract

Report tests added or updated, focused test command output, and any behavior that remains covered only by E2E.
