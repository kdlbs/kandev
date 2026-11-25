---
id: "01-share-changes-tree"
title: "Share the Changes tree"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKSPACE-ROOT-FILES-001
  - REQ-TASKS-WORKSPACE-ROOT-FILES-002
  - REQ-TASKS-WORKSPACE-ROOT-FILES-003
acceptance_criteria:
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.1
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.4
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.5
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.6
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.7
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.8
  - AC-TASKS-WORKSPACE-ROOT-FILES-002.1
  - AC-TASKS-WORKSPACE-ROOT-FILES-003.1
  - AC-TASKS-WORKSPACE-ROOT-FILES-003.2
  - AC-TASKS-WORKSPACE-ROOT-FILES-003.3
system_design:
  - ../../specs/tasks/system-design/workspace-root-files.md
---

# Task 01 Share the Changes tree

## Summary

Render **Outside repositories** with the same disclosure and file tree
presentation as Git changes. Keep Open and Move to repository available for
workspace root files on desktop and phone.

## In scope

- Share the header button with `ChangesRepositoryTimelineRow`. Preserve the
  button's DOM props for the Git repository context menu and row navigation.
- Reuse `buildChangesTree`, `flattenChangesTree`, and `TreeDirRow` with the
  original file entries. Share the file icon and name presentation with
  desktop and touch Git rows. Follow `userSettings.changesPanelLayout`.
- Keep group and folder state with `ChangesPanelRootFiles`. Reset it with the
  Changes context. Keep the group inside the existing panel scroller.
- Update the **Files outside repositories** section in `docs/public/git-operations.md`.
  Describe the chevron, Tree and List layouts, and the row menu.

## Out of scope

The workspace listing and move protocols, file discovery, settings storage,
and Git operations.

## Acceptance

1. With clean or dirty checkouts, Changes shows the group when outside files
   exist. The header retains its count when collapsed. A refresh in the same
   context preserves group and folder choices.
2. Tree and List match the Git file typography. Tree uses compact, collapsible
   folders. On phones and with coarse pointers, the changed controls meet the
   44px hit target and fit inside the scroll area.
3. A nested row opens its exact root-relative file. Its overflow can move the
   file to a selected checkout as an untracked file. An occupied target still
   reports a conflict and preserves both files.

## ASCII UI preview

See [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview) for the full views.
These excerpts cover `AC-TASKS-WORKSPACE-ROOT-FILES-001.6` through `.8`.

```text
UI-01 Desktop, Tree layout          Group collapsed
v OUTSIDE REPOSITORIES 2            > OUTSIDE REPOSITORIES 2
  v reports/daily
      summary.md            [...]

UI-02 Phone, Changes tab
v OUTSIDE REPOSITORIES 2
  v reports/daily
      summary.md             [...]
```

On the phone, tap the file to open the viewer. The visible overflow button
opens the existing menu in its bottom-sheet form below 640px.

## Verification

Run the component tests in `apps/web`. Run the managed browser commands in
order so each uses its own isolated backend. Use the nested-file fixture for
the rendered desktop and phone checks.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run test \
  components/task/changes-file-tree-model.test.ts \
  components/task/changes-panel-root-files.test.tsx \
  components/task/changes-panel-file-row.test.tsx \
  components/task/changes-panel-body-context.test.tsx \
  components/task/changes-timeline-working-tree.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium \
  tests/git/workspace-root-files.spec.ts tests/git/changes-panel-multi-repo-indent.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome \
  tests/git/mobile-workspace-root-files.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

On desktop, compare header alignment and computed typography with a Git group
in the same panel. Check keyboard activation and matched file typography in
Tree and List. At a narrow fine-pointer viewport and in `mobile-chrome`, check
group and folder hit areas. In `mobile-chrome`, check that a long file name
fits beside the overflow button. The panel owns vertical scrolling and the
document fits the viewport width. Check the visible menu and file viewer.

## Files likely touched

- `apps/web/components/task/changes-panel-root-files.tsx` and its test.
- `apps/web/components/task/changes-panel-body.tsx` for Changes context.
- `apps/web/components/task/changes-file-tree-model.ts` and its test.
- `apps/web/components/task/changes-panel-tree.tsx` and `changes-timeline-working-tree.tsx`.
- `apps/web/components/task/changes-panel-file-row.tsx` and `changes-panel-touch-file-row.tsx`.
- `apps/web/e2e/tests/git/workspace-root-files.spec.ts`, `mobile-workspace-root-files.spec.ts`, and `workspace-root-files.helpers.ts`.
- `docs/public/git-operations.md`.

## Dependencies

None. The existing listing hook, workspace file actions, and Changes layout
preference supply the inputs.

## Risks

Shared presentation changes can shift the Git rows. Keep the current desktop
alignment and run the multi-repository indentation check. Use full paths for
keys and actions to distinguish files with the same name.

## Parallelism

`sequential`

## Results

Implemented the shared Changes tree for workspace root files and addressed all
review findings: forwarded native header props and refs for Git context menus;
capped phone indentation and verified a 32-level branched tree; preserved
collapsed folder boundaries across refreshed listings; separated directory
and file React keys; added nested move/conflict and phone disclosure checks;
and kept original `WorkspaceRootFile` records in the generic tree model.

Focused component tests passed (52 tests across 7 files), as did web typecheck
and lint. Desktop browser checks passed (4 tests); the phone root-file browser
flow and Git worktree context-menu browser check passed. Public-doc validation
passed (63 tests and 56 pages); specification validation and lint passed.
`git diff --check` passed.

Follow-up review remediation caps fine-pointer root-file indentation, adds a
desktop deep-path visibility and action check, updates the 767px folder-row
target assertion, and splits the public move guidance into short steps. The
focused desktop E2E, component tests, web typecheck and lint, public-doc
validation, and specification validation passed.
