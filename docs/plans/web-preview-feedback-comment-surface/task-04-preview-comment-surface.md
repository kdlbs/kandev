---
id: "04-preview-comment-surface"
title: "Web preview adopts the shared comment surface"
status: done
wave: 2
depends_on:
  - "02-canvas-capture-readability"
  - "03-shared-selection-comment-surface"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
  - REQ-UI-SELECTION-COMMENT-SURFACE-001
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.6
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.7
  - AC-UI-SELECTION-COMMENT-SURFACE-001.1
  - AC-UI-SELECTION-COMMENT-SURFACE-001.7
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
  - ../../specs/tasks/system-design/web-preview-feedback-02.md
  - ../../specs/ui/system-design/selection-comment-surface.md
---

# Task 04: Web preview adopts the shared comment surface

## Summary

Replace the web preview's inline `DraftEditor` and the collection's inline
textarea editor with `SelectionCommentSurface`. Anchor it at the capture, or
at the edited row. Keep a dismissed draft resumable.

## In scope

- `PreviewFeedbackControls`: render the surface for an open draft. Anchor it
  with `anchorBelowRect`, using the capture rect plus the iframe origin and
  clamping to the iframe bounds. Use `contextLabel = page_route`, shared
  evidence, the controlled draft, and no Run.
- `use-preview-capture`: `isDraftSurfaceOpen`, `closeDraftSurface`, and
  `resumeDraft`. `startCapture` stays blocked while a draft exists.
- Annotate menu and Drawer: a draft row with Resume and Discard, and capture
  modes disabled with a hint. A draft no longer reopens the feedback Drawer or
  menu, but rasterizing and errors still do.
- `PreviewFeedbackEvidence`, extracted from the collection's
  `FeedbackEvidence`, accepts a draft or a saved item. It is used by the list
  and the surface.
  For a draft with `captureWarning: "canvas-uniform"` from task 02, it renders
  the warning under the thumbnail, and Add stays enabled.
- Collection Edit opens the surface in edit mode (Update and Delete,
  version-checked). On touch, the collection Drawer is replaced while editing.

## Out of scope

- Bridge or inspector changes, capture modes, composer delivery, and backend
  changes.

## ASCII UI preview

UI-01, UI-02, and UI-03 from the [plan](plan.md#ascii-ui-previews):

```text
   +=========+  <- captured region
   +-[ /pricing ]---------------- = ---+
   | [thumbnail | "Buy now"]           |
   | [Add your comment...            ] |
   | Mod+Enter to add          [+ Add] |
   +-----------------------------------+

+ Feedback ------------------------------------+
| Draft comment on /pricing  [Resume] [Discard] |
| [Text] [Element] [Screenshot]  (disabled)     |
+-----------------------------------------------+
```

## Implementation acceptance conditions

1. Desktop: after text, element, and screenshot captures, the surface appears
   within the iframe bounds, next to the captured rect. Mod+Enter saves.
   Escape keeps the draft, and Resume restores the typed text.
2. A failed screenshot upload or create keeps the surface open with the
   comment and an error. A conflict on Update keeps the edit surface open and
   refreshes the snapshot.
3. Phone (`mobile-chrome`): the capture opens the surface Drawer. Edit from
   the feedback Drawer opens the edit Drawer and returns to the list after it
   closes. The 44px targets and the no-horizontal-overflow assertions in the
   existing mobile spec still pass.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/task/inspector hooks/use-preview-capture.test.ts components/task/browser-panel.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx
pnpm run typecheck && pnpm run lint && pnpm run i18n:check
pnpm e2e:run --host --shards 1 --project chromium tests/preview/preview-feedback.spec.ts tests/chat/html-preview.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-html-preview.spec.ts
```

## Likely files

- `apps/web/components/task/inspector/preview-feedback-controls.tsx`, `preview-feedback-collection.tsx`, and their tests
- `apps/web/hooks/use-preview-capture.ts`, `.test.ts`
- `apps/web/components/task/browser-panel.tsx`, `html-preview-content.tsx` (pass the iframe ref or rect to the controls)
- `apps/web/src/locales/*/task.json` (draft row and hint copy; remove the unused `previewSaveFeedback`/`previewDiscardSelection` keys if nothing else uses them)
- `apps/web/e2e/tests/preview/preview-feedback.spec.ts`, `preview-feedback-helpers.ts`, `e2e/tests/task/mobile-html-preview.spec.ts`

## Risks

- A Drawer nested in a Drawer. See the plan's risks section.
- The preview iframe may be scrolled or partly offscreen, so clamp the anchor
  to the visible iframe rect.

## Results

Done.

- Capture drafts and pending-item edits use `SelectionCommentSurface`,
  anchored with `anchorBelowRect` (also used by question-review regions).
- Dismissed drafts are resumable from the Annotate menu; capture modes are
  disabled while a draft exists.
- Host Popover/menu handlers ignore interaction inside
  `[data-selection-comment-surface]`; on touch the edit Drawer opens above the
  collection Drawer.

Commands: inspector, hook, browser-panel and mobile viewer Vitest pass;
Chromium `preview-feedback`, `preview-feedback-comment-surface`,
`html-preview` and mobile-chrome `mobile-html-preview` pass.
