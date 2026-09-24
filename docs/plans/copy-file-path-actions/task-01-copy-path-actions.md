---
id: "01-copy-path-actions"
title: "Add copy path actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COPY-FILE-PATH-ACTIONS-001
acceptance_criteria:
  - AC-UI-COPY-FILE-PATH-ACTIONS-001.1
  - AC-UI-COPY-FILE-PATH-ACTIONS-001.2
  - AC-UI-COPY-FILE-PATH-ACTIONS-001.3
  - AC-UI-COPY-FILE-PATH-ACTIONS-001.4
  - AC-UI-COPY-FILE-PATH-ACTIONS-001.5
system_design:
  - ../../specs/ui/system-design/copy-file-path-actions.md
---

# Task 01: Add copy path actions

## Summary

Add Copy path to working-tree file rows in the Changes panel and replace the
Review diff toolbar's Copy diff action with Copy path. Both actions copy the
current repository-relative path through the shared clipboard helper.

## In scope

- Add a Copy path action to desktop Changes row hover and keyboard-focus
  actions.
- Add Copy path to the touch Changes row action menu without changing row-tap
  diff navigation.
- Replace Copy diff in the desktop and phone Review diff file actions.
- Suppress the Review toolbar's existing absolute-worktree Copy path item so
  the repository-relative action is the single Copy path choice there.
- Add focused unit and desktop/phone browser coverage for path value and access.

## Out of scope

- Copy diff actions in task diff, Monaco, and other editor surfaces.
- Path conversion, Git behavior, backend/API changes, and persistence.
- New localization keys or public documentation.

## Acceptance

- Changes row Copy path copies the exact repository-relative path on desktop
  and phone; using it does not open the diff.
- Review diff exposes Copy path in place of Copy diff on desktop and phone and
  copies the active path, including the current path for a rename.
- Phone controls and menu items meet the 44px target requirement, and desktop
  row actions are available by keyboard focus as well as pointer hover.

## ASCII UI preview

UI-01 desktop and UI-02 phone from the
[full preview](plan.md#ascii-ui-preview):

```text
Desktop: [go] browser/programme_test.go  +12 -12 [M] [undo] [edit] [copy]
Phone:   [go] programme_test.go          [+12 -12 M] [more]
         go/internal/.../browser
Menu:    Full path / Copy path / Stage or Unstage / Edit / Discard
```

UI-03 desktop and UI-04 phone from the
[full preview](plan.md#ascii-ui-preview):

```text
Desktop: [comment] path/to/programme_test.go [copy path] [external] [view] [...]
Phone:   Diff header .../path/to/programme_test.go [more]
         Menu: Copy path / Edit / Preview / diff controls
```

AC-001.1 through AC-001.5 require the current repository-relative value,
separate row and copy actions, current rename path, and desktop/phone access.
The full plan preview defines fixed versus scrollable content and each control's
placement.

## Verification

Run from the repository root after `pnpm install --frozen-lockfile` in `apps`:

```bash
(cd apps/web && pnpm exec vitest run components/task/changes-panel-file-row.test.tsx components/review/review-diff-toolbar.test.tsx)
(cd apps/web && pnpm e2e:run --host --project chromium tests/git/git-changes-panel.spec.ts tests/review/review-file-status.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-changes-panel.spec.ts tests/review/mobile-review-file-status.spec.ts)
(cd apps/web && pnpm exec eslint components/task/changes-panel-file-row.tsx components/task/changes-panel-touch-file-row.tsx components/review/review-diff-toolbar.tsx components/task/changes-panel-file-row.test.tsx components/review/review-diff-toolbar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/task/changes-panel-file-row.tsx`
- `apps/web/components/task/changes-panel-touch-file-row.tsx`
- `apps/web/components/task/changes-panel-file-row.test.tsx`
- `apps/web/components/review/review-diff-toolbar.tsx`
- `apps/web/components/review/review-diff-toolbar.test.tsx`
- `apps/web/components/editors/file-actions-dropdown.tsx`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts`
- `apps/web/e2e/tests/review/review-file-status.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-file-status.spec.ts`

## Dependencies

None.

## Risks

Keep the Changes row statistics contained at the minimum panel width. Copy the
current path for renamed files and retain the previous path only as context.

## Parallelism

`sequential`

## Inputs

- [Copy File Path Actions requirements](../../specs/ui/requirements/copy-file-path-actions.md)
- [Copy File Path Actions system design](../../specs/ui/system-design/copy-file-path-actions.md)
- Existing `copyToClipboard`, Changes row, touch action menu, and
  Review file toolbar patterns.

## Results

Implemented desktop and phone Copy path actions for Changes rows, replaced
Copy diff in the Review toolbar, and suppressed the duplicate absolute-path
copy item inside Review file menus.

Verification passed:

- Focused Vitest suites: 25 tests passed.
- Desktop Chromium E2E: 28 tests passed; managed backend and Vite builds passed.
- Mobile Chrome E2E: 10 tests passed after the menu animation settled before
  hit-area measurement.
- After formatting, `pnpm run build` and focused desktop/mobile browser reruns
  passed (2 tests per project).
- Targeted ESLint and Prettier, web typecheck, i18n ratchet, specification
  validation, specification lint, and `git diff --check` passed.
