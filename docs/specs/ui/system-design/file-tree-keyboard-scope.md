---
status: draft
system: ui
requirements:
  - REQ-UI-FILE-TREE-KEYBOARD-SCOPE-001
---

# File Tree Keyboard Scope System Design

## Purpose and boundaries

This design owns keyboard-event routing inside the shared `FileBrowser`. It
keeps native text editing authoritative while an editable descendant has focus
and preserves the existing visible-row selection model everywhere else. It
does not change file data, filesystem operations, task state, or shortcut
configuration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-FILE-TREE-KEYBOARD-SCOPE-001` | [Shortcut ownership](#shortcut-ownership), [Responsive surfaces](#responsive-surfaces) |

## Components and responsibilities

`FileBrowser` in `apps/web/components/task/file-browser.tsx` owns the container
keydown listener through `useKeyboardShortcuts`. `useMultiSelect` continues to
own selected paths and the visible-row `selectAll` operation. Nested controls
such as `InlineFileInput`, `TreeNodeName`, and the file-search input continue to
own their text values and local editing behavior.

The existing `isEditableKeydownTarget` helper in
`apps/web/lib/keyboard/utils.ts` remains the shared classification boundary for
input, textarea, and contenteditable key targets. The file browser reuses that
boundary instead of defining another editable-element detector.

## Shortcut ownership

The container listener first keeps its existing check that keyboard focus is
inside the mounted file browser. Escape handling remains unchanged.

For Command+A or Control+A, the listener examines the originating keydown
target before preventing the browser default:

1. If the target is editable, the file browser returns without calling
   `preventDefault` or `useMultiSelect.selectAll`. Native browser selection and
   the focused control retain the event.
2. If the target is not editable, the file browser prevents native document
   selection and calls `useMultiSelect.selectAll` for the current visible paths.

No new React state, event bus, store field, or browser persistence is added.

## Responsive surfaces

Desktop and mobile task layouts mount the same `FileBrowser`, so the ownership
check also applies when a hardware keyboard is used with the phone Files
surface. The nearest mobile baseline remains
`apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`, which enters Files through
the existing bottom navigation. This change adds no mobile composition,
scrolling, safe-area, touch-target, or gesture behavior.

## Failure and compatibility

An unrecognized non-editable descendant follows the existing tree shortcut
path. An editable descendant follows the same established guard used by global
application and plugin shortcut dispatchers. File-tree Escape behavior, row
selection state, drag/drop, create, rename, and search flows remain unchanged.

## Verification

Desktop Playwright coverage shall reproduce the new-file flow with lowercase
`ControlOrMeta+a`, because Playwright's uppercase key spelling emits
`event.key === "A"` and bypasses the current lowercase shortcut branch. The
test shall assert both the input selection range and zero selected tree rows
after the next browser paint. A second desktop case shall prove that the same
shortcut still selects all visible rows when the non-editable tree container
has focus.

The existing mobile file-viewer spec shall exercise the filename input with a
hardware-keyboard Control+A event and prove the same focus ownership. No visual
snapshot is required because markup, geometry, and rendered styling do not
change.
