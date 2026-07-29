---
spec: docs/specs/ui/review-markdown-preview.md
created: 2026-07-29
status: done
---

# Implementation Plan: Review Markdown Preview

## Overview

The Review file toolbar already renders `Preview markdown` for `.md` and `.mdx` paths when it
receives a preview callback. The missing link is the expanded Review dialog: its mount supplies
only the source-edit action. Wire the existing preview action through the dialog and keep the
repository name attached; on mobile, route the same intent into the existing full-height file
viewer with preview mode selected initially.

## Frontend

### Review action wiring

- Update `apps/web/components/task/use-review-dialog.ts` to expose
  `useFileEditors().openFileInMarkdownPreview` beside the existing source-open action.
- Thread a repository-aware `onPreviewMarkdown(filePath, repo?)` callback through
  `dockview-review-dialog.tsx`, `review-dialog.tsx`, `review-dialog-surface.tsx`,
  `review-diff-list.tsx`, `review-diff-header.tsx`, and `review-diff-toolbar.tsx`.
- Keep the existing `.md`/`.mdx` guard and eye-icon/menu presentation; non-Markdown rows remain
  unchanged.

### Mobile behavior

- Extend the file-opening state in
  `apps/web/components/task/mobile/session-mobile-layout.tsx` so a caller can request that a
  fetched Markdown file open with preview mode active.
- Let `TaskReviewDialogMount` accept responsive file-open overrides. The mobile layout supplies
  its existing file fetch/navigation handler for source editing and a preview-mode variant for
  Markdown rendering; desktop keeps the Dockview-backed defaults.
- Update `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx` to accept an initial
  Markdown-preview mode while retaining the existing in-view toggle.

### Mobile design contract

- **Desktop outcome:** the eye action in the sticky review header opens a Dockview file panel in
  rendered mode.
- **Mobile entry point:** the existing 44 px `More actions` button in the sticky review header.
- **Nearest exemplar:** `MobileFileViewerPanel`, which contributes the dedicated full-height file
  surface, fixed toolbar, and single internal content region.
- **Hierarchy and primary action:** Review remains the current context until the user chooses
  preview; the Files viewer then becomes the sole focal surface and renders the selected document.
- **Presentation:** direct navigation to the existing full-height viewer, appropriate for dense
  document content and repeated reading; no intermediate drawer is added beyond the existing
  actions menu.
- **Scrolling and safe area:** the existing mobile task layout owns `100dvh` and safe-area
  offsets; the viewer retains its single `PanelBody` content owner.
- **Shared versus responsive state:** file identity, fetch logic, repository scoping, and Markdown
  rendering stay shared; only the mobile initial-view flag and navigation composition differ.

## Tests

- `apps/web/components/review/review-diff-toolbar.test.tsx`: prove `.md`/`.mdx` visibility,
  non-Markdown absence, and repository-aware callback dispatch on desktop and mobile.
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`: prove a review preview request
  fetches the selected repository file, selects it, and navigates to Files with preview intent.
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`: prove initial preview mode
  renders the Markdown surface without requiring the source-view toggle.

## E2E Tests

- `apps/web/e2e/tests/review/review-markdown-preview.spec.ts`: create a changed `.md` file, open
  expanded Review, activate the desktop eye action, and assert the rendered heading is visible.
- `apps/web/e2e/tests/review/mobile-review-markdown-preview.spec.ts`: create a changed `.md` file,
  open Review from mobile Changes, choose the action from the file menu, and assert the rendered
  heading in the full-height viewer.

## Implementation

- [x] [task-01-review-markdown-preview](task-01-review-markdown-preview.md) — done

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run \
  components/review/review-diff-toolbar.test.tsx \
  components/task/mobile/session-mobile-layout.test.tsx \
  components/task/mobile/mobile-file-viewer-panel.test.tsx
pnpm --dir web e2e:run tests/review/review-markdown-preview.spec.ts
pnpm --dir web e2e:run tests/review/mobile-review-markdown-preview.spec.ts -- --project=mobile-chrome
cd ..
make fmt
make typecheck test lint
```

## Risks

- The same relative path can exist in multiple task repositories; every callback layer must
  preserve `repository_name`.
- Mobile file fetches are asynchronous and already cancel stale requests; preview intent must
  remain paired with the winning request rather than leaking to a later source-open action.
- The Review dialog closes when the responsive viewer takes focus; returning must preserve the
  existing mobile navigation behavior rather than introducing a second overlay.
