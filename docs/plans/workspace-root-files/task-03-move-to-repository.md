---
id: "03-move-to-repository"
title: "Move a workspace root file into a checkout"
status: done
wave: 3
depends_on:
  - "02-changes-section"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKSPACE-ROOT-FILES-003
acceptance_criteria:
  - AC-TASKS-WORKSPACE-ROOT-FILES-003.1
  - AC-TASKS-WORKSPACE-ROOT-FILES-003.2
  - AC-TASKS-WORKSPACE-ROOT-FILES-003.3
system_design:
  - ../../specs/tasks/system-design/workspace-root-files.md
---

# Task 03: Move a workspace root file into a checkout

## Summary

Add **Move to repository…** to workspace root file rows. A dialog lists the
server-supplied checkout destinations, with none selected by default. A
confirmed move calls the existing `workspace.file.rename` without `repo`, and
the file then appears untracked in that checkout. Nothing is staged.

## In scope

- New `components/task/changes-panel-move-root-file-dialog.tsx`. It uses the
  `@kandev/ui` Dialog and RadioGroup (as the commit dialog does), shows a
  per-destination target preview, and keeps **Move** disabled until the user
  selects a destination. Radio rows are at least 44px on coarse pointers.
- A move hook or handler calls
  `renameFile(client, sessionId, path, `${destination.path}/${path}`)`. On
  success it closes the dialog. On failure it shows a localized toast with the
  backend message and keeps the dialog open.
- The row menu item in desktop and touch rows.
- Localized copy in all seven locales.

## Out of scope

Copy, overwrite confirmation, bulk or directory moves, and automatic staging.

## ASCII UI preview

See [UI-02 and UI-03 in the plan](plan.md#ascii-ui-preview).

## Acceptance

1. Dialog tests: there is no default selection and **Move** is disabled. After
   a selection, **Move** calls `renameFile` with
   `<destination.path>/<path>` and no repo. A rejection keeps the dialog open
   and shows the error toast.
2. E2E on desktop and `mobile-chrome`: move `dashboard.json` into the second
   repository. The Outside repositories section disappears, the file appears
   under that repository's Unstaged group, and nothing is staged. A second
   move attempt onto an existing target shows the error, and the root file is
   unchanged.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/changes-panel-move-root-file-dialog.test.tsx components/task/changes-panel-root-files.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/git/workspace-root-files.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-workspace-root-files.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- New `apps/web/components/task/changes-panel-move-root-file-dialog.tsx` and test
- `apps/web/components/task/changes-panel-root-files.tsx`
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/git/workspace-root-files.spec.ts` and
  `mobile-workspace-root-files.spec.ts`

## Dependencies

Task 02.

## Risks

- Use `destinations[].path`, not the repository display name, as the prefix.
- `RenameFile` creates parent directories in the destination checkout for
  nested root paths (`notes/a.md` becomes `<checkout>/notes/a.md`). That is
  intended.

## Parallelism

`sequential`

## Results

- `ChangesPanelMoveRootFileDialog` has no preselected destination, shows the
  checkout-relative target path, and keeps **Move** disabled until selection.
  The rename uses the selected destination path without a repository scope;
  failed moves show the backend error and leave the dialog open.
- Localized all new copy in seven locales and regenerated Traditional Chinese
  and pseudo-locale catalogs.
- Focused Vitest passed: 49 tests across seven files. Typecheck, lint,
  `i18n:check`, and `i18n:ratchet` passed.
- Desktop and mobile E2E each passed. Both moved `dashboard.json` into the
  second repository as unstaged, then confirmed a conflicting repeat move left
  the recreated root file and destination file unchanged. Each managed run
  built the backend and Vite assets.
- Review regressions passed for rejecting overwrite races, not offering
  symlinked checkout destinations, binding dialog state to its session, and
  ignoring dismissal while a move is pending. Focused Vitest passed (26 tests
  across four files), and desktop and phone E2E each passed.
- Passed the specification validation, spec lint, and diff whitespace checks.
