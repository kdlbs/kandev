---
created: 2026-09-01
status: complete
requirements:
  - REQ-UI-FILE-TREE-KEYBOARD-SCOPE-001
system_design:
  - ../../specs/ui/system-design/file-tree-keyboard-scope.md
legacy_specs: []
---

# Implementation Plan: File Tree Select-All Input Scope

## Overview

Keep Command+A and Control+A inside focused Files-panel text controls without
removing the file tree's existing visible-row select-all shortcut. One
sequential frontend slice adds a focused event-target guard and proves desktop
and hardware-keyboard mobile behavior.

## Confirmed root cause

`useKeyboardShortcuts` in `apps/web/components/task/file-browser.tsx` attaches a
keydown listener to the full file-browser container. When a nested filename
input emits lowercase Command+A or Control+A, the event bubbles to that
listener. The listener checks only that focus is somewhere inside the
container, then calls `preventDefault()` and `useMultiSelect.selectAll()`.

A temporary production-build Playwright repro observed the filename selection
remaining at `13..13`, all three visible tree rows becoming selected, and the
lowercase `a` event reaching the document with `defaultPrevented: true`.

## Scope

### In scope

- Defer the file-tree select-all shortcut when its originating target is an
  editable Files-panel control.
- Preserve native select-all behavior in new-file, rename, and search inputs.
- Preserve visible-row select-all when the non-editable tree surface owns
  focus.
- Prove shared desktop and hardware-keyboard mobile behavior.

### Out of scope

- File creation, rename, search, multi-selection, drag/drop, or Escape behavior
  beyond select-all event ownership.
- New shortcut preferences or changes to key combinations.
- Files-panel markup, layout, touch actions, scrolling, or copy.
- Backend, API, persistence, or localization changes.

## Technical approach

- Import `isEditableKeydownTarget` from
  `apps/web/lib/keyboard/utils.ts` into
  `apps/web/components/task/file-browser.tsx`.
- In `useKeyboardShortcuts`, gate only the Command+A / Control+A branch on the
  event target being non-editable. Keep focus containment, Escape behavior,
  `preventDefault`, and `useMultiSelect.selectAll` otherwise unchanged.
- Reuse the established editable-target boundary rather than adding a local
  element-type helper or changing nested input handlers.

## Tests

- `AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.1` and `.2`: extend
  `apps/web/e2e/tests/task/file-tree-create.spec.ts` with the exact reported
  flow. Type a draft filename, press lowercase `ControlOrMeta+a`, await the next
  paint, then assert the full input selection range and zero selected tree
  rows.
- `AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.3`: extend
  `apps/web/e2e/tests/file-tree-multi-select.spec.ts` to focus the non-editable
  file-browser container, press lowercase `ControlOrMeta+a`, and assert every
  visible row is selected.

## E2E tests

- Desktop Chromium: the new-file regression fails before the correction for
  both observable symptoms; the non-editable tree case protects existing
  multi-selection behavior.
- Mobile Chromium: extend
  `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts` to enter the existing
  Files surface, focus the new-file input, and prove a hardware-keyboard
  Control+A remains inside the input. Touch and visual behavior are unchanged,
  so no new mobile composition or screenshot contract is needed.

## Work orders

- [x] [Task 01: Scope file-tree select-all to focus](task-01-scope-file-tree-select-all.md)

## Verification results

Task 01 completed. The targeted desktop input, desktop tree, and mobile input
Playwright checks each passed against the rebuilt production bundle. Frontend
typecheck also passed. The work-order results retain the exact commands and RED
evidence.

## Risks

- An overbroad early return could disable Escape clearing or the tree's own
  select-all behavior. Gate only the select-all branch and cover both sides.
- Playwright key spelling is case-sensitive for `event.key`. Use lowercase
  `ControlOrMeta+a` so the regression exercises the production branch.

## Open questions

None.
