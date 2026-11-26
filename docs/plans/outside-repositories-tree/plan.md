---
created: 2026-11-23
status: complete
requirements:
  - REQ-TASKS-WORKSPACE-ROOT-FILES-001
  - REQ-TASKS-WORKSPACE-ROOT-FILES-002
  - REQ-TASKS-WORKSPACE-ROOT-FILES-003
system_design:
  - ../../specs/tasks/system-design/workspace-root-files.md
legacy_specs: []
---

# Outside repositories file tree

## Outcome

Users can browse outside files by folder, then collapse the whole group from
its header. The group follows the Changes layout preference on desktop and
phone. The tasks system owns this behavior as part of the
[workspace root files requirements](../../specs/tasks/requirements/workspace-root-files.md).

`ChangesPanelRootFiles` currently renders a static heading and one row per
path. The working tree already has a repository disclosure and a compact file
tree. One work order joins these presentation paths and verifies file actions
end to end.

## Scope

- The group header, count, typography, and disclosure.
- Tree and List layouts, keyboard activation, and touch targets.
- The **Files outside repositories** section in
  [Git operations](../../public/git-operations.md).

## Out of scope

File discovery, the 500-file limit, workspace APIs, review totals, and Git actions.

## Technical approach

Follow the [workspace root files design](../../specs/tasks/system-design/workspace-root-files.md#frontend).
Share the disclosure button, folder row, and file name presentation. Build
the tree from `WorkspaceRootFile` entries with the existing Changes tree model.
Keep the group after the Git timeline in the Changes scroller. A group or
folder toggle only updates local disclosure state. Use open and move callbacks
with the complete root-relative path.

## ASCII UI preview

### UI-01 Desktop Changes group

In a multi-repository task, scroll to the outside files. The current rows use
full paths and have no disclosure control.

```text
Outside repositories                    (2)
  reports/daily/summary.md             [...]
  notes.md                             [...]
```

Proposed Tree layout, expanded and collapsed in the same Changes scroller.

```text
Expanded                             Collapsed
v OUTSIDE REPOSITORIES 2             > OUTSIDE REPOSITORIES 2
  v reports/daily
      summary.md              [...]
    notes.md                  [...]
```

Header alignment and the header and file typography match the repository groups.
Group indentation, nesting, and control order are required. The drawing's
spacing is illustrative.
In List, the same group shows root-relative paths with the regular Changes
file typography. The truncation note goes below the expanded rows when needed.

### UI-02 Phone Changes group

Tap **Changes** in the task navigation. The grouped tree scrolls with the
rest of Changes. The navigation stays fixed below it.

```text
v OUTSIDE REPOSITORIES 2
  v reports/daily
      summary.md             [...]
    notes.md                 [...]
---------------------------------
  Chat       Changes      Files
```

Tap a file to open the mobile file viewer. Tap the visible overflow control
to open the action menu. The group, folder, file, and overflow controls have
at least 44px hit areas. Use the same collapsed header as UI-01.

## Verification mapping

| Criteria | Evidence |
| --- | --- |
| `001.1`, `001.4`–`001.7` | `changes-panel-root-files.test.tsx` covers the count, empty and unsupported listings, the truncation note, disclosure across refreshes, and Tree/List labels. `changes-file-tree-model.test.ts` covers compact folders from root-file entries. |
| `001.6`, `001.7`, `002.1`, `003.1`–`003.3` | `workspace-root-files.spec.ts` checks desktop disclosure, header alignment and typography against Git changes, and open/move with a nested path. `changes-panel-multi-repo-indent.spec.ts` checks Git group alignment. |
| `001.6`–`001.8`, `002.1`, `003.1`–`003.3` | `mobile-workspace-root-files.spec.ts` checks disclosure, touch targets, panel containment, and open/move from the phone Changes view. |

The identifiers in this table use the `AC-TASKS-WORKSPACE-ROOT-FILES-` prefix.
The work order lists the exact test commands.

## Work order

- [x] [Task 01 Share the Changes tree](task-01-share-changes-tree.md) — complete

## Verification results

Implementation and review remediation are complete. Focused component tests
passed (52 tests across 7 files); web typecheck and lint passed. Desktop browser
checks passed (4 tests), the phone root-file flow passed with a 32-level tree,
and the Git worktree context-menu browser check passed. Public-doc validation
passed (63 tests and 56 pages); spec validation and lint passed. `git diff
--check` passed.

Follow-up review remediation caps fine-pointer root-file indentation, verifies
deep desktop file-name and action bounds, updates the narrow viewport row-size
expectation, and splits the public move guidance into short steps. The focused
desktop E2E, component tests, typecheck, lint, and documentation validators
passed.

## Risks

- Two folders can contain files with the same name. Open and Move use the
  full path, even when the row label uses the name alone.
- Very deep paths can consume horizontal space. Root-file indentation is capped
  on desktop and phone, and browser checks cover nested controls.
