---
status: current
system: ui
requirements:
  - REQ-UI-TOUCH-SELECTION-ACTIONS-001
---

# Touch Selection Actions System Design

## Purpose and boundaries

This design replaces selection-anchored comment triggers with one shared
selection action dock when the primary pointer is coarse. It covers four
surfaces:

| Surface | Selection owner | Current trigger |
| --- | --- | --- |
| Task Chat / Quick Chat agent replies | `MessageCommentSurface` (`components/task/chat/messages/`) | `SelectionCommentTrigger` in `message-comment-controls.tsx` |
| Office thread comment extracts | Same `MessageCommentSurface`, via `components/task/simple/comment-extract-surface.tsx` | Same |
| Markdown file preview | `useMarkdownPreviewComments` + `MarkdownPreviewCommentOverlays` | `markdown-preview-comment-button` |
| Code file editors | `use-codemirror-editor-state.ts`, `use-monaco-editor-state.ts` | `floating-comment-btn` with `floatingButtonPos` |

Desktop presentation, comment data, anchors, composers, Add/Run delivery,
and backend behavior do not change. No API, persistence, WebSocket, or plugin
contract changes. The Plan editor dock keeps its own formatting content; only
its bar geometry moves into the shared hook described below.

## Root cause

Each trigger is placed from the selection rectangle (`selection.rect`,
`currentSelection.position`, or `floatingButtonPos`). Android and iOS place
native selection actions from the same rectangle in a layer above the page, so
no placement, flip, or `z-index` can guarantee Kandev's button stays
uncovered. `responsive-plan-formatting.md` documents the same cause for the
Plan editor and resolved it with a docked strip.

## Requirement mapping

| Acceptance criterion | Design section |
| --- | --- |
| `.1` | [Presentation selection](#presentation-selection) |
| `.2` | [Presentation selection](#presentation-selection), [Surface integration](#surface-integration) |
| `.3`, `.4` | [Dock geometry](#dock-geometry) |
| `.5` | [Live selection tracking](#live-selection-tracking) |
| `.6` | [Surface integration](#surface-integration), [Portal and modal containment](#portal-and-modal-containment) |
| `.7` | [Interaction and accessibility](#interaction-and-accessibility) |
| `.8` | [Interaction and accessibility](#interaction-and-accessibility) |
| `.9` | [Surface integration](#surface-integration) |
| `.10` | [Toolbar slot](#toolbar-slot) |

## Presentation selection

Touch presentation uses the existing `useTouchDrawer()` predicate
(`!isFinePointer`) from `hooks/use-compact-task-chrome.ts`. Agent-message
comments already use it to choose the bottom Drawer, so the trigger and the
composer switch together and a fine-pointer narrow window keeps desktop
behavior. Each surface calls the predicate once and passes the result down;
fine-pointer branches stay byte-for-byte equivalent.

## Shared components

### `useDockedBarPosition`

Hook in `hooks/use-docked-bar-position.ts`, extracted from
`MobileFormattingToolbar` in `plan-bubble-menu.tsx`:

```ts
useDockedBarPosition({
  containerRef?: RefObject<HTMLElement | null>,
  barHeight: number,
  baseBottomOffset?: string,
}): DockedBarPosition // { top, left?, width? } or { bottom }
dockedBarStyle(position, origin?): CSSProperties
```

It combines `useVisualViewportOffset`, container bounds observed through
`ResizeObserver` plus window (capture) and `visualViewport` resize/scroll
listeners, and the pure `resolveDockedBarPosition`. `dockedBarStyle` converts
the position into fixed styles, or absolute styles relative to `origin` when
the bar renders inside a positioned portal container. `MobileFormattingToolbar`
is the first consumer with unchanged output, covered by the existing Plan
tests and mobile E2E.

### `SelectionActionDock`

New component in `components/shared/selection-action-dock.tsx`:

```tsx
<SelectionActionDock
  open={boolean}
  hostRef={RefObject<HTMLElement | null>}   // bottom edge to dock against
  portalContainer={HTMLElement | null}      // modal dialog when present
  label={string}                            // localized toolbar name
  testId={string}
>
  <SelectionActionDockButton icon={...} label={...} onActivate={...} testId={...} />
</SelectionActionDock>
```

- Renders a transparent, `pointer-events-none` 48 px positioner spanning the
  host's bottom edge (fixed, or absolute inside a portal container) using
  `useDockedBarPosition`, with the `role="toolbar"` group right-aligned inside
  it and `pointer-events-auto`. Only the icon buttons intercept taps; the rest
  of the row stays visible and interactive.
- `SelectionActionDockButton` is icon-only, matching the Plan dock's Comment
  action: a 44 px minimum hit area around a 32 px `bg-primary` square with the
  `IconMessage` glyph, `cursor-pointer`, and a localized `aria-label`/`title`
  ("Comment on selection").
- The toolbar carries `data-selection-action-dock`, which
  `useTouchSelectionTracking` uses to keep the captured selection while the
  dock is pressed.
- `useSelectionDockHost(rootRef)` resolves the nearest
  `[data-selection-dock-host]` ancestor into a ref after each render.
- Every button prevents default on `pointerdown` and `mousedown` so tapping it
  does not collapse the DOM selection before `onActivate` runs.

### Toolbar slot

`components/shared/selection-action-slot.tsx` provides
`SelectionActionSlotProvider`, `SelectionActionSlot`, and
`useSelectionActionSlot`. `TaskChatPanel` and `QuickChatContent` wrap their
transcript and composer in the provider; `ChatStatusBar` renders the slot as
the last child of its utility controls. The slot is a `display: contents` span
carrying `data-selection-action-dock`, so pressing a slotted action keeps the
tracked selection. When a slot is mounted, `MessageCommentControls` portals the
icon-only Comment button into it instead of rendering `SelectionActionDock`.
Without a provider (Office threads, Markdown preview, code editors, minimal
Quick Chat toolbars) the slot renders nothing and surfaces use the dock.

On phones the composer status row's workflow action no longer forces its own
row (see `REQ-UI-COMPOSER-ACTION-WRAP-001`), so the workflow action, status
icons, utilities, and the Comment action share one row when they fit.

## Dock geometry

`hostRef` resolution per surface:

- Chat and Office: the nearest ancestor marked `data-selection-dock-host`.
  `message-list-native.tsx` sets `SessionPanelContent`'s `selectionDockHost`
  prop, which marks the transcript scroller, so the dock sits at the
  scroller's visible bottom edge directly above the composer (Task Chat, Quick
  Chat, and run transcripts). `OfficeSimplePane` marks its main scroller.
- Markdown preview: the preview's scroll container (`scrollRef`).
- Code editors: the editor area below the toolbar (`editorAreaRef`).

With the keyboard closed the dock's top is `hostBottom - 48`. With the keyboard
open it is `min(hostBottom, visualViewportBottom) - 48`, matching the Plan dock.
When no host is found, `resolveVisualViewportPosition` places it at the visual
viewport bottom above `env(safe-area-inset-bottom)`.

## Live selection tracking

Touch selections can change without touch events reaching the page: native
handle drags fire only `selectionchange`. A shared hook
`useTouchSelectionTracking(rootRef, enabled, onChange)` in
`hooks/use-touch-selection-tracking.ts`:

- Subscribes to `document` `selectionchange` only while `enabled` (touch
  presentation and the surface accepts selections).
- Coalesces events into one `requestAnimationFrame` callback.
- Reports the current selection when both anchor and focus nodes are inside
  `rootRef`, and `null` when the selection is collapsed, whitespace-only, or
  outside the root.

Surfaces map that callback onto their existing resolver:
`getMessageSelection` for messages and `resolveMarkdownDomSelection` for the
preview. The editors observe their own selection models instead: CodeMirror's
selection update listener sets the current selection on touch
(`readCodeMirrorSelection` and `selectionEndPoint` in
`codemirror/codemirror-selection.ts`), and Monaco's existing
`onDidChangeCursorSelection` listener already tracks it.

In touch presentation the existing dismiss-on-scroll and dismiss-on-pointerdown
listeners in `useCommentTargetState` do not run; the selection itself controls
visibility. Fine-pointer dismissal is unchanged.

## Surface integration

### Agent-message and Office thread comments

- `MessageCommentSurface` passes `useDrawer` to `useCommentTargetState`. In
  touch presentation it enables `useTouchSelectionTracking` instead of the
  `onTouchEnd` capture and scroll/pointer dismissal.
- `MessageCommentControls` renders `SelectionActionDock` instead of
  `SelectionCommentTrigger` when `useDrawer && target && !composerOpen`.
  Activating Comment calls `setComposerOpen(true)` with the captured target and
  the existing `MessageCommentDrawer` opens unchanged.
- Eligibility stays `canSelectMessageText`; streaming, read-only, and
  sessionless messages never enable tracking.

### Markdown preview

- `useMarkdownPreviewComments` exposes the touch flag; in touch presentation
  it enables `useTouchSelectionTracking` alongside its existing touchend path.
- `MarkdownPreviewCommentOverlays` renders `SelectionActionDock` instead of the
  floating `markdown-preview-comment-button` in touch presentation. Activating
  calls the existing `openComposer`.

### Code file editors

- `components/editors/editor-selection-comment-trigger.tsx` renders either the
  existing floating `floating-comment-btn` (fine pointer) or the dock (touch),
  with the shared `code-editor-comment-button` test ID.
- CodeMirror shows it when `floatingButtonPos` is set; Monaco shows it for any
  current selection on touch and for `floatingButtonPos` otherwise. Activating
  calls the existing `handleFloatingButtonClick`, whose event argument is now
  optional.
- The Markdown preview reuses the same trigger with its
  `markdown-preview-comment-button` test ID.

## Portal and modal containment

The dock portals into the same container the surface already uses:
`portalContainer` for messages (`[role="dialog"]` in Quick Chat), the overlay
root for the Markdown preview, and `document.body` otherwise. Rendering inside
the dialog keeps it outside Radix's inert tree. Positions are converted into
container coordinates exactly as `SelectionCommentTrigger` does today.

## Interaction and accessibility

- Native selection actions are untouched; Kandev never calls
  `preventDefault` on selection or context-menu events.
- The toolbar label and the Comment button reuse localized copy. One new key,
  `task:selectionActions` ("Selection actions"), names the toolbar in all
  seven catalogs and the pseudo-locale.
- The dock never takes focus on appearance; screen-reader users reach it in
  document order after the transcript.

## Verification

- Unit: `use-docked-bar-position.test.ts` (closed and open keyboard, host and
  fallback); `use-touch-selection-tracking.test.ts` (inside, outside,
  collapsed, coalescing); `selection-action-dock.test.tsx` (role, label, 44 px
  target, selection preserved on pointerdown).
- Component: `message-comment-surface.test.tsx` for fine versus coarse pointer,
  handle adjustment, scroll survival, and collapse;
  `use-markdown-preview-comments.test.ts`,
  `markdown-preview-content.touch-dock.test.tsx`,
  `editor-selection-comment-trigger.test.tsx`,
  `codemirror-code-editor.touch-dock.test.tsx`, and
  `codemirror-selection.test.ts` for the other surfaces.
- Regression: existing Plan bubble menu tests stay green after the geometry
  extraction.
- E2E (`mobile-chrome`): chat and Markdown preview docks are at most 56 px
  wide and sit in their host's bottom-right corner, keep the DOM selection, and
  open the existing composer;
  the Quick Chat dock is interactive; Office extracts use the dock.
