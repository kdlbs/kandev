---
id: "01-shared-dock-and-chat-messages"
title: "Shared selection dock and docked chat comment trigger"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TOUCH-SELECTION-ACTIONS-001
acceptance_criteria:
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.1
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.2
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.3
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.4
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.5
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.6
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.7
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.8
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.9
system_design:
  - ../../specs/ui/system-design/touch-selection-actions.md
---

# Task 01: Shared Selection Dock and Docked Chat Comment Trigger

## Summary

Build the shared dock primitives and fix the reported bug: on coarse pointers,
agent-reply and Office thread comment selections show the Comment action in a
bottom dock instead of a floating button the native menu covers.

## In scope

- Extract `useDockedBarPosition` from `MobileFormattingToolbar`; keep Plan dock
  output identical.
- Add `useTouchSelectionTracking` and `SelectionActionDock` /
  `SelectionActionDockButton`.
- Mark the chat transcript scroller (and the Office thread comment scroller)
  with `data-selection-dock-host`.
- In `MessageCommentSurface` / `MessageCommentControls`, use the dock and
  selection tracking on touch; keep fine-pointer trigger, shortcut, and
  dismissal unchanged.
- Add `task:selectionActions` in en, pt-pt, zh-cn, zh-hk, zh-tw, ja, ko
  (`pnpm run i18n:zh-hant` for the Traditional pair).
- Amend `AC-UI-AGENT-MESSAGE-COMMENTS-001.3` to reference this requirement for
  the touch trigger.
- Update `e2e/tests/chat/mobile-agent-message-comments.spec.ts` and
  `e2e/tests/office/mobile-thread-extract-comments.spec.ts` to assert the
  docked trigger.

## Out of scope

- Markdown preview and code editors (task 02).
- Plan dock content or behavior changes.

## ASCII UI preview

See [plan.md UI-01 and UI-02](plan.md#ascii-ui-preview).

```text
|  and backfills existing rows.        |
|======================================|  dock: fixed, 48px, transcript bottom
|                        [ (C) Comment]|
|--------------------------------------|
| [ Message the agent...         ] (>) |
```

## Acceptance

- Failing tests first: a coarse-pointer `MessageCommentSurface` selection
  renders the dock (role `toolbar`) and no `agent-message-comment-trigger`
  floating wrapper; a handle-adjust `selectionchange` updates the target; a
  scroll event does not dismiss it. Fine-pointer tests remain unchanged.
- `mobile-chrome` E2E: after selecting agent text, the docked Comment button's
  box sits within 48 px of the transcript scroller's bottom edge, opens the
  Drawer, and Run delivers; the Quick Chat dock is interactive; the Office
  mobile extract flow still passes.
- Plan bubble menu tests pass unchanged after the geometry extraction.

## Verification

```bash
cd apps/web
pnpm exec vitest run \
  hooks/use-docked-bar-position.test.ts \
  hooks/use-touch-selection-tracking.test.ts \
  components/shared/selection-action-dock.test.tsx \
  components/task/chat/messages/message-comment-surface.test.tsx \
  components/editors/tiptap/plan-bubble-menu.test.tsx \
  hooks/use-visual-viewport-offset.test.ts
pnpm e2e:run --project mobile-chrome \
  tests/chat/mobile-agent-message-comments.spec.ts \
  tests/office/mobile-thread-extract-comments.spec.ts \
  tests/task/mobile-plan-formatting-toolbar.spec.ts
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
```

## Files likely touched

- `apps/web/hooks/use-docked-bar-position.ts` (+ test)
- `apps/web/hooks/use-touch-selection-tracking.ts` (+ test)
- `apps/web/components/shared/selection-action-dock.tsx` (+ test)
- `apps/web/components/editors/tiptap/plan-bubble-menu.tsx`
- `apps/web/components/task/chat/messages/message-comment-surface.tsx` (+ test)
- `apps/web/components/task/chat/messages/message-comment-controls.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/simple/comment-extract-surface.tsx` or its list host
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`
- `apps/web/e2e/tests/chat/mobile-agent-message-comments.spec.ts`
- `apps/web/e2e/tests/office/mobile-thread-extract-comments.spec.ts`
- `docs/specs/ui/requirements/agent-message-comments.md`

## Dependencies

None.

## Risks

- `message-comment-surface.tsx` is near the 400-line mark; put touch wiring in
  a helper hook to stay within function and complexity limits.
- jsdom does not emit `selectionchange` from range edits; dispatch it
  explicitly in tests.
- Office thread lists may not have a single scroll owner; fall back to the
  visual-viewport position rather than guessing a host.

## Parallelism

`sequential`

## Results

Tracked in kdlbs/kandev#5270. Added `useDockedBarPosition` / `dockedBarStyle`
(extracted from the Plan dock, whose output is unchanged),
`useTouchSelectionTracking`, and `SelectionActionDock` /
`SelectionActionDockButton` / `useSelectionDockHost`. On coarse pointers,
`MessageCommentControls` renders the dock (toolbar "Selection actions",
Comment button keeps the `agent-message-comment-trigger` test ID) instead of
the floating trigger. `MessageCommentSurface` follows `selectionchange` while
touch-selectable and stops dismissing on scroll/pointerdown in that mode.
`SessionPanelContent` gained a `selectionDockHost` prop used by the chat
transcript; `OfficeSimplePane` marks its scroller. `task:selectionActions` was
added to all seven catalogs and the pseudo-locale.

Verification:

- Failing-first: the new surface dock tests and the three new hook/component
  test files failed before implementation (missing modules / no toolbar).
- Focused Vitest (dock, hooks, message surface, Plan bubble menu, visual
  viewport) passed.
- `mobile-chrome` E2E: `mobile-agent-message-comments`,
  `mobile-thread-extract-comments`, and `mobile-plan-formatting-toolbar`
  passed (5 tests).
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check` passed.

Physical Android Chrome and iOS Safari checks were not available; Playwright
cannot render the native selection menu.
