---
created: 2026-11-18
status: implemented
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
legacy_specs: []
---

# Implementation plan: Web preview annotation review

## Overview

The second test round of [annotation mode](../web-preview-annotation-mode/plan.md)
reported nine issues:

1. An empty comment box cannot be dismissed without typing, saving, and
   deleting.
2. Escape closes a comment box but leaves its numbered marker.
3. Clicking a marker does not reopen its comment.
4. Region capture stays on "Capturing screenshot...".
5. The drawn region disappears while it is captured. It should stay, ideally
   with an animated border.
6. Scrolling right after drawing a region, before capture finishes, must not
   change what is captured.
7. The toolbar's count button opens the list but does not close it.
8. The element comment box flashes, and dragging it highlights other
   elements.
9. A comment list beside the preview, open by default in the mode, that can be
   hidden to give the page the full width.

Decisions from the round:

- A docked comment panel where comments are written and edited, with markers
  and entries sharing one number. Hiding it keeps the mode and returns the
  full width; the floating comment box is used while it is hidden. The device
  remembers the choice.
- Closing a comment with no text removes the capture. Closing with text keeps
  it as an unsaved draft.
- Root causes are recorded in
  [part 4](../../specs/tasks/system-design/web-preview-feedback-04.md#defects-found-in-testing).
  Issue 4 is a StrictMode lifecycle bug that only dev builds hit.

## Requirements and designs

| Area | Criteria | Design |
| --- | --- | --- |
| Capture feedback and reliability | `006.5` | [part 4: Capture feedback](../../specs/tasks/system-design/web-preview-feedback-04.md#capture-feedback) |
| Closing a comment | `005.4`, `005.5`, `006.4` | [part 4: Closing a comment](../../specs/tasks/system-design/web-preview-feedback-04.md#closing-a-comment) |
| Markers and numbering | `006.2` | [part 4: Numbering and markers](../../specs/tasks/system-design/web-preview-feedback-04.md#numbering-and-markers) |
| Pausing the tool | `006.6` | [part 4: Pausing the tool](../../specs/tasks/system-design/web-preview-feedback-04.md#pausing-the-tool) |
| Comment panel and toolbar | `005.2`, `006.1`, `006.3` | [part 4: Comment panel](../../specs/tasks/system-design/web-preview-feedback-04.md#comment-panel), [Toolbar](../../specs/tasks/system-design/web-preview-feedback-04.md#toolbar), [Where a comment is written](../../specs/tasks/system-design/web-preview-feedback-04.md#where-a-comment-is-written) |

All criteria IDs above are `AC-TASKS-WEB-PREVIEW-FEEDBACK-*`.

## Work orders and dependency order

| ID | Work order | Wave | Depends on |
| --- | --- | --- | --- |
| 01 | [Capture reliability and busy outline](task-01-capture-reliability.md) | 1 | none |
| 02 | [Shared comment surface: close, composer, drag shield](task-02-shared-comment-surface.md) | 1 | none |
| 03 | [Bridge additions](task-03-bridge-additions.md) | 1 | none |
| 04 | [Close reducer, numbering, markers, pausing](task-04-draft-lifecycle.md) | 2 | 01, 03 |
| 05 | [Comment panel and toolbar toggle](task-05-comment-panel.md) | 3 | 02, 04 |
| 06 | [E2E coverage and public docs](task-06-e2e-and-docs.md) | 4 | 05 |

Wave 1 touches separate files: the screenshot hook and rasterizer (01), the
shared selection-comment components (02), and the inspector script with its
validators (03).

## ASCII UI previews

The drawings show structure only. All copy goes through `t()` in seven
locales.

### UI-01: Desktop, panel open (default)

```text
+-- Browser ------------------------------------------------ [Annotate 3] --+
|#################################################|  Comments          [x] |
|#   +----------------------------------------+   #| Unsaved                 |
|#   | [Interact][Text][Element][Region*] |   #|  3  [thumb] "tighter"   |
|#   |  [(c) 3 pressed] | [Exit]            |   #|      [Resume] [Discard] |
|#   +----------------------------------------+   #| /pricing                |
|#                                                 #|  1  [thumb] "bigger"    |
|#   +1-------------+   (2)  <- element marker     #|      [Edit] [Delete]    |
|#   |              o                              #|  2  "Buy now" "colour"  |
|#   +--------------o   +3- - - - - -+ <- unsaved  #|      [Edit] [Delete]    |
|#                      + - - - - - -+             #|            [Clear all]  |
|#################################################|                          |
+---------------------------------------------------------------------------+
            page area (reflows)                        320 px panel
```

The `(c) 3` toggle is pressed while the panel is open. Clicking marker 2
selects entry 2 and opens its editor in the panel.

### UI-02: Desktop, writing a comment in the panel

```text
|#   +4=~=~=~=~=~=+  <- busy outline,   #| /pricing                |
|#   ~            ~     rotating border #|  4  Capturing...        |
|#   +=~=~=~=~=~=~+                     #|  +--------------------+ |
|#                                      #|  | Add your comment   | |
|#                                      #|  +--------------------+ |
|#                                      #|        [Close] [Add]    |
```

`Add` is disabled until the image is ready. `Close` with no text removes
region 4.

### UI-03: Desktop, panel hidden (full width)

```text
+-- Browser ------------------------------------------------ [Annotate 3] --+
|##########################################################################|
|#          +--------------------------------------------------+          #|
|#          | [Interact][Text][Element*][Region] | [(c) 3] | [Exit] |     #|
|#          +--------------------------------------------------+          #|
|#   +-----------------+                                                  #|
|#   | Buy now  [=====]|  <- highlighted element, tool paused             #|
|#   +-----------------+                                                  #|
|#      +-- :: ----------------------- [x] --+  <- shared floating box     #|
|#      | <button> "Buy now"                 |                            #|
|#      | [Add your comment              ]   |                            #|
|#      |                     [Add]          |                            #|
|#      +------------------------------------+                            #|
|##########################################################################|
```

### UI-04: Narrow desktop host (under 720 px): panel overlays

```text
+-- Browser ------------------------+
|###################+--------------+|
|# page (full width)| Comments  [x]||
|#  +1----+         |  1  "bigger" ||
|#  +-----+         |  2  "colour" ||
|###################+--------------+|
+-----------------------------------+
```

### UI-05: Phone, sheet hidden (default) and shown

```text
+---------------------------+   +---------------------------+
|###########################|   |#  (page, dimmed)         #|
|#  (page)                 #|   +---------------------------+
|#  +1------+   (2)        #|   | ===                       |
|#  +-------+              #|   | Comments                  |
|###########################|   |  1  [thumb] "bigger"  [>] |
| [I][T][E][R*] [(c)3] [Exit]|  |  2  "Buy now"         [>] |
+---------------------------+   +---------------------------+
```

On phones a capture opens the shared drawer composer, which has a Close
button.

## Verification strategy

Each work order lists its exact commands. Across the package:

- Vitest for the StrictMode lifecycle, timeout, `onclone` scroll restore,
  close reducer, numbering, panel preference, shared surface controls, drag
  shield, and validators (01-05). The existing selection-comment, chat, plan,
  and question-review tests stay green after task 02.
- Node-executed inspector tests for the bridge additions (03).
- Playwright `chromium` and `mobile-chrome` for the flows in part 4's
  verification section (06), plus the existing preview specs as regression.
- `make fmt`, then `make typecheck test lint`, and `pnpm run i18n:check`,
  before the commit.

## Risks

- **01:** a capturing draft has no image yet. Every consumer of drafts
  (save, send gating, screenshot limit, thumbnails) must handle it; saving
  waits for the image or reports the capture error.
- **02:** the shared surface is used by chat, plan, diff, and editor comments.
  `onCancel` and the drag shield must not change their behavior unless they
  opt in; the shield only exists during a drag.
- **03:** making badges clickable inside the page can cover page content.
  Badges are 22 pixels and clickable only in annotation mode.
- **05:** docking reflows the page when the panel toggles, which moves
  content under document-anchored region outlines, as any resize does. The
  inspector re-projects on resize; region outlines keep document coordinates,
  so a reflow can offset them. Overlay mode avoids reflow on narrow hosts.
- **05:** the Browser panel and HTML preview mount the layer in different
  containers. Both must become row layouts without breaking their existing
  sizing.

## Exclusions

- Threaded replies or resolved states for comments.
- Reordering comments.
- A panel outside annotation mode; the header popover keeps the list there.
- Changing the backend or the screenshot limit.

## Implementation record

Delivered as planned, with these refinements recorded in part 4:

- The Close control sits beside Add in every comment form, rather than as an
  X in the desktop header, so the inline panel form and the popover match.
- `project-markers` gained `reveal_id` besides `highlighted_id`. Only an
  entry picked in the panel scrolls the page; opening a new capture's comment
  never does.
- A region drag that starts on a marker badge still draws a region.
- On hosts narrower than 720 pixels the overlay panel starts below the
  toolbar, so every tool stays reachable.
- Scrolling during a capture and the busy outline are covered by Vitest
  (`onclone` scroll restore, capturing-draft lifecycle); a capture finishes
  too quickly in E2E to observe them reliably.
- Older E2E flows that click under the panel on the narrow E2E preview start
  with it hidden through `startWithCommentPanelHidden`.
