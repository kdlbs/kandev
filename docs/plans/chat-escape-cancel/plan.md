---
created: 2026-11-05
status: implemented
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-004
system_design:
  - ../../specs/tasks/system-design/task-keyboard-actions.md
legacy_specs: []
---

# Implementation Plan: Chat Escape cancellation

## Overview

Let a user cancel the active agent turn by pressing Escape while the chat
composer is focused. Reuse the task system's existing cancel-turn action and
limit the shortcut to the composer so dialogs, toolbar controls, and transcript
interactions keep their existing key behavior.

## Scope

### In scope

- Add the off-by-default account preference for Escape cancellation.
- Handle Escape in the shared task chat and Quick Chat composer only when the
  setting is enabled and cancellation is available.
- Reuse `handleCancelTurn` and the current resolved session.
- Prove the behavior and guards with unit and desktop/phone browser coverage.

### Out of scope

- A global shortcut binding.
- Cancellation from transcript focus or other chat controls.
- A touch button or a new cancellation transport.

## Technical approach

Use `useChatPanelHandlers` in `apps/web/components/task/chat/chat-input-area.tsx`
and its existing `handleCancelTurn`. The account preference uses the existing
user-settings API and defaults to off. Scope the Escape keydown to the actual
composer. Consume it only when the user enabled the setting, cancellation is
available, no editor control owns Escape, and the event is not an IME
composition event. Quick Chat uses the existing Escape guard registry to keep
Radix from closing the dialog when the composer will cancel the turn. The
resolved session and existing cancel request remain authoritative.

## ASCII UI preview

UI-01: Desktop task chat, agent running, composer focused.

```text
Chat transcript
--------------------------------
Agent is working...
--------------------------------
[ Message composer                 ]  <- focus here, press Escape to cancel
[ Send                             ]
```

UI-02: Phone task chat, attached keyboard, agent running.

```text
+------------------------------+
| Chat transcript              |
| Agent is working...          |
|                              |
| [ Message composer         ] | <- focus here, press Escape
| [ Send                     ] |
+------------------------------+
```

The keyboard settings page exposes a switch, off by default. When enabled, Escape cancels only from the
focused composer while running. Phone composition stays in the existing chat
surface; no touch target, drawer, or scroll behavior changes. Previews map to
AC-TASKS-KEYBOARD-ACTIONS-004.1 through .4.

## Tests

`apps/web/components/task/chat/chat-input-body.test.tsx` covers the composer
key handler, running/idle state, IME composition, repeated keys, and default
prevention. `chat-escape-cancel.test.ts` covers preserving Escape ownership
for editor menus and suggestions. `apps/web/components/settings/general-settings.test.tsx`
covers account-setting reconciliation and saves. `apps/web/components/quick-chat/quick-chat-modal.test.tsx`
covers the dialog guard. Existing chat-input tests cover surrounding composer
behavior.

## E2E tests

- Desktop Chromium: `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`
  focuses the task chat composer, presses Escape during a running turn, and
  observes cancellation; it also confirms Escape outside the composer does not
  cancel.
- Phone Chromium: `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts`
  uses the existing mobile chat layout with an attached keyboard, focuses the
  composer, and presses Escape during a running turn.
- Chromium: `apps/web/e2e/tests/chat/quick-chat.spec.ts` checks that Escape
  cancels a running Quick Chat turn and keeps its dialog open.

## Work orders

- [x] [Task 01: Cancel a running turn with Escape](task-01-chat-escape-cancel.md)

## Verification results

Focused unit, Go, TypeScript, translation, and specification checks passed.
Desktop and mobile Playwright scenarios passed with cancellation enabled; the
desktop disabled-setting scenario also passed.

## Risks

- Chat input editors or nested controls may already consume Escape; the handler
  must be scoped to the composer boundary and avoid overriding composition.
- Browser/mobile keyboard behavior can differ; verify actual Playwright key
  dispatch against the phone project.
