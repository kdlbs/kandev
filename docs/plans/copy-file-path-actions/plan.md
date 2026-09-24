---
created: 2026-09-24
status: implemented
requirements:
  - REQ-UI-COPY-FILE-PATH-ACTIONS-001
system_design:
  - ../../specs/ui/system-design/copy-file-path-actions.md
legacy_specs: []
---

# Implementation Plan: Copy File Path Actions

## Overview

Add Copy path to working-tree rows in the task Changes panel and replace Copy
diff with Copy path in the Review diff toolbar. Implement the two surfaces
together so they use the same repository-relative path meaning and clipboard
behavior. Refuse to copy paths containing C0 or DEL ASCII control characters
and report that outcome to prevent terminal input injection.

## Scope

### In scope

- Add Copy path to desktop hover and keyboard-focus actions for Changes rows.
- Add Copy path to the existing touch action menu for Changes rows.
- Replace Copy diff with Copy path in desktop and phone Review diff file actions.
- Reuse `copyToClipboard` for safe paths and the existing `task:copyPath`
  translation.
- Reject C0 and DEL ASCII control characters from every Copy path entry point
  and show localized refusal feedback.
- Add focused unit and desktop/phone E2E coverage for the copied value and
  action reachability.

### Out of scope

- Copy diff controls in task diff toolbars, Monaco, or other editor surfaces.
- Path normalization, repository data, Git behavior, APIs, or persistence.
- Public documentation changes.

## Technical approach

- Extend `FileRowActions` and `TouchFileRowActions` with
  the same copy handler for `ChangedFile.path`. Stop propagation so
  copying does not open the diff. Ensure desktop actions become visible on
  keyboard focus as well as hover.
- Replace the Review toolbar's desktop Copy diff button and phone menu item in
  `review-diff-toolbar.tsx`. Copy the current `filePath`,
  including the new path for renamed files.
- Use `copyPathToClipboard` in both surfaces to reject paths with C0 or DEL
  ASCII control characters, then use `copyToClipboard` for safe paths. Use
  `task:copyPath` for localized accessible names and a localized refusal
  message for rejected paths.
- Suppress the shared editor menu's absolute-worktree Copy path item in the
  Review toolbar menus, so the Review surface exposes one Copy path action with
  the repository-relative value. Keep those shared menus unchanged elsewhere.
- Update existing row and Review toolbar unit tests, then extend the existing
  Changes and Review browser scenarios for desktop and phone.

## ASCII UI preview

UI-01: Changes panel working-tree file row, desktop pointer hover or keyboard
focus. The path and statistics stay in the row; Copy path joins the existing
secondary actions. Copy path and row activation are separate controls.

```text
[go] browser/programme_test.go          +12 -12 [M] [undo] [edit] [copy]
```

UI-02: Changes panel working-tree file row, phone. Tapping the row opens the
diff; the visible 44px menu trigger opens a menu whose rows are at least 44px.
Copy path is a menu item, not a hover action.

```text
[go] programme_test.go                  [+12 -12 M] [more]
     go/internal/.../browser

Menu: Full path
      Copy path
      Stage / Unstage
      Edit
      Discard
```

UI-03: Review diff file toolbar, desktop. Copy path replaces Copy diff and
copies the current repository-relative path.

```text
[comment] path/to/programme_test.go +12/-12 [copy path] [external] [view] [edit] [...]
```

UI-04: Review diff file toolbar, phone. The existing 44px action trigger opens
the file menu; Copy path replaces Copy diff there. The diff keeps its existing
single content scroll region.

```text
Diff header: .../path/to/programme_test.go [status]
[more]

Menu: Copy path
      Edit
      Preview / diff controls
```

UI-05: If either surface receives a path containing a C0 or DEL ASCII control
character, Copy path leaves the clipboard unchanged and shows a localized
refusal notice. The notice does not change the row or active diff.

```text
Copy path: refused
Notice: This path contains control characters and cannot be copied.
```

The drawings show action order, access, and hierarchy. Icon appearance and
spacing are illustrative. UI-01 through UI-04 map to
`AC-UI-COPY-FILE-PATH-ACTIONS-001.1` through `.4`; the
targeted browser checks prove the copied value and desktop/phone access.
UI-05 maps to `AC-UI-COPY-FILE-PATH-ACTIONS-001.6`.

## Tests

| Acceptance criterion                             | Evidence                                                                                                                     |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.1`, `.5`       | `components/task/changes-panel-file-row.test.tsx`: desktop and phone tests assert the exact path and no diff navigation      |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.2`, `.3`, `.5` | `components/review/review-diff-toolbar.test.tsx`: desktop copies the current path for a rename; phone copies its active path |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.4`             | Desktop and phone E2E checks below assert keyboard/pointer or menu reachability and 44px phone controls                      |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.6`             | Component suites reject newline, ESC, and DEL across all four entry points; utility tests cover the full C0/DEL range        |

## E2E tests

- `tests/git/git-changes-panel.spec.ts` (chromium):
  `copies a changed file path without opening its diff`.
- `tests/task/mobile-changes-panel.spec.ts` (mobile-chrome):
  `compact file rows preserve names and expose touch actions` also copies from
  the row menu.
- `tests/review/review-file-status.spec.ts` (chromium):
  the Review status scenario copies the new path for a renamed file.
- `tests/review/mobile-review-file-status.spec.ts` (mobile-chrome):
  the mobile header scenario copies the current path from its file menu.

Component tests also verify control-character paths are rejected from all four
desktop and phone entry points with localized refusal feedback.

Each flow asserts the exact repository-relative clipboard value. The phone flows
also assert the relevant menu or control has a 44px hit area and the page has no
horizontal overflow.

## Work orders

- [x] [Task 01: Add copy path actions](task-01-copy-path-actions.md) (done)

## Verification results

- `pnpm exec vitest run components/task/changes-panel-file-row.test.tsx components/review/review-diff-toolbar.test.tsx lib/utils/copy-repository-path.test.ts`: passed (39 tests, including control-character rejection across all four desktop and phone entry points).
- `pnpm e2e:run --host --project chromium tests/git/git-changes-panel.spec.ts tests/review/review-file-status.spec.ts`: passed (28 tests; production builds succeeded).
- `pnpm e2e:run --no-build --host --project mobile-chrome tests/task/mobile-changes-panel.spec.ts tests/review/mobile-review-file-status.spec.ts`: passed (10 tests; using the production bundle built by the preceding managed mobile run).
- After formatting, `pnpm run build` passed, and focused desktop and mobile E2E reruns each passed (2 tests per project).
- Targeted ESLint, Prettier, typecheck, and `pnpm run i18n:ratchet`: passed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- After PR review remediation, the focused desktop Chromium E2E suite passed (28 tests) and the phone mobile-chrome suite passed (10 tests); both managed runs rebuilt the backend and Vite assets.
- After remediation, web typecheck, targeted ESLint and Prettier, `i18n:check`, `i18n:ratchet`, specification validation and lint all passed.

## Risks

- The additional Changes row control must not reduce the visible path or
  overlap the trailing statistics at the panel's supported minimum width.
- Renamed files must copy the current path from the diff header, not the
  previous path retained for rename context.
