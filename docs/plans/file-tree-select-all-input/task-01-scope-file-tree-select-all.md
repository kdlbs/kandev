---
id: "01-scope-file-tree-select-all"
title: "Scope file-tree select-all to focus"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-TREE-KEYBOARD-SCOPE-001
acceptance_criteria:
  - AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.1
  - AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.2
  - AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.3
system_design:
  - ../../specs/ui/system-design/file-tree-keyboard-scope.md
---

# Task 01: Scope File-Tree Select-All to Focus

## Summary

Make the file-browser container defer Command+A and Control+A to focused text
controls while retaining visible-row selection for the non-editable tree. Add
desktop and mobile Playwright regressions before changing production code.

## In scope

- Add the editable-target guard to the file browser's select-all shortcut.
- Cover new-file input ownership on desktop and hardware-keyboard mobile.
- Cover retained tree-level select-all with non-editable focus.

## Out of scope

- Other file-browser keyboard behavior or shortcut configuration.
- Input markup, Files-panel layout, touch interactions, or backend behavior.
- New localization keys or public documentation.

## Acceptance

- A focused new-file control selects its complete value without selecting any
  file-tree row on desktop and mobile hardware-keyboard paths.
- Other editable Files controls inherit the same guard through the shared
  target classifier without per-input handlers.
- A focused non-editable tree container still selects every visible row.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/task/file-tree-create.spec.ts -- --grep "select-all"
cd apps/web && pnpm e2e:run --no-build tests/file-tree-multi-select.spec.ts -- --grep "select-all"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-file-viewer.spec.ts -- --grep "select-all"
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task/file-browser.tsx`
- `apps/web/e2e/tests/task/file-tree-create.spec.ts`
- `apps/web/e2e/tests/file-tree-multi-select.spec.ts`
- `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`

## Dependencies

None.

## Risks

- Guarding the whole keydown listener would change Escape behavior; restrict
  the guard to the select-all branch.
- Uppercase Playwright shortcut spelling would miss the lowercase production
  comparison and yield a false green test.

## Parallelism

`sequential`

## Inputs

- [File Tree Keyboard Scope Requirements](../../specs/ui/requirements/file-tree-keyboard-scope.md)
- [File Tree Keyboard Scope System Design](../../specs/ui/system-design/file-tree-keyboard-scope.md)
- Confirmed source path in `FileBrowser.useKeyboardShortcuts` and established
  editable-target behavior in `useAppShortcuts`.

## Results

Done.

- Reused `isEditableKeydownTarget` to defer only the file tree's select-all
  branch when an editable descendant originated the event. Escape and
  non-editable tree shortcuts remain unchanged.
- Added desktop and mobile new-file regressions plus a desktop preservation
  test for visible-row select-all.
- RED evidence: before the fix, the desktop draft stayed selected at `13..13`
  while three rows became selected; the mobile draft stayed at `15..15`.
- Verification passed:
  - `cd apps/web && pnpm e2e:run tests/task/file-tree-create.spec.ts -- --grep "select-all"`
  - `cd apps/web && pnpm e2e:run --no-build tests/file-tree-multi-select.spec.ts -- --grep "select-all"`
  - `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-file-viewer.spec.ts -- --grep "select-all"`
  - `cd apps/web && pnpm run typecheck`
