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

The layout owns the conversation flex boundary and composer popup containment.
It does not change state, APIs, persistence, or shared dialog defaults.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001` | [Components and responsibilities](#components-and-responsibilities), [Height and scroll contract](#height-and-scroll-contract), [Responsive behavior](#responsive-behavior), [Verification](#verification), [Composer model popup containment](#composer-model-popup-containment) |

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

The transcript retains its scroll owner on all viewports. Composer popups own
their separate option-list scrolling. The
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

## Composer model popup containment

For AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.7 and .8, the shared
`ModelConfigSelector` resolves its trigger's nearest `[data-slot="dialog-content"]`
when opening. Pass that element to the existing `PopoverContent.portalContainer`.
Outside a dialog, retain the default body portal. Resolve from the actual trigger
on each opening so a closed or replaced dialog cannot leave a stale container.

`ModelConfigSelectorTrigger` exposes its button reference to the selector.
`ModelConfigSelectorContent` retains `CommandList` as the model scroll owner and
its existing search and selection handlers. The popup remains portaled, avoiding
clipping by composer ancestors, but is a DOM descendant of the dialog's permitted
scroll and focus region. Do not disable modal scroll locking or cancel wheel
handlers globally. Provider-option subviews inherit the same portal containment.

Desktop retains the anchored picker. Phone retains the existing touch-capable
picker inside the full-height Quick Chat dialog, as exercised by
`mobile-model-selector.spec.ts` on the task surface. This is a brief searchable
choice with existing coarse-pointer targets, so a new drawer is unnecessary.
Entry is the composer model button; hierarchy is search, model list, then
provider options. The list scrolls; the composer and background remain stable.
Selecting a model is the primary action. Preserve viewport containment and
existing safe-area clearance, keyboard focus, and dismissal behavior.

Behavioral browser tests must send real wheel/touch input to an overflowing list,
assert increasing scrollTop, and select a newly revealed supported model.
Programmatically setting scrollTop does not prove scroll-lock compatibility.
Cover modal Quick Chat and the non-modal task selector; verify search focus,
Escape focus return, phone containment, and no document horizontal overflow.
Desktop wheel, mobile touch, selection, search, Escape focus return, and
non-modal fallback are covered by the focused model-scroll E2E scenarios.
