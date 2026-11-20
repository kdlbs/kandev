---
id: "01-chat-escape-cancel"
title: "Cancel a running turn with Escape"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-004
acceptance_criteria:
  - AC-TASKS-KEYBOARD-ACTIONS-004.1
  - AC-TASKS-KEYBOARD-ACTIONS-004.2
  - AC-TASKS-KEYBOARD-ACTIONS-004.3
  - AC-TASKS-KEYBOARD-ACTIONS-004.4
  - AC-TASKS-KEYBOARD-ACTIONS-004.5
system_design:
  - ../../specs/tasks/system-design/task-keyboard-actions.md
---

# Task 01: Cancel a running turn with Escape

## Summary

Add a composer-scoped Escape shortcut that invokes the existing cancel-turn
action when the account preference is enabled and cancellation is available.
Preserve Escape behavior in other chat surfaces. The shared composer also works
inside Quick Chat and in the phone chat layout with an attached keyboard.

## In scope

- Add a guarded key handler using the current `handleCancelTurn` callback.
- Reconcile account-setting drafts with live updates and send only edited fields.
- Keep Quick Chat open when Escape cancels a turn.
- Cover running, idle, focus, IME, and resolved-session behavior with unit tests.
- Add desktop and phone Playwright coverage for the chat flow.

## Out of scope

- New global shortcut binding, touch controls, or cancellation transport.
- Cancellation from the transcript or chat toolbar.

## Acceptance

- The persisted Escape cancellation setting defaults off and can be enabled in keyboard settings.
- When enabled, Escape from the focused composer cancels exactly one active turn using the
  resolved session; it does nothing when idle or composing.
- Escape outside the composer retains its existing behavior.
- Desktop and phone browser tests prove the user-visible path.

## ASCII UI preview

UI-01: Desktop task chat, running agent and focused composer.

```text
Chat transcript
Agent is working...
[ Message composer                 ]  <- focus here, press Escape to cancel
[ Send                             ]
```

UI-02: Phone task chat, attached keyboard.

```text
+------------------------------+
| Chat transcript              |
| Agent is working...          |
| [ Message composer         ] | <- focus here, press Escape
| [ Send                     ] |
+------------------------------+
```

Full context: [plan preview](plan.md#ascii-ui-preview). Both previews map to
AC-TASKS-KEYBOARD-ACTIONS-004.1 through .4; the phone retains its current chat
composition and introduces no touch control.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/settings/general-settings.test.tsx components/quick-chat/quick-chat-modal.test.tsx components/task/chat/chat-input-body.test.tsx components/task/chat/chat-escape-cancel.test.ts components/task/chat/chat-input-container.test.tsx components/task/chat/chat-input-area.test.tsx components/task/chat/tiptap-input-file-search-ownership.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/pause-resume-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-pause-resume-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat.spec.ts --grep "Escape cancels a running Quick Chat turn without closing the dialog")
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint on changed TS/TSX paths and the repo's i18n checks if any
user-facing copy is added. No new copy is expected.

## Files likely touched

- `apps/web/components/task/chat/chat-input-body.tsx`
- `apps/web/components/task/chat/chat-input-body.test.tsx`
- `apps/web/components/task/chat/chat-escape-cancel.ts`
- `apps/web/components/task/chat/chat-escape-cancel.test.ts`
- `apps/web/components/task/chat/tiptap-editor-area.tsx`
- `apps/web/components/task/chat/tiptap-editor-events.ts`
- `apps/web/components/task/chat/tiptap-input-surface.tsx`
- `apps/web/components/task/chat/tiptap-menu-keyboard.ts`
- `apps/web/components/task/chat/tiptap-message-history.ts`
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts`

## Dependencies

None.

## Risks

- The rich text editor may have its own Escape behavior; preserve it unless the
  running composer handler accepts the event.
- Phone browser keyboard automation must focus the composer before dispatch.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/task-keyboard-actions.md)
- [System design](../../specs/tasks/system-design/task-keyboard-actions.md)
- Existing `useChatPanelHandlers`, cancel-turn tests, and task chat E2E setup.

## Results

The setting defaults off and persists with user settings. Focused settings and
composer unit tests, Go user-settings and boot-payload tests, TypeScript,
translation checks, and specification validation passed. Focused desktop and
mobile Playwright scenarios passed with the setting enabled; a desktop scenario
also confirmed Escape leaves a running agent alone with the setting disabled.
