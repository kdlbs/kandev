---
created: 2026-11-16
status: implemented
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-UI-SELECTION-COMMENT-SURFACE-001
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
  - ../../specs/tasks/system-design/web-preview-feedback-02.md
  - ../../specs/tasks/system-design/plan-comments.md
  - ../../specs/tasks/system-design/plan-comments-02.md
  - ../../specs/ui/system-design/selection-comment-surface.md
legacy_specs: []
---

# Implementation plan: Web preview feedback reliability and shared comment surface

## Overview

This plan improves web-preview feedback in three ways and removes duplicate
comment popovers across Kandev:

1. **Issue #4964:** comment-bearing queued messages (web preview feedback and
   plan comments) are no longer silently dropped when dispatch fails before
   agentctl.
2. **Issue #3992:** screenshot feedback over WebGL canvases (Flutter web's
   CanvasKit and `--wasm` renderers included) contains the
   rendered pixels. A capture over a canvas that comes out one uniform color
   shows a warning, and the user decides whether to keep it.
3. **DRY:** one `SelectionCommentSurface` replaces `PlanSelectionPopover`,
   `EditorCommentPopover`, and the web preview's inline draft and edit forms.
   The anchoring and evidence helpers follow the question-review region
   popover from PR #5239.

A search of open issues found no other web-preview issues besides #4964 and
#3992. The Flutter semantics tip from #3992 becomes a docs note in task 05.

## Requirements and designs

| Area | Requirement | Design |
| --- | --- | --- |
| Pre-transport delivery failure | `AC-TASKS-PLAN-COMMENTS-002.9` | [plan-comments-02.md: Failures after the marker but before transport](../../specs/tasks/system-design/plan-comments-02.md#failures-after-the-marker-but-before-transport) |
| Canvas readability | `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.8`, `.9` | [web-preview-feedback-02.md: Canvas pixel readability](../../specs/tasks/system-design/web-preview-feedback-02.md#canvas-pixel-readability) |
| Shared surface | `AC-UI-SELECTION-COMMENT-SURFACE-001.1`–`.8` | [selection-comment-surface.md](../../specs/ui/system-design/selection-comment-surface.md) |
| Preview adoption | `AC-TASKS-WEB-PREVIEW-FEEDBACK-004.6`, `.7` | [web-preview-feedback-02.md: Comment surface](../../specs/tasks/system-design/web-preview-feedback-02.md#comment-surface) |

## Work orders and dependency order

| ID | Work order | Wave | Depends on |
| --- | --- | --- | --- |
| 01 | [Keep undispatched comment messages queued](task-01-undispatched-comment-delivery.md) | 1 | none |
| 02 | [Readable WebGL screenshot captures](task-02-canvas-capture-readability.md) | 1 | none |
| 03 | [Shared selection comment surface](task-03-shared-selection-comment-surface.md) | 1 | none |
| 04 | [Web preview adopts the shared surface](task-04-preview-comment-surface.md) | 2 | 03 (and 02 for the shared evidence/error copy) |
| 05 | [Public docs](task-05-public-docs.md) | 3 | 02, 04 |

Tasks 01, 02, and 03 touch separate files and can be done in any order.
Task 04 edits the same preview files as task 02's error handling
(`use-preview-capture.ts`), so run it after task 02.

## ASCII UI previews

The drawings show structure only. Spacing is illustrative. All copy goes
through `t()` in seven locales.

### UI-01: Desktop capture comment (Browser panel, after a capture)

Before: the draft form sits inside the toolbar's Annotate menu, away from the
capture.

```text
+-- Browser ---------------------------------------- [Annotate 2 v] --+
|                                                 +----------------+  |
|   (page)                                        | Feedback       |  |
|   +=========+  <- captured region               | [Text][Elem][Shot]|
|   |         |                                   | /pricing       |  |
|   +=========+                                   | "Buy now"      |  |
|                                                 | [textarea    ] |  |
|                                                 | Discard   Save |  |
|                                                 +----------------+  |
+----------------------------------------------------------------------+
```

After: the shared surface floats below the capture. The toolbar keeps only the
mode menu and the pending list.

```text
+-- Browser ---------------------------------------- [Annotate 2  ] --+
|   (page)                                                             |
|   +=========+  <- captured region                                    |
|   |         |                                                        |
|   +=========+                                                        |
|   +-[ /pricing ]---------------- = ---+   <- drag handle, route label |
|   | [thumbnail 320x180 | "Buy now"]   |   <- evidence                 |
|   | [Add your comment...            ] |                               |
|   | Mod+Enter to add            [+ Add] |  <- no Run                  |
|   +-----------------------------------+                               |
+----------------------------------------------------------------------+
```

### UI-02: Draft kept after dismissal (Annotate menu)

```text
+ Feedback ------------------------------+
| Draft comment on /pricing  [Resume] [Discard] |
| Finish or discard the draft to capture again  |
| [Text] [Element] [Screenshot]  (disabled)     |
| ---- Pending (2) ----                         |
| /pricing  "Buy now"  "make it bigger" [Edit][x]|
+-----------------------------------------------+
```

### UI-03: Phone capture comment and edit (coarse pointer)

```text
+---------------------------+
| (preview, full viewport)  |
|                           |
|---------------------------|  <- shared surface Drawer
| Comment                   |
| /pricing                  |
| [thumbnail]               |
| [Add your comment...    ] |
| [        + Add        ]   |  44px
+---------------------------+
```

Editing a pending item from the feedback Drawer opens the same Drawer with the
title Edit comment, plus Update and Delete. Closing it returns to the feedback
Drawer.

### UI-04: Uniform canvas capture (warning state)

The warning appears on the comment surface under the thumbnail. Add stays
enabled, because the region may really be solid.

```text
+-[ /app ]------------------------ = ---+
| [ solid thumbnail 400x300 ]           |
| ! Canvas content may not have been    |
|   captured. Check the image first.    |
| [Add your comment...                ] |
| [Discard]                     [+ Add] |
+---------------------------------------+
```

### UI-05: Code editor and Markdown preview

Desktop is unchanged (line-range badge, Add/Run). On a coarse pointer, the
docked Comment action now opens the shared Drawer instead of a floating
popover:

```text
|---------------------------|
| Comment   [Lines 3-5]     |
| "const x = ..."           |
| [Add your comment...    ] |
| [ + Add ][ > Run ]        |
+---------------------------+
```

## Verification strategy

Each work order lists its exact commands. Across the package:

- Go unit and integration tests for lifecycle classification, marker
  clearing, requeue, and logs (01). Go string-order tests for the shim (02).
- Vitest for the screenshot uniformity check, the pixel-limit helper, the
  shared surface, and preview adoption (02–04).
- Playwright `chromium` for WebGL capture, the anchored surface, editing,
  draft resume, and the regression specs of existing surfaces. Playwright
  `mobile-chrome` for the Drawers (02–04).
- Run `make fmt`, then `make typecheck test lint`, and `pnpm run i18n:check`,
  before the commit.

## Risks

- **01:** requeue must keep `client_queue_id`, the fingerprint, and
  durable-transcript metadata. If `requeueMessageForSession` rebuilds the
  entry without them, use `restoreQueuedMessageForSession` for comment-bearing
  entries instead, and cover both. On-turn-start processing already ran inside
  the boundary, so check that `turnStartAlreadyProcessed` stops it from
  running twice on retry.
- **02:** forcing `preserveDrawingBuffer` can lower frame rate on some GPUs
  for heavy WebGL pages in previews. It applies only to proxied preview pages.
  The task 02 spike confirmed CanvasKit and `--wasm` (`skwasm`) captures with
  Flutter 3.50. Worker renderers stay unsupported and show the warning.
- **03:** moving the editor popovers onto the shared surface changes their
  touch presentation from floating to Drawer. This is the intended change and
  is covered by `mobile-file-viewer`. Removing translation keys must not break
  other users. Search for each key before deleting it.
- **04:** a Drawer nested inside a Drawer on touch. If `@kandev/ui` Drawer
  cannot nest, close the collection Drawer while editing and reopen it after,
  as the design states.

## Exclusions

- Browser screen-capture permission flows (`getDisplayMedia`) and native
  desktop capture.
- Monaco and diff inline comment forms, and `CommentViewPopover`.
- Marking chat messages as undelivered in the transcript (#4958 territory).
- Commits, pushes, and PRs until the user asks for them.
