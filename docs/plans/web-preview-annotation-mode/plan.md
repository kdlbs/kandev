---
created: 2026-11-17
status: implemented
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
  - ../../specs/tasks/system-design/web-preview-feedback-02.md
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
legacy_specs: []
---

# Implementation plan: Web preview annotation mode

## Overview

User testing of the shared comment surface produced four requests:

1. Selected text in the preview should be commentable without entering a
   mode, with a floating comment button on desktop and the selection dock on
   phones, as in chat and plan comments.
2. Leaving capture should be obvious: Escape and a visible Exit control,
   instead of a Cancel item hidden in a closed menu.
3. Screenshot regions should work like image review in ask-user questions: draw
   any number of rectangles, each with a comment, without restarting the tool.
4. Replace the one-shot menu with an annotation mode: an outlined preview and
   a toolbar of tools that stay selected, like an image editor.

Decisions from the test round:

- Keep the page live. Freezing a still image would reuse more of image review,
  but would stop scrolling and break the Element and Text tools.
- Capture each region when its drag ends, never at send time. Outlines follow
  the document or the inner scroll container they were drawn on.
- Regions can be moved and resized after capture, for drafts and saved items.
  Release recaptures and keeps the comment. A saved item's image is replaced
  in a version-checked backend update.

## Requirements and designs

| Area | Criteria | Design |
| --- | --- | --- |
| Comment on selection | `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1` | [part 3: Comment on selection](../../specs/tasks/system-design/web-preview-feedback-03.md#comment-on-selection) |
| Mode, tools, layout | `005.2`, `005.3` | [part 3: Mode and tools](../../specs/tasks/system-design/web-preview-feedback-03.md#mode-and-tools), [Layout](../../specs/tasks/system-design/web-preview-feedback-03.md#layout) |
| Drafts | `005.4` (supersedes `004.7`) | [part 3: Drafts](../../specs/tasks/system-design/web-preview-feedback-03.md#drafts) |
| Escape and exit | `005.5` | [part 3: Escape and exit](../../specs/tasks/system-design/web-preview-feedback-03.md#escape-and-exit) |
| Regions and recapture | `005.6` | [part 3: Regions](../../specs/tasks/system-design/web-preview-feedback-03.md#regions), [Replacing a saved screenshot](../../specs/tasks/system-design/web-preview-feedback-03.md#replacing-a-saved-screenshot) |
| Scroll tracking | `005.7` | [part 3: Tracking regions through scrolling](../../specs/tasks/system-design/web-preview-feedback-03.md#tracking-regions-through-scrolling) |

## Work orders and dependency order

| ID | Work order | Wave | Depends on |
| --- | --- | --- | --- |
| 01 | [Replace a saved screenshot capture](task-01-replace-screenshot-capture.md) | 1 | none |
| 02 | [Shared region annotation primitives](task-02-shared-region-annotation.md) | 1 | none |
| 03 | [Inspector bridge protocol 3](task-03-bridge-protocol-3.md) | 1 | none |
| 04 | [Draft list and annotation mode state](task-04-drafts-and-mode-state.md) | 2 | 03 |
| 05 | [Annotation layer, toolbar, and regions](task-05-annotation-layer.md) | 3 | 01, 02, 04 |
| 06 | [Public docs](task-06-public-docs.md) | 4 | 05 |

Tasks 01, 02, and 03 touch separate files (backend, shared components with
question review, inspector script and validators). Task 04 changes the hook
that task 05's UI consumes.

## ASCII UI previews

The drawings show structure only. All copy goes through `t()` in seven
locales.

### UI-01: Comment on selection, outside the mode (desktop)

```text
+-- Browser ------------------------------------------ [Annotate 3] --+
|                                                                      |
|   Lorem ipsum [dolor sit amet], consectetur                          |
|                              [c]   <- floating comment button        |
|                                                                      |
+----------------------------------------------------------------------+
```

Clicking the button opens the shared comment surface below the selection.

### UI-02: Annotation mode toolbar (desktop)

```text
+-- Browser ------------------------------------------ [Annotate 3] --+
|#####################################################################|  <- 2px primary ring
|#     +--------------------------------------------------------+     #|
|#     | [Interact][Text][Element][Region*] | 3 | [Exit] |     #|
|#     +--------------------------------------------------------+     #|
|#                                                                   #|
|#   +1-------------+         +2- - - - - - -+  <- unsaved, dashed   #|
|#   |              o         |              |                       #|
|#   +--------------o         + - - - - - - -+                       #|
|#     ^ corner handles (Region tool only)                           #|
|#####################################################################|
+----------------------------------------------------------------------+
```

`*` marks the pressed tool. The count button opens the pending panel.

### UI-03: Pending panel with unsaved drafts

```text
+ Feedback (3 saved, 1 unsaved) ------------------+
| Unsaved                                          |
|  2  /pricing  [thumbnail]         [Resume][x]    |
| ---- /pricing ----                               |
|  1  [thumbnail]  "make it bigger"    [Edit][x]   |
|  *  "Buy now"    "wrong colour"      [Edit][x]   |
|                               [Clear all]        |
+--------------------------------------------------+
```

### UI-04: Phone, annotation mode

```text
+---------------------------+
|###########################|
|#  (page)                 #|
|#  +1------+              #|
|#  |       o              #|
|#  +-------o              #|
|###########################|
| [I][T][E][R*]  (3) [Exit] |  <- bottom bar, 44px, safe area
+---------------------------+
```

### UI-05: Phone, comment on selection

```text
+---------------------------+
|  Lorem [dolor sit] amet   |
|  (native selection menu)  |
|                           |
|              [c Comment] |  <- shared SelectionActionDock
+---------------------------+
```

### UI-06: Region limit reached

```text
| [I][T][E][R disabled] | 10 | [Exit] |
  "Screenshot limit reached (10). Send or delete some first."
```

## Verification strategy

Each work order lists its exact commands. Across the package:

- Go tests for the screenshot replacement and the inspector script (01, 03).
- Vitest for validators, geometry, the overlay, the draft list, the Escape
  reducer, and recapture rollback (02-05). Question-review tests stay green
  after the extraction (02).
- Playwright `chromium` and `mobile-chrome` for the flows in the design's
  verification section (05), plus the existing preview and question-review
  specs as regression.
- Run `make fmt`, then `make typecheck test lint`, and `pnpm run i18n:check`,
  before the commit.

## Risks

- **03:** `selectionchange` fires often. Coalesce per animation frame and send
  only when the rectangle changes.
- **03:** scroll-container detection can pick a container that later
  re-renders. A missing selector reports `visible: false`, and the outline
  hides rather than drifting.
- **05:** a parent overlay over the iframe must not swallow wheel or touch
  scrolling. The layer stays `pointer-events: none`, and only handles,
  toolbar, and the selection button take input. Region drawing stays in the
  page bridge.
- **05:** recapturing a partly off-screen region renders DOM that is not
  visible. The design accepts this because the user is resizing it in view;
  html2canvas renders the full rectangle.
- **01:** the old attachment must be released only after commit, so a rollback
  never leaves an item without its image.

## Exclusions

- Freehand drawing, arrows, and other shapes.
- Moving or resizing text and element captures.
- Resize handles in question review. The shared overlay supports them, but
  that feature does not opt in here.
- Capturing at send time or full-page stitched captures.
