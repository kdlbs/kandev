---
created: 2026-09-22
status: implemented
requirements:
  - REQ-TASKS-OPEN-FOLDER-001
system_design:
  - ../../specs/tasks/system-design/open-task-folder.md
legacy_specs: []
---

# Implementation plan: Editor dropdown folder action

## Overview

One sequential work order relocates the standalone button added in PR #3784
(commit `32d640f26`) into the usual editor dropdown. Estimate: 45-90 minutes
including focused browser verification. Tasks owns this capability because the
selected task session determines the target workspace.

Confirmed intent: remove the added standalone control and expose folder opening
inside the editor dropdown. Keep existing native opening, host availability,
worktree selection, and Files-menu access. This is a targeted UI relocation, not
a wholesale revert of PR #3784. No unresolved product decisions.

## Scope

In scope: dropdown composition and focus, removal of unused button, migration of
its regression tests, desktop browser flow, phone regression and public usage copy.
Out of scope: backend changes, editor preferences, archived-task visibility,
remote mounting, and redesigning phone task navigation.

## Technical approach

Follow the [design](../../specs/tasks/system-design/open-task-folder.md).
Reuse the existing folder action and responsive picker. Decouple dropdown
availability from enabled-editor count while preserving primary editor behavior.
Extract a focused presentation component only if needed for component limits.
Update `docs/public/developer-tools.md` with the new menu path during implementation;
leave published instructions matching shipped behavior during this design turn.
No ADR: the existing ownership, transport and native-launch boundaries remain.

## ASCII UI preview

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

## Tests

Migrate `open-task-folder-button.test.tsx` coverage into `editors-menu.test.tsx`
before removing the old suite. Add failing tests for placement/removal, zero
editors, keyboard activation, host unavailability, busy state, errors/retry,
single/multiple worktrees, cancellation/focus restoration and session changes.
Preserve editor launching/default behavior. Use the hook suite for duplicate
suppression and payload failures (AC .1-.3, .5). Update all three desktop folder
browser scenarios to enter through the dropdown, add zero-editor coverage,
and assert absence of the separate control. Retain phone folder scenarios for
selection and unavailable hosts, with touch geometry and no-overflow checks
(AC .4). Stub native launch HTTP responses; do not open the host GUI in CI.
Inspect desktop and phone screenshots against UI-01/UI-02.

## Work orders

- [x] [Task 01: Relocate folder action](task-01-relocate-folder-action.md)

## Verification commands

From repository root; install workspace dependencies from `apps/` if missing.
The managed E2E runner builds the backend, Vite assets and plugin fixtures;
separate build commands are unnecessary. Ensure Go is on PATH and set
PLAYWRIGHT_BROWSERS_PATH to a writable installed browser cache when required.

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

## Verification results

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

## Risks

The editor-count guard can accidentally hide folder opening. Dropdown focus
restoration can dismiss a newly opened picker unless opening waits for menu close.
The original package records an unrelated source-attachment E2E baseline failure;
this package targets the folder suites. Native window visibility remains outside
HTTP-stubbed browser evidence.
