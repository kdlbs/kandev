---
created: 2026-09-05
status: complete
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
legacy_specs: []
---

# Implementation Plan: File Create Menu Focus

## Overview

Keep the Files-panel filename input focused after a person chooses **New File**
from the upload-enabled create menu. One sequential frontend slice defers inline
creation until the dropdown finishes closing and proves the shared desktop and
mobile behavior.

## Confirmed root cause

`CreateMenu` calls `onStartCreate` from `DropdownMenuItem.onSelect`. This mounts
`InlineFileInput` while Radix is still closing the dropdown. The input focuses
itself on the next animation frame, then Radix restores focus to the create-menu
trigger. `InlineFileInput.onBlur` interprets the empty value as cancellation and
removes the input.

An isolated browser reproduction observed the input focus, trigger focus
restoration, and input removal in sequence. The input lost focus after about
163 ms on desktop and 97 ms on a Pixel-sized mobile viewport. No console error
or file API request accompanied the disappearance.

## Scope

### In scope

- Defer the **New File** callback until the create dropdown closes.
- Suppress Radix trigger-focus restoration only for a pending **New File**
  selection so the freshly mounted filename input keeps focus.
- Preserve default trigger-focus restoration for upload selections, Escape,
  and outside dismissal.
- Prove filename-input persistence and focus on desktop and mobile.

### Out of scope

- File creation, upload, download, rename, tree selection, or filesystem API
  semantics.
- Changes to `InlineFileInput` blur-to-submit and blur-to-cancel behavior.
- Menu layout, touch-target sizing, responsive composition, copy, or
  localization.
- Backend, persistence, feature flags, and public documentation.

## Technical approach

- Add a pending-create ref and focused close handler to `CreateMenu` in
  `apps/web/components/task/file-browser-toolbar.tsx`.
- Make the **New File** item mark creation as pending. In
  `DropdownMenuContent.onCloseAutoFocus`, prevent default focus restoration,
  consume the pending state, and call `onStartCreate`.
- Leave all non-create closes on Radix's default path. This mirrors the shipped
  deferred inline-rename pattern in `FileContextMenuSurface` and the original
  workspace-file-transfer plan's requirement to use `onCloseAutoFocus`.
- Keep `InlineFileInput` unchanged; once it mounts after the dropdown closes,
  its existing animation-frame focus and blur semantics are correct.

## Mobile design contract

- Desktop enters through the Files-panel create dropdown; mobile enters through
  the same shared toolbar after selecting Files from the bottom navigation.
- The shared `CreateMenu`, file-browser state, and inline input own the behavior
  on both viewports. No mobile-specific presentation branch is added.
- The existing Radix menu remains the nearest shipped contextual-action
  exemplar, including its 44px mobile trigger and menu-item hit areas.
- Information hierarchy, scroll ownership, safe-area handling, and menu geometry
  are unchanged. The only repaired outcome is that the primary filename input
  remains visible and focused after the menu closes.

## Tests

- `AC-UI-WORKSPACE-FILE-TRANSFER-001.1`: extend
  `apps/web/components/task/file-browser-toolbar.test.tsx` with a stateful
  harness that uses the real inline input. Before the correction, choosing
  **New File** lets close-autofocus blur and remove the input; after the
  correction, the menu is closed and the input remains focused.
- Preserve default focus restoration after upload selection and menu dismissal
  in the same component suite so the create-only exception cannot leak.

## E2E tests

- Desktop Chromium: strengthen
  `apps/web/e2e/tests/task/file-tree-create.spec.ts` so the helper waits for the
  dropdown to finish closing before asserting that the filename input remains
  visible and focused.
- Mobile Chromium: strengthen the existing new-file flow in
  `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts` with the same post-close
  persistence and focus assertion. The shared behavior changes without a new
  mobile composition or screenshot contract.

## Work orders

- [x] [Task 01: Preserve new-file focus after menu close](task-01-preserve-new-file-focus.md)

## Verification results

- Component regression: 8 tests passed. The regression failed before the
  production change because the filename input disappeared during dropdown
  close-autofocus.
- Desktop Chromium E2E: 1 test passed after waiting for the create menu to
  detach before checking the input.
- Mobile Chromium E2E: 1 test passed with the same post-close focus contract.
- Web typecheck: passed.
- Targeted ESLint: passed with no warnings.
- `git diff --check`: passed.

## Risks

- An unconditional `preventDefault()` would strand focus on ordinary upload or
  dismissal paths. The pending state must scope both deferral and focus
  suppression to **New File**.
- A pending flag that survives one close could replay creation on a later menu
  close. Consume it exactly once and cover the next-close behavior.
- Immediate E2E assertions can race the closing animation and report a false
  pass. Assertions must wait for the menu item to detach before checking the
  filename input.

## Open questions

None.
