---
status: draft
system: ui
requirements:
  - REQ-UI-RESPONSIVE-PLAN-FORMATTING-001
---

# Responsive Plan Formatting System Design

## Purpose and boundaries

This design replaces the task Plan editor's selection-anchored formatting
surface on phone and touch-tablet layouts. Desktop keeps the current Tiptap
`BubbleMenu`. Mobile uses the same editor commands and state through a docked
presentation that does not share geometry with native text-selection chrome.

The task system continues to own plan data, revisions, autosave, and comments.
The plugin system's public `host.ui.RichTextEditor` props do not change. No
backend, API, persistence, or WebSocket boundary changes.

## Root cause and evidence

`PlanBubbleMenu` currently renders one Tiptap `BubbleMenu` for every non-empty
selection outside a code block and requests `placement: "top"` on every
viewport. Tiptap positions that DOM element from the selection rectangle.
Android and iOS independently position native selection actions around the
same rectangle, so Kandev cannot establish a reliable order between the two
surfaces through DOM placement, flipping, or `z-index`.

The existing `useVisualViewportOffset` hook and `MobileTerminalKeybar` already
demonstrate the required keyboard geometry. Their focused unit tests passed
during diagnosis, including top anchoring from `offsetTop + height` and
visual-viewport scroll updates.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-RESPONSIVE-PLAN-FORMATTING-001` | [Responsive presentation](#responsive-presentation), [Editor state and commands](#editor-state-and-commands), [Viewport and scroll contract](#viewport-and-scroll-contract), [Interaction and accessibility](#interaction-and-accessibility), [Verification](#verification) |

## Responsive presentation

`TaskPlanPanel` selects presentation with `useResponsiveBreakpoint`:

- `isMobile` or `isTablet`: docked formatting strip.
- compact or full desktop: existing selection bubble.

The responsive choice is transient presentation state. It does not recreate
the editor, overwrite a preference, or change Markdown serialization. The
desktop and docked variants share the formatting controls and command handlers;
only their container, visibility rule, sizing, and position differ.

The mobile entry point is a non-whitespace text selection in the existing Plan
tab. The strip is a one-dimensional editor accessory while an eligible
selection is focused. A caret or whitespace-only selection leaves the strip
hidden to preserve editing space. This fits a frequent set of shallow actions
better than a drawer. Formatting actions come first, followed by link and the
selection-only Comment action. The inline link input temporarily replaces the
actions in the same docked surface and remains mounted while it owns focus.

The nearest shipped mobile exemplar is `MobileTerminalKeybar`. It contributes
visual-viewport positioning, keyboard focus retention, fixed-height horizontal
controls, and mobile task-navigation clearance. It does not define the Plan
toolbar's command set or touch sizing.

## Components and responsibilities

- `TaskPlanPanel` supplies the responsive presentation and the mobile task
  navigation offset without changing plan state or autosave behavior.
- `TipTapPlanEditor` forwards the selected presentation to the formatting
  surface and reserves editor scroll space while the docked strip is active.
- `PlanBubbleMenu` retains the current desktop BubbleMenu behavior and owns the
  docked variant, shared controls, link flow, and comment selection handoff.
- `useVisualViewportOffset` continues to publish keyboard-open state and the
  visual viewport's bottom edge. A pure shared positioning helper accepts bar
  height and the host's closed-keyboard bottom offset.
- `MobileTerminalKeybar` adopts the shared positioning helper without changing
  its rendered geometry or shell-input behavior.

The mobile task layout remains the single source for its bottom-navigation
height. It passes that value down as an internal presentation input rather than
introducing a second literal or a persisted setting.

## Editor state and commands

The formatting surface subscribes to Tiptap transactions for the current
selection, code-block context, link state, and active inline marks. It also
subscribes to editor focus and blur events because Tiptap's React editor-state
selector updates on transactions, not focus changes alone.

The docked strip is visible when the Plan editor is focused and its current
selection contains non-whitespace text, or while its link input owns focus. It
is hidden for a caret, a whitespace-only selection, and a code block. Bold,
italic, underline, strikethrough, inline code, highlight, and link reuse the
current editor command chains. Comment remains available only when the Plan
panel supplied its comment callback and the current selection contains
non-whitespace text.

The desktop BubbleMenu retains its non-empty-selection visibility condition.
It consumes the same reactive active-mark snapshot as the docked strip so
pressed styling follows selection and command transactions in both variants.

## Render and transaction stability

The desktop BubbleMenu receives stable option and callback identities when
their values do not change. Tiptap dispatches a metadata transaction when its
menu options change. A new options object on each render can create a feedback
loop between that transaction and the editor snapshot subscription.

The editor snapshot subscription compares each derived snapshot with the
current snapshot. It publishes state only when focus, selection, code-block
context, or active marks change. Metadata-only transactions do not cause a
render. Selection and formatting transactions still publish the changed
snapshot.

These two controls keep the desktop selection bubble reactive without making
Tiptap option updates part of the React render cycle. The mobile presentation
uses the same deduplicated editor snapshot.

## Viewport and scroll contract

The docked strip has a stable measured height. Its position resolves as follows:

1. When the keyboard is open, use fixed positioning with `top` equal to the
   visual viewport's bottom edge minus the strip height. This follows the
   existing iOS-safe terminal keybar behavior.
2. When the keyboard is closed, use fixed bottom positioning with the mobile
   task-navigation offset plus `env(safe-area-inset-bottom, 0px)`.
3. Without Visual Viewport API support, treat the keyboard as closed and use
   the safe bottom fallback.

The Plan editor remains the only vertical scroll owner. While the strip is
visible, the editable region reserves at least the strip height at its bottom.
The strip itself can scroll horizontally and uses overscroll containment. It
must not widen the document or create a second vertical scroller.

## Interaction and accessibility

Docked icon buttons use semantic `button` elements with localized accessible
names and `aria-pressed` for toggled marks. Each button has a minimum 44-pixel
touch dimension even though the compact visual action surface is 32 pixels and
the dock is 48 pixels tall.

Formatting buttons prevent default on both `pointerdown` and `mousedown` so a
touch does not move focus or collapse the selection before the command runs.
The click handler applies one command and explicitly returns focus to the
editor. The link button is the exception only after it intentionally focuses
the inline URL input; Apply and Escape return to the editor consistently.

Kandev does not set `user-select: none`, disable the WebKit touch callout, or
intercept native clipboard actions on the editable content.

## Failure and recovery

- A missing or destroyed editor keeps both presentations unmounted.
- A focus transition outside the editor and link input hides the docked strip;
  refocusing restores it from current editor state.
- A caret or whitespace-only selection hides the dock without affecting the
  editor's native keyboard and selection behavior.
- If the visual viewport changes repeatedly while the keyboard animates, the
  latest hook snapshot determines position; no timer, polling loop, or global
  store is introduced.
- A browser that does not expose Visual Viewport API receives the closed-
  keyboard fallback. Native selection actions remain available in all cases.

## Persistence and security

All responsive, focus, selection, and toolbar state is transient browser state.
No data migration or new trusted input exists. Link values continue through
the existing Tiptap link command and current rendering protections.

## Verification

Component tests cover the responsive branch, reactive mark/selection state,
selection-driven visibility, link mode, compact sizing, selection-preserving
pointer behavior, and the shared visual-viewport positioning helper. Existing
terminal keybar tests guard the extracted geometry against regression.

A `mobile-chrome` Playwright scenario opens a seeded task plan, focuses the real
editor, selects text, taps Bold, and verifies the serialized/rendered result.
It also drives visual-viewport resize and scroll events using the established
terminal-keybar test pattern, then asserts keyboard-edge tracking, task-nav
clearance, 44-pixel controls, internal horizontal overflow, and no document-
level horizontal overflow.

Desktop browser emulation cannot render Android or iOS native selection chrome.
Final device acceptance therefore checks Android Chrome and iOS Safari to
confirm that native Cut, Copy, and Paste remain usable while Kandev's strip
stays at the keyboard edge. Automated tests prove the Kandev geometry,
selection-driven visibility, compact sizing, and formatting outcome rather than
claiming to inspect operating-system UI.

No production telemetry is added; deterministic component and browser evidence
is sufficient for this local presentation correction.

## Related decisions

None. This corrects an existing responsive UI surface and reuses an established
visual-viewport pattern without creating a new architecture boundary.
