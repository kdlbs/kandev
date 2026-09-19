---
status: current
system: ui
requirements:
  - REQ-UI-RESIZABLE-MARKDOWN-TABLES-001
---

# Resizable Markdown Tables System Design

## Purpose and boundaries

This design extends the existing chat table-resize behavior to rendered
Markdown file previews and the editable Plan view. It keeps resize state inside
the mounted renderer and introduces no API, persistence, Markdown syntax, or
permission change.

Rendered Markdown and Plan tables use different document engines. ReactMarkdown
surfaces reuse Kandev's existing accessible adjacent-column overlay. The Plan
editor uses TipTap's table-resize integration because ProseMirror owns its table
DOM and editing transactions. Both engines share capability gating, the
64-pixel minimum, ephemeral state, and table-local overflow policy.

## Requirement mapping

| Requirement                            | Design section                                                                                                                                                                                       |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-RESIZABLE-MARKDOWN-TABLES-001` | [Surface contracts](#surface-contracts), [Rendered Markdown flow](#rendered-markdown-flow), [Plan editor flow](#plan-editor-flow), [Responsive and mobile contract](#responsive-and-mobile-contract) |

## Surface contracts

| Surface                        | Renderer                                                  | Desktop fine-pointer behavior                                             | Phone or coarse-pointer behavior                        |
| ------------------------------ | --------------------------------------------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------- |
| Chat Markdown                  | ReactMarkdown plus `ResizableMarkdownTable`               | Existing accessible adjacent-column separators                            | Existing wrapping and local scrolling; no separators    |
| Rendered Markdown file preview | `MarkdownPreviewRenderer` plus source-aware table wrapper | Same shared separators while retaining preview comments and source ranges | Existing mobile file-viewer preview; no separators      |
| Editable Plan view             | TipTap `Table` extension                                  | Native internal-boundary drag with a 64-pixel minimum                     | Resizing disabled; existing Plan panel remains readable |

Read-only plan revision and comparison renderers remain unchanged. Other
ReactMarkdown consumers that already use `markdownComponents` continue to
inherit the shared renderer contract.

## Components and responsibilities

- `apps/web/components/shared/resizable-markdown-table.tsx` owns semantic table
  markup, separator rendering, responsive capability checks, and the single
  table-local scroll root for ReactMarkdown surfaces.
- `apps/web/components/shared/use-markdown-table-resize.ts` owns geometry,
  pointer capture, keyboard resizing, reset behavior, streamed-content
  remeasurement, and cleanup for rendered Markdown tables.
- `apps/web/components/task/markdown-preview-content.tsx` attaches source-line
  and comment behavior to the shared resizable table without introducing a
  nested horizontal scroll owner.
- `apps/web/components/editors/tiptap/tiptap-plan-editor.tsx` configures TipTap's
  editable table extension for eligible layouts. TipTap remains the authority
  for ProseMirror transactions and editor DOM.
- `apps/web/hooks/use-responsive-breakpoint.ts` supplies the phone and pointer
  capability used by both renderer paths.
- `apps/web/app/globals.css` owns narrowly scoped preview and Plan table resize
  geometry, cursor feedback, and overflow containment.

## Rendered Markdown flow

`markdownComponents.table` continues to resolve to `ResizableMarkdownTable`.
The file-preview component currently replaces that mapping with a plain
source-aware wrapper. The replacement will instead compose source-range and
comment metadata onto the shared resizable table root.

The composition must retain exactly one `overflow-x-auto` owner around each
table. The same element, or a non-scrolling parent plus the shared scroll root,
retains `data-md-source-start`, `data-md-source-end`, comment-state classes, and
click handling. Resize buttons remain interactive targets, so preview comment
click delegation does not open a comment while a user resizes.

The existing resize hook keeps its current contract:

1. Measure the browser's automatic first-row geometry.
2. Activate a fixed `<colgroup>` only after a non-zero adjustment.
3. Resize the adjacent pair through `resizeAdjacentColumns`, preserving pair and
   table totals and enforcing `MIN_MARKDOWN_COLUMN_WIDTH`.
4. Remeasure after controlled width changes, wrapper changes, and streamed DOM
   mutations.
5. Reset on Enter, double-click, column-count change, capability loss, or
   unmount; restore the drag-start snapshot on pointer cancellation.

## Plan editor flow

The live Plan panel already uses TipTap's `Table`, `TableRow`, `TableCell`, and
`TableHeader` extensions. Its `Table` configuration changes from
`resizable: false` to capability-aware resizing with:

- `cellMinWidth` set to `MIN_MARKDOWN_COLUMN_WIDTH`;
- `lastColumnResizable: false`, so only internal boundaries act as resize
  boundaries;
- a handle width large enough for reliable fine-pointer targeting without
  covering normal cell interaction.

`TipTapPlanEditor` reads `useResponsiveBreakpoint()` and enables resizing only
when `isFinePointer && !isMobile`. The resize capability is a `useEditor`
creation dependency. Crossing that capability boundary recreates the editor
from the parent-owned `draftContent`, which removes ProseMirror resize plugins
and ephemeral `colwidth` attributes without losing typed Markdown.

During a drag, ProseMirror updates cell `colwidth` attributes and TipTap's table
node view updates its `<colgroup>`. The Markdown serializer does not encode
those attributes. Therefore a resize can reflow the mounted editor but cannot
change the plan source or schedule a distinct width-only backend value.

The Plan table wrapper is the local horizontal scroll owner. Scoped CSS styles
TipTap's `tableWrapper`, `column-resize-handle`, and `resize-cursor` classes
inside `.tiptap-plan-wrapper`; dependency-global selectors are not added.

## Responsive and mobile contract

- **Desktop outcome and entry point:** File-preview and Plan tables expose inline
  boundary targets where the table already renders. No menu or extra step is
  introduced.
- **Mobile entry points:** The existing mobile Files panel opens
  `MobileFileViewerPanel` and its Markdown preview. The existing Plan bottom-nav
  action opens `TaskPlanPanel` as the focused panel.
- **Nearest shipped exemplars:** Chat tables in
  `mobile-markdown-wrap.spec.ts` provide the wrapping and table-local scrolling
  contract. `MobileFileViewerPanel` provides the focused file-preview surface,
  and `session-mobile-layout.tsx` provides the focused Plan surface.
- **Hierarchy and primary action:** Reading the file or editing the plan remains
  primary. Optional precision resizing does not add controls that obscure
  narrow cells.
- **Presentation:** Existing full-height file and Plan surfaces remain. No new
  drawer, route, toolbar, or navigation branch is needed.
- **Scroll ownership:** Each surface keeps its current vertical owner. A wide
  table owns only its local horizontal scroll; page and document horizontal
  overflow stay zero.
- **Shared versus specialized behavior:** Markdown content, editor state,
  minimum width, and responsive capability are shared. ReactMarkdown and TipTap
  retain engine-specific resize implementations.

Mobile Playwright coverage opens both target surfaces, confirms table content is
readable, confirms no resize affordance appears, and checks document overflow.

## Failure and recovery

- Missing or invalid rendered-table geometry leaves automatic layout active and
  exposes no separator.
- Preview source-line metadata remains authoritative even when a table cannot be
  resized.
- Pointer cancellation or capability loss clears global cursor and selection
  overrides in the shared renderer.
- TipTap owns mouse listener teardown when the editor or resize plugin is
  destroyed. Recreating the editor on capability loss restores current parent
  Markdown and discards width-only attributes.
- A table narrower than its content scrolls inside its local wrapper instead of
  widening the file preview, Plan panel, or document.

## Persistence and compatibility

No schema, API, local-storage, session-storage, or Markdown-format change is
introduced. Existing stored chat messages, files, plans, and revisions remain
compatible. Widths exist only in mounted React state or the mounted ProseMirror
document and disappear on remount or reload.

## Security

The change does not broaden Markdown HTML handling. File previews retain the
existing `rehype-raw` followed immediately by `rehype-sanitize` boundary.
Resize interactions do not add HTML input, URL handling, or backend calls.

## Verification strategy

- Focused Vitest coverage keeps rendered Markdown geometry, responsive reset,
  source-wrapper composition, and Markdown serialization behavior explicit.
- Desktop Playwright resizes a table in the Files preview and a table in the
  Plan panel, checking column geometry, source preservation, and local overflow.
- Mobile Chrome opens both surfaces, verifies no resize affordance, and checks
  readable table content plus zero document overflow.

## Related decisions

No new architecture decision is required. The design extends the existing
renderer and editor boundaries without changing ownership or persistence.
