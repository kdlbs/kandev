---
created: 2026-08-24
status: implemented
requirements:
  - REQ-UI-RESIZABLE-MARKDOWN-TABLES-001
system_design:
  - ../../specs/ui/system-design/resizable-markdown-tables.md
legacy_specs: []
---

# Implementation Plan: Markdown Table Resize Surface Parity

## Overview

Extend the shipped chat table-column resize behavior to rendered Markdown file
previews, then enable the editor-native equivalent in the live Plan view. The
file-preview slice comes first because it reuses Kandev's existing shared
renderer and establishes source-comment and scroll-owner compatibility before
the Plan-specific TipTap integration.

## Scope

### In scope

- Rendered Markdown file previews reuse accessible shared table separators on
  desktop fine-pointer layouts.
- File-preview source ranges, comments, links, and one local horizontal scroll
  owner survive the shared renderer composition.
- Editable Plan tables use TipTap column dragging with the same responsive gate,
  64-pixel minimum, internal-boundary scope, and ephemeral source-preserving
  state.
- Desktop and mobile Playwright evidence covers both target surfaces.

### Out of scope

- Persisting widths or encoding them in Markdown.
- Touch resizing or a mobile width editor.
- Read-only plan revision and plan comparison renderers.
- Table row resizing, column reorder/hide, and non-Markdown data grids.
- Refactoring unrelated Markdown consumers or Plan editing behavior.

## Technical approach

### Rendered Markdown file previews

Update `MarkdownPreviewTable` in
`apps/web/components/task/markdown-preview-content.tsx` to compose
`ResizableMarkdownTable` instead of replacing it with a plain table. Extend the
shared component's wrapper composition only as far as needed to keep
`SourceBlock` source data, comment classes, and click handling on the table
surface.

Keep exactly one `.markdown-table-scroll` / `overflow-x-auto` owner. Do not put
the shared resizer inside the preview's current scrolling table wrapper. Resize
buttons must remain interactive targets under
`isInteractiveSourceClickTarget`, so dragging does not open comment UI.

Preserve the existing `MarkdownPreviewRenderer` raw-HTML safety chain and all
non-table component mappings.

### Editable Plan view

In `apps/web/components/editors/tiptap/tiptap-plan-editor.tsx`, derive resize
capability from `useResponsiveBreakpoint()` and pass it into
`buildEditorExtensions`. Configure TipTap `Table` with
`cellMinWidth: MIN_MARKDOWN_COLUMN_WIDTH`, `lastColumnResizable: false`, and a
fine-pointer handle width.

Make resize capability a `useEditor` creation dependency. A phone, pointer-mode,
or viewport capability change recreates the editor from parent-owned draft
Markdown, removes the resize plugin and width-only ProseMirror attributes, and
does not lose typed source text.

Add scoped styles in `apps/web/app/globals.css` for
`.tiptap-plan-wrapper .tableWrapper`, `.column-resize-handle`, and
`.resize-cursor`. The Plan table wrapper owns horizontal scrolling. Do not add
dependency-global selectors or import ProseMirror's global table stylesheet.

## Tests

- `AC-UI-RESIZABLE-MARKDOWN-TABLES-001.1`, `.3`, `.7`, `.8`, `.9`, `.11`,
  and `.13`:
  extend
  `apps/web/components/task/markdown-preview-content.external-link.test.tsx`
  and retain the existing geometry/hook tests under
  `apps/web/components/shared/` and `apps/web/lib/markdown/`.
- `AC-UI-RESIZABLE-MARKDOWN-TABLES-001.10` and `.11`: use a rendered Plan table to
  prove in `apps/web/components/editors/tiptap/tiptap-plan-editor.test.tsx` and
  Playwright that responsive TipTap configuration changes geometry without
  changing serialized Markdown.
- `AC-UI-RESIZABLE-MARKDOWN-TABLES-001.12`: cover file-preview and Plan mobile paths
  in the configured Mobile Chrome project.

## E2E tests

- Extend `apps/web/e2e/tests/chat/markdown-preview.spec.ts` for the desktop Files
  preview path. Drag an internal boundary and assert adjacent rendered widths,
  table width, source attributes, and document containment.
- Extend `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts` for the phone Files
  preview path. Assert no separator, readable table content, local horizontal
  containment, and zero document overflow.
- Add `apps/web/e2e/tests/task/plan-table-resize.spec.ts` for desktop. Open the
  Plan panel, drag an internal table boundary, assert the 64-pixel clamp and
  changed geometry, then verify the backend plan Markdown is unchanged and a
  reload clears ephemeral widths.
- Add `apps/web/e2e/tests/task/mobile-plan-table-resize.spec.ts` for Mobile
  Chrome. Open Plan from the phone bottom navigation and assert no resize
  affordance, readable table content, and zero document overflow.

## Work orders

- [x] [Task 01: Extend resize controls to Markdown previews](task-01-extend-markdown-previews.md)
- [x] [Task 02: Enable resizing in editable Plan view](task-02-enable-plan-resizing.md)

## Verification results

Both work orders passed focused unit, typecheck, lint, desktop Playwright,
Mobile Chrome Playwright, and diff checks. Rendered Markdown file previews now
reuse the shared accessible resizer, while editable Plan tables use TipTap's
responsive resize plugin without persisting width state.

## Risks

- Preview source/comment markup currently owns the scroll wrapper. Naive nesting
  would create two horizontal scroll owners and misalign separators.
- TipTap's native plugin stores temporary `colwidth` attributes in its mounted
  ProseMirror document. Tests must prove the Markdown serializer and backend plan
  remain unchanged.
- Changing resize capability requires editor recreation. Parent draft state must
  be current so a viewport or pointer-mode transition cannot lose unsaved text.
- ProseMirror generates resize handles and wrapper class names. CSS and E2E
  selectors must stay narrowly scoped to the pinned TipTap table contract.
- Desktop and mobile tests run against production assets; every source change
  needs a rebuilt bundle through the managed E2E runner.
