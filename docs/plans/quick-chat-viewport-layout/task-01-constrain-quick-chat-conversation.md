---
id: "01-constrain-quick-chat-conversation"
title: "Constrain the Quick Chat conversation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001
acceptance_criteria:
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.1
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.2
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.3
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.4
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.5
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.6
system_design:
  - ../../specs/ui/system-design/quick-chat-viewport-layout.md
---

# Task 01: Constrain the Quick Chat conversation

## Summary

Complete the Quick Chat flex height chain. Add rendered regressions for empty,
long, laptop-height, and phone conversation states.

## In scope

- Add the missing flex behavior to the conversation-slot wrapper.
- Add desktop geometry and transcript-overflow assertions.
- Add mobile composer-containment assertions.

## Out of scope

- Changes to composer size, toolbar behavior, or resize state.
- Changes to tab, terminal, session, API, or persistence behavior.
- Changes to shared `DialogContent` styles.

## Acceptance

- A new conversation fills the available slot and keeps the composer at the
  dialog bottom.
- A bulk transcript scrolls inside `SessionPanelContent` while the composer
  remains inside the dialog.
- The same composer-containment rule passes in the phone surface.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web run typecheck
pnpm --dir web e2e:run --project chromium tests/chat/quick-chat.spec.ts -- --grep "viewport layout"
pnpm --dir web e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-entry.spec.ts -- --grep "composer containment"
```

Write and run the desktop regression before the source correction. The test
must fail because the composer does not use the full conversation slot.

The managed E2E runner rebuilds the production Vite assets before each final
browser result.

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-session-view.tsx`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`

## Dependencies

None.

## Risks

- Message loading can change layout during a geometry read. Wait for the chat
  editor and the final bulk message before the assertion.
- The test must read the real transcript scroll element. A synthetic wrapper
  can hide a second scroll owner.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/quick-chat-viewport-layout.md`
- `docs/specs/ui/system-design/quick-chat-viewport-layout.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Results

- Added the missing flex boundary to the active Quick Chat conversation slot.
- Added a desktop geometry test for a new chat and a 20-message transcript.
  Before the fix, the composer was 393 px above the dialog bottom. After the
  fix, the composer stays at the bottom and the message scroller owns the
  transcript overflow.
- Added a phone geometry test. Before the fix, the composer was 392 px above
  the dialog bottom. After the fix, the full-height dialog contains the
  composer at its bottom.
- The web type check, desktop E2E test, mobile E2E test, Prettier check,
  specification lint, and diff check passed.
