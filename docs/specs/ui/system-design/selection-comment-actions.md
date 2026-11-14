---
status: current
system: ui
requirements:
  - REQ-UI-SELECTION-COMMENT-ACTIONS-001
---

# Selection comment actions design

## Purpose and boundaries

The UI system owns the common presentation of selection Comment actions.
Selection capture and comment editors retain their feature-specific handlers.
This design introduces no global comment controller or new persistence owner.

[Agent-message selection](agent-message-selection.md) retains message eligibility
and mouse capture. [Responsive Plan formatting](responsive-plan-formatting.md)
provides the dock and visual-viewport precedent. Plan, diff gutters, whole-file
comments, and Browser annotations do not migrate in this package.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SELECTION-COMMENT-ACTIONS-001` | [Presentation and ownership](#presentation-and-ownership), [Geometry](#geometry), [Selection lifecycle](#selection-lifecycle), [Activation](#activation), [Verification](#verification) |

## Presentation and ownership

Use `useResponsiveBreakpoint` and choose the dock when `isMobile || !isFinePointer`.
The width condition also covers narrow fine-pointer windows. The pointer
condition covers touch devices wider than the tablet layout. Wider fine-pointer
surfaces keep their existing floating actions.

Add an internal `SelectionCommentHostProvider` and `SelectionCommentDock` in
`apps/web/components/shared/selection-comment-dock.tsx`. The provider supplies
owner geometry and the permitted portal container. It does not own quotes,
selection state, or comment mutations. An optional lower boundary identifies a
composer inside an otherwise shared scrolling viewport.

Only an active, eligible dock mounts browser geometry listeners. The provider
itself does not subscribe for every message. Each feature keeps its existing
desktop trigger and renders the shared dock for touch presentation.

| Consumer | Owner viewport and boundary | Existing activation target |
| --- | --- | --- |
| Task Chat and Quick Chat | `NativeMessageList`'s `SessionPanelContent`, using `scrollRef`; its allocated bottom already clears the composer | `MessageCommentControls` opens the existing drawer or popover |
| Office thread extracts | `TaskChat` receives `scrollParent`; the local `ChatInput` wrapper supplies the lower boundary when visible | `CommentExtractSurface` reuses `MessageCommentSurface`; no Run action is added |
| Markdown file preview | `MarkdownPreviewContent`'s existing `scrollRef` | `useMarkdownPreviewComments.openComposer` |
| CodeMirror file editor | `CodeMirrorCodeEditor`'s existing `editorAreaRef` | `handleFloatingButtonClick` opens `EditorCommentPopover` |
| Monaco file editor | `MonacoEditorView`'s existing `editorAreaRef` | `handleFloatingButtonClick` opens the inline comment view zone |

The nearest shipped exemplar is `MobileFormattingToolbar` in `plan-bubble-menu.tsx`.
Reuse its owner-bounded dock concept and `useVisualViewportOffset`. Do not move
Plan's formatting state into the new component or change the public plugin contract.

The phone entry point remains native selection in the current content surface.
A short toolbar exposes the single Comment action. It needs no intermediate
menu. The current comment editor opens after activation. The existing content
region remains the only vertical scroll owner.

## Geometry

Add a pure `resolveSelectionCommentDockPosition` helper in
`apps/web/components/shared/selection-comment-dock-position.ts`.
Its inputs describe the owner rectangle, optional composer boundary, visible
viewport, dock height, safe-area clearance, and portal coordinate origin.
It receives no selection rectangle or mouse coordinates.

Intersect the owner rectangle with the visible viewport. Use the smaller of
the owner bottom, visible viewport bottom, and supplied composer top as the
lower edge. Place the dock immediately inside that edge. This keeps chat above
its composer and puts a surface that extends behind the keyboard above the keyboard.

Use the current `offsetTop + height` geometry from `useVisualViewportOffset`
for keyboard movement. Account for horizontal visual-viewport offsets when
the dock intersects a zoomed or panned viewport. Apply device safe-area
clearance only where the owner reaches that edge. A composer or navigation
region that already clears the edge needs no additional inset.

The action row follows the Plan strip's compact 48px presentation, with a
labelled action at least 44px high and wide. Any required safe-area padding
sits outside that row. Hide the dock when its owner has no visible room for
the complete action. A detached owner cannot fall back to the whole window.

Observe the active owner and composer with `ResizeObserver`. Refresh geometry
on ancestor scroll, window resize, and visual-viewport resize and scroll.
Remove listeners and observers when the dock closes or its owner changes.
Geometry updates do not clear a valid touch selection.

Portal content must stay inside the enclosing modal's interactive scope.
Quick Chat already passes its nearest dialog as `portalContainer`. Preserve
that boundary and convert viewport coordinates once into the actual portal
containing block, including its origin and scroll offset. File surfaces also
resolve the dock's portal from their owner's enclosing modal. Without a modal,
they can use their existing overlay root. A higher `z-index` does not replace
modal ownership or solve native-menu overlap.

Reserve the visible dock's occupied height in the existing scroll region.
Keep existing padding and scroll-anchor offsets additive. Remove only the
dock's reservation on closure. Do not resize the editor or replace its model.

## Selection lifecycle

`MessageCommentSurface` currently captures local `touchend` and owned document
mouse releases. Keep `useMessageCommentMouseSelection`'s start-inside rule.
Add scoped touch selection tracking for delayed native `selectionchange` events
and handle adjustments. Track the owner interaction so an unrelated click
cannot revive a dismissed quote. Validate both selection boundaries through
`getMessageSelection`. Suspend capture while the comment editor is open.

`useMarkdownPreviewComments` already validates source lines after `touchend`.
Extend its touch path to track native range changes with the same ownership
and ignored-target rules. On touch, viewport changes reposition the dock
instead of invoking the current unconditional resize dismissal. Keep desktop
dismissal behavior where it does not conflict with the touch contract.

CodeMirror currently derives its trigger from mouse movement and `mouseup`.
Its touch path must derive a non-whitespace quote and lines from the editor's
selection updates and focus state. Monaco already records `currentSelection`
from `onDidChangeCursorSelection`, but gates the trigger on `mouseup`.
Use that validated selection for its touch dock without requiring mouse coordinates.
Preserve fine-pointer positioning and existing shortcuts in both editors.

Only the active owning surface can expose the action. A collapsed or invalid
range clears the pending trigger. Outside dismissal, owner removal, and
identity changes clear stale state and dispose touch subscriptions. An open
editor retains its captured quote and feedback through its existing lifecycle.

## Activation

Reuse existing localized Comment labels and accessible names. The dock action
prevents default focus movement on `pointerdown` and `mousedown`. It snapshots
the validated quote before the current editor takes focus. Native selection
behavior on the content itself remains untouched.

Keep existing message IDs, selected text, repository identity, and source line
ranges. Chat retains Add/Run and its existing drawer behavior. Office retains
its no-Run extract flow. File comments retain their current popover or inline
editor, persistence, and delivery paths. No comment type or payload changes.

## Failure, persistence, and security

A missing owner, invalid selection, or insufficient visible space hides the
action. It creates no comment and does not change file content or chat drafts.
Without Visual Viewport API, use the current owner and window bounds.
Cleanup restores the previous content allocation and preserves existing focus rules.

All new state is transient browser presentation state. Existing authorization,
comment persistence, and text sanitization remain unchanged. No metrics, polling,
network requests, or new public interfaces are required.

## Verification

Pure geometry tests cover owner bounds, composer clearance, keyboard resize
and scroll, nonzero portal origins, and missing viewport support. Component
tests cover selection-independent positioning, pointer modes, touch-only
capture, changing ranges, selection retention, dismissal, and listener cleanup.

Mobile browser scenarios cover Task Chat, Quick Chat's modal, Office extracts,
Markdown preview, CodeMirror, and Monaco. Assert dock geometry, hit targets,
viewport containment, final-content reachability, and actual comment creation.
Include a wide coarse-pointer context and a narrow fine-pointer context.
Desktop tests retain each floating action and its existing comment flow.

Playwright cannot inspect the operating system's native selection menu.
Automated selection and visual-viewport stimuli prove Kandev's geometry and
event handling only. Record Android Chrome and iOS Safari device results
separately when available. Do not describe emulation as physical-device proof.

## Related decisions and delivery

No new ADR is required. This repair extends the existing UI dock precedent
without changing an authority, persistence, or public contract boundary.

- [Implementation plan](../../../plans/touch-selection-comments/plan.md).
