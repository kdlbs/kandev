---
created: 2026-11-05
status: draft
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

- Handle Escape from the focused task chat composer only when its agent is
  running.
- Reuse `handleCancelTurn` and the current resolved session.
- Prove the behavior and guards with unit and desktop/phone browser coverage.

### Out of scope

- A new keyboard-shortcut preference or global shortcut.
- Cancellation from transcript focus or other chat controls.
- A touch button, backend/API change, or new cancellation transport.

## Technical approach

Use `useChatPanelHandlers` in `apps/web/components/task/chat/chat-input-area.tsx`
and its existing `handleCancelTurn`. Scope the Escape keydown to the actual
composer, consume it only when the user has enabled the setting, cancellation is available, and the event is not an IME
composition event, and preserve the existing cancel request and error handling.
The rendered chat's running state and resolved session remain authoritative.

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
for editor menus and suggestions. Existing chat-input tests cover surrounding
composer behavior.

## E2E tests

- Desktop Chromium: `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`
  focuses the task chat composer, presses Escape during a running turn, and
  observes cancellation; it also confirms Escape outside the composer does not
  cancel.
- Phone Chromium: `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts`
  uses the existing mobile chat layout with an attached keyboard, focuses the
  composer, and presses Escape during a running turn.

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
