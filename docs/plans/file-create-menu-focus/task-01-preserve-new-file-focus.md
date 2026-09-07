---
id: "01-preserve-new-file-focus"
title: "Preserve new-file focus after menu close"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.1
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 01: Preserve New-File Focus After Menu Close

## Summary

Make the create dropdown finish closing before it mounts the inline filename
input. Add component, desktop, and mobile regressions that prove the input
remains visible and focused after close-autofocus completes.

## In scope

- Add create-only pending state and `onCloseAutoFocus` handling to
  `CreateMenu`.
- Add a failing component regression before changing production code.
- Strengthen the existing desktop and mobile new-file E2E flows to assert the
  post-close state.
- Preserve default trigger-focus restoration for non-create menu closes.

## Out of scope

- Changing inline input blur semantics or filesystem operations.
- Changing upload pickers, menu content, responsive layout, or copy.
- Adding a new mobile surface or modifying touch-target geometry.
- Backend and public-documentation changes.

## Acceptance

- Choosing **New File** closes the menu, then mounts and focuses one filename
  input that remains available for typing on desktop and mobile.
- Upload actions and ordinary dismissal retain Radix's normal focus restoration
  and never start inline creation.
- A consumed pending create cannot replay on a later menu close.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/file-browser-toolbar.test.tsx
cd apps/web && pnpm e2e:run --host tests/task/file-tree-create.spec.ts -- --grep "New file at root"
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-file-viewer.spec.ts -- --grep "select-all stays in the new-file name input"
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/task/file-browser-toolbar.tsx components/task/file-browser-toolbar.test.tsx e2e/tests/task/file-tree-create.spec.ts e2e/tests/task/mobile-file-viewer.spec.ts
```

## Files likely touched

- `apps/web/components/task/file-browser-toolbar.tsx`
- `apps/web/components/task/file-browser-toolbar.test.tsx`
- `apps/web/e2e/tests/task/file-tree-create.spec.ts`
- `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`

## Dependencies

None.

## Risks

- Suppressing close-autofocus outside the pending-create branch would regress
  keyboard focus for upload and dismissal paths.
- Waiting on elapsed time instead of the menu-close event would make the E2E
  regression timing-dependent.

## Parallelism

`sequential`

## Inputs

- [Workspace File Transfer Requirements](../../specs/ui/requirements/workspace-file-transfer.md),
  especially `AC-UI-WORKSPACE-FILE-TRANSFER-001.1`.
- [Workspace File Transfer System Design](../../specs/ui/system-design/workspace-file-transfer.md),
  especially the frontend `CreateMenu` responsibility.
- `FileContextMenuSurface` as the shipped pending-action and close-autofocus
  exemplar.
- Isolated desktop and Pixel-sized browser focus traces from the diagnostic
  turn.

## Results

- Added a create-only pending ref to `CreateMenu`. **New File** now mounts the
  inline input from `onCloseAutoFocus`, after the dropdown has closed, and
  suppresses trigger focus restoration for that close only.
- Added a stateful component regression using the real `InlineFileInput`. The
  test failed before the production change because the input was removed, then
  passed after the fix. It also proves the consumed create state does not
  replay and that upload and Escape paths retain normal trigger focus.
- Strengthened desktop and mobile E2E flows to wait for the menu item to detach
  before asserting filename-input focus.
- Verification passed: 8 focused component tests, 1 desktop Chromium E2E, 1
  mobile Chromium E2E, web typecheck, targeted ESLint, and `git diff --check`.
