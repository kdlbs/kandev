---
id: "01-relocate-folder-action"
title: "Relocate folder action into editor dropdown"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-OPEN-FOLDER-001
acceptance_criteria:
  - AC-TASKS-OPEN-FOLDER-001.1
  - AC-TASKS-OPEN-FOLDER-001.2
  - AC-TASKS-OPEN-FOLDER-001.3
  - AC-TASKS-OPEN-FOLDER-001.4
  - AC-TASKS-OPEN-FOLDER-001.5
system_design:
  - ../../specs/tasks/system-design/open-task-folder.md
---

# Task 01: Relocate folder action

## Summary

Replace the separate folder button with a folder item in the existing editor
menu. Reuse native opening and the responsive repository picker.

## In scope

Menu wiring/focus, removal of unused button, migrated and expanded regression
coverage, public usage copy and targeted verification.

## Out of scope

Backend/API changes, new native integrations, editor preferences and mobile
navigation redesign.

## Acceptance

- Folder opening is reachable through the editor dropdown with zero configured editors; no separate toolbar folder button remains.
- Existing worktree, unavailable/busy, failure/retry, cancellation and editor behavior pass focused regression tests, including phone Files access.
- Browser evidence matches the previews and public documentation names the new entry point.

## ASCII UI preview

See [combined preview](plan.md#ascii-ui-preview).

### UI-01: Desktop editor dropdown, expanded

Before: `[Open in editor | v] [Folder]`

```text
[Open in editor | v]
                 +------------------------+
                 | Existing editor entries|
                 |------------------------|
                 | Open folder            |
                 +------------------------+
                   -> Choose a folder (multiple worktrees only)
```

### UI-02: Phone Files workspace actions, expanded

```text
[Files                         ...]
       [Add repositories          ]
       [Open workspace folder     ]
          -> [Choose a folder     ]
             [repo-a / branch-a   ]
             [repo-b / branch-b   ]
```

Structural requirements: no standalone toolbar folder button; folder row follows
editor choices, independently enabled. No editors: disabled editor primary action
and no-editors row, usable dropdown and available folder action. No session:
dropdown disabled. Unknown/unavailable opener or pending request: folder row disabled.
Opening has a localized live status; errors produce a retryable localized toast.
Desktop controls remain 28px. Phone retains its visible Files trigger and existing
inset menu/picker, 44px targets, one internal scroller and safe-area spacing; no
horizontal page overflow. This temporary choice uses the shipped
`MobilePickerSheet` exemplar. Menu closes before picker opens; dismissal restores
the persistent trigger. Icons and spacing are illustrative.
Maps to AC-TASKS-OPEN-FOLDER-001.1 through .4 and desktop/mobile folder E2E suites.

## Verification

Use TDD: migrate/add failing regression tests before changing production files.
Install dependencies from `apps/` if missing. The managed E2E runner builds
the backend, Vite assets and plugin fixtures. Ensure Go is on PATH and point
PLAYWRIGHT_BROWSERS_PATH at an installed browser cache. Run from repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/editors-menu.test.tsx components/task/editors-menu-availability.test.ts components/task/editor-worktree-options.test.ts components/task/task-top-bar.test.tsx hooks/use-open-session-folder.test.ts components/task/file-browser-toolbar.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/task/open-task-folder.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-open-task-folder.spec.ts)
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/editors-menu.tsx`, new `editor-actions-dropdown.tsx`, and `editors-menu.test.tsx`.
- `apps/web/components/task/task-top-bar.tsx` and its test.
- Remove `apps/web/components/task/open-task-folder-button.tsx` and migrate its test coverage before deleting its test file.
- `apps/web/e2e/tests/task/open-task-folder.spec.ts`; retain `mobile-open-task-folder.spec.ts`, updating only if regression evidence requires it.
- `docs/public/developer-tools.md`; locale catalogs only if existing localized copy cannot be reused.

## Dependencies

None. Shared `useTaskFolderAction`, `TaskFolderPicker`, and folder transport are shipped.

## Risks

Avoid focusing removed menu items or gating folder access on editor configuration.
Preserve independent editor/folder loading and per-session request suppression.

## Parallelism

`sequential`

## Inputs

[Requirements](../../specs/tasks/requirements/open-task-folder.md),
[design](../../specs/tasks/system-design/open-task-folder.md),
`WorkspaceActionsMenu` close-to-picker pattern in `file-browser-toolbar.tsx`,
and existing folder button/unit and desktop/phone E2E tests.

## Results

Implemented on 2026-09-23.

- RED: the migrated test failed because the editor dropdown was disabled with zero
  editors; the remaining folder-menu regressions failed because its folder entry
  did not exist. The error/retry regression also failed before implementation.
- GREEN: `pnpm exec vitest run` with the six suites listed above passed 49 tests.
  All eight original folder-button behaviors are retained in the migrated suite,
  plus error/retry and independent editor-launch coverage.
- `NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck` passed. The default
  2 GiB heap exhausted memory; rerunning with 4 GiB resolved that environment limit.
- Changed-file ESLint passed for both editor components, the migrated test,
  task topbar and desktop E2E spec.
- Managed E2E built the Go runtime/helpers, production Vite assets and plugin
  fixture. Desktop folder suite passed 3/3; unchanged phone folder suite passed
  2/2. Final runs reused those fresh artifacts with `--no-build`, Go on PATH,
  and `PLAYWRIGHT_BROWSERS_PATH=/tmp/kandev-folder-browsers`.
- `node scripts/validate-public-docs.mjs` passed (47 pages), and its test file
  passed 62 tests. Spec catalog validation (299 decisions, 1108 specifications),
  full spec lint and diff whitespace checks passed.

### Localization baseline exception

`pnpm run i18n:check` fails on 32 missing Japanese keys: 22 `executors:sshReachability*`
and 10 `task:launchWarning*` entries. `git diff --quiet HEAD -- apps/web/src/locales`
confirms all catalogs are unchanged. This change reuses existing localized copy.
The remaining five i18n checks were run directly and passed: Trans indices,
inline plurals, module-scope translations, em dashes, and non-JSX copy.
This unrelated catalog failure is retained as an explicit validation exception.

### Browser evidence and limits

Screenshots retained in `/tmp/editor-folder-evidence/`: desktop folder menu,
desktop repository picker and mobile repository picker. Phone evidence shows
an inset sheet, contained 44px rows and no horizontal overflow. Desktop tests
verify the removed button, 28px dropdown trigger, menu reachability without
editors, selected-worktree payload, keyboard use, cancellation focus and retry.
Native opening is HTTP-stubbed; these tests do not assert Finder/Explorer window
visibility. No backend native-launch behavior changed.

## PR review remediation (2026-09-24)

Claude's inline finding and summary suggestion identified the editor-only
accessible name on a menu that also opens folders. The trigger now announces
"Editor and folder actions" through `task:editorActions` in all six locales and
the pseudo locale; Traditional Chinese values use the repository converter.
The new focused regression failed before the change.

Post-fix checks: `pnpm exec vitest run components/task/editors-menu.test.tsx`
passes 11 tests; changed-file ESLint passes; a fresh managed production build and
`pnpm e2e:run --host tests/task/open-task-folder.spec.ts` pass all 3 browser tests,
including the accessible-name assertion. The local full i18n check still reports
the previously recorded Japanese keys, which are already fixed on current main;
none of its findings concern the new label.
