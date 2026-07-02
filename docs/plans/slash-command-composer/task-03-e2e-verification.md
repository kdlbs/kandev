---
id: "03-e2e-verification"
title: "E2E slash command composer verification"
status: done
wave: 3
depends_on: ["01-frontend-selection", "02-unit-tests"]
plan: "plan.md"
spec: "../../specs/ui/slash-command-composer.md"
---

# Task 03: E2E Slash Command Composer Verification

## Acceptance

- Desktop task chat E2E proves Enter selection inserts a slash command draft and does not create a user message.
- Desktop task chat E2E proves an explicit Send after editing the selected command sends the full draft.
- Quick chat E2E proves the shared composer keeps a selected command as a draft.
- Mobile E2E proves touch selection keeps the command as a draft and does not rely on hover or desktop-only controls.

## Verification

```bash
cd apps/web
pnpm e2e:run tests/chat/slash-command-composer.spec.ts tests/chat/mobile-slash-command-composer.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/chat/slash-command-composer.spec.ts`
- `apps/web/e2e/tests/chat/mobile-slash-command-composer.spec.ts`
- `apps/web/e2e/pages/session-page.ts` if a small reusable composer helper is needed.

## Dependencies

Tasks 01 and 02.

## Inputs

- Existing task chat patterns in `apps/web/e2e/tests/session/session-resume-commands.spec.ts`.
- Existing quick chat helper pattern in `apps/web/e2e/tests/chat/quick-chat.spec.ts`.
- Existing `seedAvailableCommands` helper if a deterministic store seed is preferred over waiting for the mock agent's WebSocket command update.

## Output contract

Report the exact E2E command, pass/fail result, artifact paths for failures, and whether desktop and mobile projects both exercised the new behavior.
