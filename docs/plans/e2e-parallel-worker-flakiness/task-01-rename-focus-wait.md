---
id: "01-rename-focus-wait"
title: "Deterministic rename focus handoff"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/e2e-parallel-worker-flakiness/spec.md"
---

# Task 01: Deterministic rename focus handoff

## Acceptance

- Rename sets a pending action; `ContextMenuContent.onCloseAutoFocus` prevents
  default trigger restoration only for that action and invokes edit mode.
- `TreeNodeName` immediately focuses and selects the mounted textbox, while
  preserving the 400 ms blur safeguard.
- The E2E helper retains visible and focus assertions without an extended focus
  timeout. The focused component regression verifies the close handoff without
  advancing a 150 ms timer.

## Verification

```sh
cd apps/web
pnpm run build:e2e
pnpm run e2e:raw tests/task/file-tree-rename.spec.ts --project=chromium --workers=2
pnpm run e2e:raw tests/task tests/kanban tests/gitlab --project=chromium --workers=4
```

Run the second command at least twice; the original finding reproduced the
failure inconsistently across runs (it hit this spec in one run and
`add-workspace-sources.spec.ts` in another).

## Files likely touched

- `apps/web/e2e/tests/task/file-tree-rename.spec.ts` (lines 41-54, the
  `startRenameViaContextMenu` helper)

## Dependencies

None.

## Parallelism

Sequential: establishes the shared focus lifecycle before Task 02.

## Inputs

- Root cause in `plan.md` and the spec's "Broken behavior" section.
- `apps/web/components/task/file-context-menu.tsx` — production lifecycle target.
- `apps/web/components/task/file-context-menu.test.tsx` — focused regression.
- `apps/web/e2e/tests/task/file-tree-rename.spec.ts` — user-visible confirmation.

## Output contract

Report the deterministic lifecycle handoff, focused regression, and the
targeted + 4-worker verification results.

## Results

- `file-context-menu.tsx`: `FileContextMenuSurface` holds `renamePendingRef`;
  `ContextMenuContent.onCloseAutoFocus` calls `preventDefault()` and invokes
  `onStartRename()` only when a rename is pending, otherwise leaves Radix's
  default focus restoration untouched. `TreeNodeName`'s rename effect now
  focuses/selects the input synchronously instead of via a 150ms `setTimeout`;
  the 400ms blur-enable timer is unchanged.
- `file-context-menu.test.tsx`: added `RenameHarness` + a regression that opens
  the menu, clicks Rename, advances fake timers by 0ms (proving no reliance on
  the removed 150ms timer), and asserts the textbox is `document.activeElement`
  with the full filename selected. 9/9 tests pass.
- `file-tree-rename.spec.ts`: `startRenameViaContextMenu` now asserts
  `toBeFocused()` with the default timeout (no 5s extension). 4/4 tests pass
  standalone; 4/4 pass inside both loaded 4-worker runs (see plan.md Results).
