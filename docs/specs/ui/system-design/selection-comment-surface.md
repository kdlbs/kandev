---
status: current
system: ui
requirements:
  - REQ-UI-SELECTION-COMMENT-SURFACE-001
---

# Selection Comment Surface System Design

## Purpose and boundaries

Three components currently author selection comments:

| Component | Users | Desktop | Coarse pointer |
| --- | --- | --- | --- |
| `components/task/plan-selection-popover.tsx` (`PlanSelectionPopover`) | Plan panel, `message-comment-controls.tsx` (chat and Office extracts), `question-review/region-comment-popover.tsx` | Portal popover, own `useDrag` and `usePopoverDismiss`, `placeFloatingRect` placement | Drawer |
| `components/task/editor-comment-popover.tsx` (`EditorCommentPopover`) | `codemirror-code-editor.tsx`, `markdown-preview-content.tsx` | Fixed popover, `use-draggable-popover.ts` drag and dismiss, line-range badge | None (same floating popover) |
| `components/task/inspector/preview-feedback-controls.tsx` (`DraftEditor`) | Web preview feedback | Inline section in the toolbar menu | Inside the feedback Drawer |

This design replaces all three with one shared component. It is a frontend-only
change: there are no API, persistence, WebSocket, or plugin contract changes.
Each hosting feature keeps its own callbacks and state.

## Requirement mapping

| Criterion | Section |
| --- | --- |
| `AC-UI-SELECTION-COMMENT-SURFACE-001.1` | [Migration](#migration) |
| `AC-UI-SELECTION-COMMENT-SURFACE-001.2` | [Desktop presentation](#desktop-presentation) |
| `AC-UI-SELECTION-COMMENT-SURFACE-001.3` | [Coarse-pointer presentation](#coarse-pointer-presentation) |
| `AC-UI-SELECTION-COMMENT-SURFACE-001.4` | [Component contract](#component-contract) |
| `AC-UI-SELECTION-COMMENT-SURFACE-001.5`, `.6` | [Component contract](#component-contract) |
| `AC-UI-SELECTION-COMMENT-SURFACE-001.7` | [Controlled draft](#controlled-draft) |
| `AC-UI-SELECTION-COMMENT-SURFACE-001.8` | [Migration](#migration) |

## Component contract

The component lives in `components/shared/selection-comment/` and is exported
as `SelectionCommentSurface`. It is the former `PlanSelectionPopover`,
generalized and split into files so each stays under the 600-line limit:
`selection-comment-surface.tsx` (the pointer switch and both variants),
`surface-parts.tsx` (composer, dismissal, drag, delete, actions, controlled
draft, and the context-label header), `surface-types.ts`, and
`line-range-label.ts` (the code and Markdown range label).

```ts
type SelectionCommentSurfaceProps = {
  selectedText: string;                 // default evidence and Drawer description
  position: { x: number; y: number };   // desktop anchor, viewport pixels
  onAdd: (comment: string, selectedText: string) => boolean | void | Promise<boolean | void>;
  onAddAndRun?: (comment: string, selectedText: string) => boolean | void | Promise<boolean | void>;
  onClose: () => void;
  editingComment?: string;
  onDelete?: () => boolean | void | Promise<boolean | void>;
  contextLabel?: string;                // e.g. "Lines 3-5" or a page route
  evidence?: React.ReactNode;           // replaces the quoted selectedText
  draftComment?: string;                // controlled draft (see below)
  onDraftChange?: (comment: string) => void;
  inputLabel?: string;                  // accessible name for the comment field
  portalContainer?: HTMLElement | null;
  errorMessage?: string | null;
  runDisabledReason?: string | null;
  testId?: string; inputTestId?: string; addButtonTestId?: string; runButtonTestId?: string;
};
```

The existing return-value contract stays: a callback that returns `false`
keeps the surface open with its text (`.5`). Edit mode hides Run and shows
Update plus optional Delete (`.6`). `evidence` is React content supplied by
the caller. The surface never uses `dangerouslySetInnerHTML`, and captured
page HTML stays escaped code inside the hosting feature's evidence node.

## Desktop presentation

Placement keeps `placeFloatingRect` and `floatingBounds` from
`floating-selection-position.ts`. It prefers below the anchor and flips above
when there is not enough room. Placement uses the rendered height, measured
with a `ResizeObserver` (180 px minimum), so tall evidence such as a
screenshot thumbnail still fits. The surface is capped at the viewport height
and scrolls internally. A window `resize` re-renders the surface, so
placement re-clamps to the smaller viewport, as `EditorCommentPopover` did.
Width stays at 340 px, capped at `calc(100vw - 2rem)`. When `contextLabel` is
set, it renders as a mono badge in the drag-handle row, as
`EditorCommentPopover` showed its line range. The root carries
`data-selection-comment-surface` so host Popovers and menus can ignore
interaction inside it.

The surface keeps the former `PlanSelectionPopover` drag and dismissal hooks.
Escape closes it even while the comment field has focus, and dismissal is
blocked while a mutation is pending. `use-draggable-popover.ts` stays as it
is for `CommentViewPopover`: its dismissal deliberately ignores Escape inside
editable fields, so merging the two would change the established Escape
behavior of either Plan/chat comments or the comment viewer.

## Coarse-pointer presentation

`useTouchDrawer()` selects the Drawer variant, as `PlanSelectionPopover`
already does. The Drawer header shows the title (Comment or Edit comment), the
`contextLabel` when present, and the evidence. Long evidence is clamped in a
description, and a thumbnail is capped at `max-h-40`. The body has one scroll
owner. Actions keep `min-h-11`.

## Controlled draft

When `draftComment` and `onDraftChange` are both supplied, the textarea is
controlled by the caller. Otherwise the surface keeps local state initialized
from `editingComment`, as it does today. Web preview feedback uses the
controlled form so its logical capture draft survives dismissal (`.7`). No
other caller has to change.

## Migration

1. Move `PlanSelectionPopover` to `SelectionCommentSurface` and update the
   three existing importers. Their test IDs and translation keys do not
   change.
2. Replace `EditorCommentPopover` in `codemirror-code-editor.tsx` and
   `MarkdownPreviewCommentOverlays` with `SelectionCommentSurface`. Pass the
   line range as `contextLabel`, `onSubmit` as `onAdd`, and `onSubmitAndRun`
   as `onAddAndRun`. Keep the `data-markdown-comment-popover` wrapper. With a
   coarse pointer, the docked Comment action now opens the Drawer (`.8`).
3. Delete `editor-comment-popover.tsx` and `plan-selection-popover.tsx`. Move
   their tests into the shared folder; the editor's viewport-shrink test
   becomes a surface test. Remove `task:modKeyEnterToAdd`,
   `task:shiftEnterToRun2`, `task:saveCommentForReviewShortcut`, and
   `task:saveAndSendToAgentShortcut` from all seven locales and pseudo.
4. The web-preview surface is migrated by the
   [web preview feedback design](../../tasks/system-design/web-preview-feedback-02.md#comment-surface).

## Failure and accessibility

Focus moves to the textarea on open. `inputLabel` gives the comment field an
accessible name when the surrounding copy does not. Errors use `role="alert"`. The Drawer keeps vaul's focus
trap. Disabled Run keeps the focusable tooltip wrapper.

## Verification

- Unit: the moved `plan-selection-popover` tests, adapted to the new props
  (`contextLabel`, `evidence`, controlled draft, `false` keeps open, edit hides
  Run, dismissal blocked while pending, re-clamping after the viewport
  shrinks).
- Component: `markdown-preview-content` and `codemirror-code-editor` tests
  show the line-range label and Add/Run with a fine pointer and the Drawer
  with a coarse pointer.
- E2E: `chromium` `markdown-preview`, `agent-message-comments`, and plan
  comment specs. `mobile-chrome` `mobile-file-viewer` (Drawer from the dock),
  `mobile-agent-message-comments`, and `mobile-question-image-review`.

## Implementation plans

- [Web preview feedback and comment surface](../../../plans/web-preview-feedback-comment-surface/plan.md)
