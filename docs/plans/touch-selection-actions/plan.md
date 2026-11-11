---
created: 2026-11-11
status: implemented
requirements:
  - REQ-UI-TOUCH-SELECTION-ACTIONS-001
system_design:
  - ../../specs/ui/system-design/touch-selection-actions.md
legacy_specs: []
---

# Implementation Plan: Dock Touch Selection Comment Actions

## Overview

Tracking issue: kdlbs/kandev#5270.

On Android, selecting agent-reply text shows Kandev's floating Comment button
and the native selection menu covers it almost immediately. The Plan editor
avoids this by docking its actions at the bottom of the visible surface. This
plan applies that pattern to every text-selection comment surface on coarse
pointers: chat agent replies (Task Chat and Quick Chat), Office thread comment
extracts, the Markdown file preview, and the code file editors.

Delivery runs in two waves. Wave 1 builds the shared geometry hook, selection
tracker, and dock component, and fixes the reported chat surface end to end.
Wave 2 adopts the dock in the Markdown preview and code editors.

## Scope

### In scope

- Extract the Plan dock's bar geometry into `useDockedBarPosition` and keep the
  Plan dock as a regression consumer.
- Add `useTouchSelectionTracking` (`selectionchange`-driven) and
  `SelectionActionDock`.
- Replace floating triggers with the dock on coarse pointers in the four
  surfaces above, preserving fine-pointer behavior.
- Add one localized toolbar name in seven catalogs.
- Unit, component, and `mobile-chrome` E2E coverage.

### Out of scope

- Native selection-menu suppression.
- Comment data, anchors, delivery, composer redesign, or backend changes.
- Desktop trigger or popover changes.
- Image-region comments and Browser preview feedback.

## ASCII UI preview

Structure is normative; spacing is illustrative. Desktop (fine pointer) is
unchanged and not redrawn.

### UI-01: Chat agent reply, phone, text selected (before)

```text
+--------------------------------------+
| = Task title                     ... |
|--------------------------------------|
|  Agent                               |
|  The migration [renames the column]  |
|            +-------------------+ [C] |  <- native menu covers Kandev [C]
|            | Copy  Share  All  |     |
|            +-------------------+     |
|  and backfills existing rows.        |
|--------------------------------------|
| [ Message the agent...         ] (>) |
+--------------------------------------+
```

### UI-02: Chat agent reply, phone, text selected (after)

```text
+--------------------------------------+
| = Task title                     ... |
|--------------------------------------|  transcript scrolls
|  Agent                               |
|  The migration [renames the column]  |
|            +-------------------+     |
|            | Copy  Share  All  |     |  <- native menu, unobstructed
|            +-------------------+     |
|  and backfills existing rows.        |
|======================================|  dock: fixed, 48px, transcript bottom
|                        [ (C) Comment]|  toolbar "Selection actions"
|--------------------------------------|
| [ Message the agent...         ] (>) |
+--------------------------------------+
```

Tapping Comment opens the existing bottom Drawer (selected-text preview,
input, Add | Run). With the software keyboard open the dock sits directly
above the keyboard. In Quick Chat the dock renders inside the dialog at the
bottom of its transcript.

### UI-03: Markdown preview / code editor, phone, text selected (after)

```text
+--------------------------------------+
| <- README.md                 Preview |
|--------------------------------------|  preview/editor scrolls
|  ## Install                          |
|  Run [pnpm install] from apps/.      |
|            | Copy  Share  All  |     |
|======================================|  dock at preview/editor bottom
|                        [ (C) Comment]|
+--------------------------------------+
```

Tapping Comment opens each surface's existing comment composer.

Maps to `AC-UI-TOUCH-SELECTION-ACTIONS-001.2`, `.3`, `.4`, `.6`, `.8`.

## Work orders

| Order | Wave | Depends on | Result |
| --- | --- | --- | --- |
| [01](task-01-shared-dock-and-chat-messages.md) | 1 | none | Shared dock primitives; chat and Office thread comments docked on touch |
| [02](task-02-preview-and-editor-dock.md) | 2 | 01 | Markdown preview and code editors docked on touch |

## Verification summary

See each work order. Both finish with `pnpm run typecheck`, changed-file
lint, `pnpm run i18n:check` (01 only), and their `mobile-chrome` E2E specs.

## Risks

- `selectionchange` fires at high frequency during handle drags; coalesce to
  one animation frame and avoid store writes when the range is unchanged.
- Some Android builds clear the DOM selection on any tap; the captured target
  must not depend on the selection still existing at click time.
- The transcript host must be the visible scroller, not the virtualized inner
  content, or the dock will sit off-screen.
- Physical Android Chrome and iOS Safari checks may be unavailable; Playwright
  `mobile-chrome` cannot render the native selection menu.
