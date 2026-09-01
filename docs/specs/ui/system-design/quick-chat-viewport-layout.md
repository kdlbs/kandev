---
status: current
system: ui
requirements:
  - REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001
---

# Quick Chat viewport layout System Design

## Purpose and boundaries

The UI system owns the height and scroll contract for the Quick Chat dialog.
The task system supplies conversation data but does not own this layout.

The correction changes one flex boundary in the conversation view. It does not
change state, APIs, persistence, or the shared dialog primitive.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001` | [Components and responsibilities](#components-and-responsibilities), [Height and scroll contract](#height-and-scroll-contract), [Responsive behavior](#responsive-behavior), [Verification](#verification) |

## Components and responsibilities

- [`QuickChatModal`](../../../../apps/web/components/quick-chat/quick-chat-modal.tsx)
  owns the viewport-bound dialog. It uses `85vh` on wider viewports and
  `100dvh` on phone viewports.
- [`QuickChatSessionView`](../../../../apps/web/components/quick-chat/quick-chat-session-view.tsx)
  owns the recovery notice and the remaining conversation slot.
- [`QuickChatContent`](../../../../apps/web/components/quick-chat/quick-chat-content.tsx)
  owns the transcript, clarification panel, and chat composer column.
- [`MessageList`](../../../../apps/web/components/task/chat/message-list-native.tsx)
  uses `SessionPanelContent` as the transcript scroll owner.

## Height and scroll contract

`QuickChatModal` is a flex column with a bounded height. The tab strip uses its
intrinsic height. The active session view uses the remaining height.

The conversation slot in `QuickChatSessionView` must also be a flex container.
This rule gives `QuickChatContent` a definite height for its `flex-1` behavior.

`QuickChatContent` keeps the transcript as `min-h-0 flex-1`. The clarification
panel and chat composer keep their intrinsic heights. `SessionPanelContent`
keeps `overflow-y-auto` and remains the only transcript scroll owner.

Without the flex boundary, short content uses its intrinsic height and leaves
unused space below the composer. Long content expands beyond the dialog and
moves the composer below the viewport.

## Responsive behavior

The desktop outcome keeps the composer at the bottom of the centered dialog.
The existing Home and session-sheet actions remain the mobile entry points.

The nearest mobile exemplar is
[`mobile-quick-chat-entry.spec.ts`](../../../../apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts).
The phone surface remains a full-height dialog. This correction does not add a
new mobile composition.

The transcript remains the single vertical scroll owner on all viewports. The
dialog keeps its existing dynamic viewport units and safe-area padding. The
composer keeps the existing shared state, toolbar, input behavior, and actions.

## Failure and recovery

This layout has no runtime error state. If the height chain is incomplete, the
browser uses content height and produces the two incorrect layouts.

The correction restores the complete flex chain. A viewport resize then causes
normal browser layout without a reload or state change.

## State, security, and accessibility

The correction adds no state, persistence, API, or security boundary. The
existing Radix focus trap, Escape behavior, close controls, and focus return do
not change.

## Verification

- A desktop Playwright scenario uses a laptop-height viewport. It checks the
  composer position before and after a bulk transcript fills the message area.
- The desktop scenario shrinks the viewport and rechecks composer containment.
- The desktop scenario checks that only the transcript gains vertical overflow.
- A mobile Playwright scenario checks composer containment and transcript
  overflow in the existing full-height surface.
- The focused web type check covers the React and TypeScript integration.

## Related decisions

None. This correction completes an existing local flex layout.
