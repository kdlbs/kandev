---
created: 2026-11-18
status: implemented
requirements:
  - REQ-TASKS-WORKSPACE-ROOT-FILES-001
  - REQ-TASKS-WORKSPACE-ROOT-FILES-002
  - REQ-TASKS-WORKSPACE-ROOT-FILES-003
system_design:
  - ../../specs/tasks/system-design/workspace-root-files.md
legacy_specs: []
---

# Implementation plan: Show workspace root files in Changes

## Outcome

Changes in a multi-repository task shows an **Outside repositories** section
for regular files that agents create in the shared task workspace root, next to
the checkouts. Users can open those files and explicitly move each one into a
chosen checkout. Stage, Commit, and Push then work as usual. Source:
[kdlbs/kandev#5372](https://github.com/kdlbs/kandev/issues/5372).

## Reproduced cause

| Observation | Cause | Change |
| --- | --- | --- |
| `dashboard.json` at the task root is absent from Changes. | Changes renders only per-repository `GitStatusUpdate`s. The bare root tracker has no Git scope, and it suppresses publication by design (`publishGitStatus`). Each repository's `git ls-files --others` only sees its own checkout. | Add a separate, read-only root-file listing and notification path that never enters Git status. |
| Chat **Open file** works for the same path. | File reads resolve against the root tracker's `workDir`. | Reuse that open path from the new section. |
| No way to put the file under Git from the UI. | No destination flow exists. The root tracker's `RenameFile` already moves files inside the root without overwriting. | Use the existing `workspace.file.rename` with a checkout-prefixed target. |

## Work orders

Execute sequentially. Delegation is not authorized.

- [x] [01: Discover and publish workspace root files](task-01-root-file-discovery.md) (done)
- [x] [02: Show and open workspace root files in Changes](task-02-changes-section.md) (done)
- [x] [03: Move a workspace root file into a checkout](task-03-move-to-repository.md) (done)

## ASCII UI preview

These previews are structural. They fix section placement, grouping, the
control order, and the explicit destination step. Spacing, icons, and exact
copy are illustrative. All copy is localized.

### UI-01: Changes with clean checkouts and one root file (desktop)

Entry point: the Changes panel of a two-checkout Worktree task. State: both
checkouts are clean, and the agent wrote `dashboard.json` at the root.

```text
+-- Changes ------------------------------------------------+
| [toolbar: repo selector | Commit | Push ...]              |
+----------------------------------------------------------+
| v Outside repositories (1)                               |
|     {} dashboard.json                          [ ... ]   |
|                                                 +-------------------------+
|                                                 | Open                    |
|                                                 | Move to repository...   |
|                                                 +-------------------------+
+----------------------------------------------------------+
```

- The section replaces the "no changes" empty state. When checkouts have
  changes, it follows the Unstaged/Staged/history timeline in the same
  scroller.
- A row click opens the file. The row menu has **Open** first, then **Move to
  repository…**.
- A note such as "Showing the first 500 files" appears below the rows only when
  the list is truncated.

### UI-02: Move to repository dialog

```text
+-- Move dashboard.json to a repository -------------------+
| Choose the checkout that should receive this file.       |
| The file is moved, not staged.                           |
|                                                          |
|  ( ) frontend        -> frontend/dashboard.json          |
|  ( ) backend         -> backend/dashboard.json           |
|                                                          |
|                               [ Cancel ]  [ Move ]       |
+----------------------------------------------------------+
```

- No destination is preselected, and **Move** is disabled until the user picks
  one.
- An existing target produces an error toast. The dialog stays open, and the
  file is unchanged.

### UI-03: Phone

```text
+-------------------------------+
| Changes                   [x] |
+-------------------------------+
| v Outside repositories (1)    |
| +---------------------------+ |
| | {} dashboard.json     [⋮] | |  <- 44px touch row
| +---------------------------+ |
+-------------------------------+
| Bottom sheet (row menu):      |
|   Open                        |
|   Move to repository...       |
+-------------------------------+
```

- Uses the shared touch row and the existing DropdownMenu bottom sheet below
  640px. The dialog radio rows are at least 44px. The phone Changes sheet
  mounts the same `ChangesPanelBody`, so the composition is not a separate
  surface.

## Verification mapping

The numeric prefixes below refer to `AC-TASKS-WORKSPACE-ROOT-FILES-*`.

| Criteria | Evidence |
| --- | --- |
| 001.3-.5, 002.2 | Task 01: `workspace_root_files_test.go` covers exclusions, symlinks, nested `.git`, the 500 cap, unsupported roots, and no Git subprocess. |
| 001.2 | Task 01: an observer test emits create/write/remove notifications from tick deltas. Task 02: a hook test refetches on an empty-repository notification. |
| 001.1, 001.5, 002.1 | Task 02: section and hook Vitest tests, plus a desktop and phone E2E test with clean checkouts. |
| 003.1-.3 | Task 03: dialog tests (no default, disabled Move, conflict error) and desktop/phone E2E moves into the second checkout's Unstaged group without staging; repeat moves preserve both source and destination contents. |

## End-to-end evidence

`apps/web/e2e/tests/git/workspace-root-files.spec.ts` and
`apps/web/e2e/tests/git/mobile-workspace-root-files.spec.ts` use the
two-repository fixture pattern from `changes-panel-multi-repo-indent.spec.ts`.
They write `dashboard.json` into the task root, assert that the section appears,
open the file, and move it into the second repository. They then assert that
the file appears in that repository's Unstaged group with no staged changes.
Each flow recreates the root file and confirms a second move reports the
existing target without changing either file.

## Risks

- **Large roots:** the walk skips checkouts, dot-directories, and
  `node_modules`, and stops at 500 files. Runaway agent output at the root is
  therefore bounded per tick.
- **Agent tool directories:** `.claude/` and `.codex/` are hidden by the
  dot-directory rule, by design. Dot-files at the root, such as `.env`, still
  appear.
- **Destination identity:** the frontend must use the server-supplied
  `destinations[].path`, never a display label, for the target prefix.
  Adopted linked worktrees can be nested paths.
- **Exclusions:** copy actions, bulk moves, diffs, and single-repository tasks
  are out of scope, as stated in the requirements.

## Status

Implemented. All three work orders are complete. The backend and Vite builds,
focused frontend tests, typecheck, lint, i18n checks, and desktop/phone E2E flows
passed. The root file is moved into the selected checkout without staging, and
an existing target leaves both files unchanged.
