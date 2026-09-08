---
created: 2026-09-01
status: done
requirements:
  - REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001
system_design:
  - ../../specs/ui/system-design/quick-chat-viewport-layout.md
legacy_specs: []
---

# Implementation Plan: Quick Chat viewport layout

## Overview

Restore the missing flex boundary in the Quick Chat conversation slot. Then
prove composer containment and transcript scrolling on desktop and mobile.

The source correction and its rendered regressions form one frontend work
order. No backend or state change is necessary.

## Scope

### In scope

- Make the active conversation fill the remaining dialog height.
- Keep the composer at the bottom for an empty or short transcript.
- Keep a long transcript inside the existing message scroll owner.
- Prove the behavior at laptop and phone viewport sizes.

### Out of scope

- Change the composer size or resize control.
- Change Quick Chat tabs, session state, or terminal layouts.
- Change the shared dialog primitive.

## Technical approach

### Conversation height chain

Update the conversation-slot wrapper in
`apps/web/components/quick-chat/quick-chat-session-view.tsx`. Make this wrapper
a flex container so that `QuickChatContent` can resolve its existing `flex-1`
height.

Keep the existing `min-h-0` chain. Keep `QuickChatContent`, `MessageList`, and
`SessionPanelContent` as the current content and scroll boundaries.

### Rendered regression coverage

Update `apps/web/e2e/tests/chat/quick-chat.spec.ts`. Add a laptop-height scenario
that starts a new chat and checks composer containment. Send the mock-agent
bulk command to create a long transcript. Then check that the transcript
scrolls while the composer remains inside the dialog.

Update `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`. Start a chat
through the existing phone entry point. Check that the composer remains inside
the full-height dialog.

## Tests

- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.1 through 001.4 and 001.6:** The
  desktop geometry scenario checks the dialog, conversation, transcript, and
  composer bounds at a laptop-height viewport.
- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.3 and 001.6:** The bulk transcript
  produces a message scroll height greater than its client height.
- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.5:** The mobile entry scenario checks
  composer containment in the full-height dialog.

## E2E tests

- `apps/web/e2e/tests/chat/quick-chat.spec.ts` proves the desktop short and long
  transcript states in the `chromium` project.
- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts` proves the phone
  composer state in the `mobile-chrome` project.

## Work orders

- [x] [Task 01: Constrain the Quick Chat conversation](task-01-constrain-quick-chat-conversation.md)

## Verification results

- `pnpm --filter @kandev/web run typecheck`: passed.
- Desktop Quick Chat viewport E2E test: passed in Chromium. The test covers a
  new chat and a long transcript at 1440 by 800.
- Mobile composer-containment E2E test: passed in mobile Chrome.
- Prettier, specification lint, and diff checks: passed.

## Risks

- An assertion against visibility alone can miss a composer that sits in the
  wrong vertical position. The E2E tests must compare element bounds.
- The long-transcript scenario must inspect the existing transcript scroll
  element. It must not accept dialog-level overflow.
- The change affects the shared conversation view on desktop and phone.
