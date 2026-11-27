---
status: current
system: ui
requirements:
  - REQ-UI-REVERSE-SEARCH-FOCUS-001
---

# Reverse Search Focus System Design

## Purpose and boundaries

The shared chat composer owns focus handoff for its transient Ctrl+R history
overlay. Task chat and Quick Chat use the same `TipTapInput` and
`MessageHistorySearch` components. No backend, history model, or persisted draft
contract changes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-REVERSE-SEARCH-FOCUS-001` | [Escape focus handoff](#escape-focus-handoff), [Verification](#verification) |

## Escape focus handoff

`MessageHistorySearch` in
`apps/web/components/task/chat/message-history-search.tsx` owns the search
field's Escape key handler and its document-level fallback for an Escape from
elsewhere inside the Quick Chat dialog. Both paths must invoke one dedicated
Escape-dismiss callback after claiming the key. The normal close callback
continues to serve outside clicks and viewport changes.

`TipTapInput` in `apps/web/components/task/chat/tiptap-input.tsx` supplies the
Escape-dismiss callback. It closes this composer's reverse-search overlay and
focuses its live TipTap editor. The handler must use the editor instance owned
by this `TipTapInput`; global active-element lookup would risk focusing another
composer. TipTap's focus command restores editing without changing document
content or selecting a search hit. The existing history-selection path keeps
its own behavior, including inserting the selected history entry.

The Quick Chat overlay remains portaled inside its dialog focus scope. Its
existing clarification Escape guard and propagation handling continue to
consume this key before the dialog can dismiss. The document fallback retains
its current ownership check: on the task panel it claims only the overlay; in
Quick Chat it can also claim the containing dialog. An Escape outside that
scope cannot redirect focus into this composer.

## Mobile behavior

The phone uses the existing Quick Chat dialog and composer. The physical
keyboard shortcut and Escape path use the shared handler; touch composition,
overlay geometry, scroll ownership, and safe-area layout do not change. The
nearest shipped phone surface is the mobile Quick Chat composer in
`apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`. A phone with a
hardware keyboard gets the same focus return.

## Verification

Component tests in `message-history-search.test.ts` cover both Escape routes,
plus non-Escape close and unrelated-target behavior. A browser regression
asserts focus and immediate typing after Escape in task chat and Quick Chat;
the mobile Quick Chat browser path asserts the same outcome with a hardware
keyboard shortcut. The browser tests also check that the dialog stays open and
the draft is preserved.

## Implementation Plans

- [Restore chat focus after history Escape](../../../plans/reverse-search-focus/plan.md)
