---
created: 2026-09-26
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001
system_design:
  - ../../specs/integrations/system-design/github-pr-unlink-menus.md
legacy_specs: []
---

# Implementation Plan: GitHub PR unlink menus

## Overview

Add a direct `Edit` context menu to the GitHub PR top-bar control, then add
association-specific unlink entries to the Kanban card and sidebar row menus.
Reuse the persisted association unlink contract. Implement the shared mutation
and top-bar interaction first so both task menus can consume it.

## Scope

### In scope

- Single- and multi-PR GitHub associations in the top bar, sidebar task row,
  and Kanban card menus.
- Desktop right-click and visible dots controls on sidebar rows and cards.
- Scoped state, pending and error feedback, localization, accessibility,
  browser coverage, and the existing public review guide.

### Out of scope

- GitLab MR and plugin-owned change-request menus.
- GitHub PR or branch deletion, bulk unlink, and a new confirmation dialog.

## Technical approach

### Shared association action

Reuse `useTaskPR.unlink` and the existing
`DELETE /api/v1/github/task-prs/:associationId` path. If the card needs the
mutation without subscribing every mounted card to `github.task_pr.sync`,
extract a small scoped unlink action from `useTaskPR`. Keep the existing
`removeTaskPR` and resource invalidation behavior, success/error toast, and
per-association pending guard. The backend API, event payload, and database
schema remain unchanged. The status-summary projector consumes the existing
deletion event and reloads active task PR associations so compact indicators
clear after the final unlink.

### Top-bar shortcut

Add a Radix context menu around the `PRTopbarButton` trigger. Use `Edit` as the
root submenu and label each child by repository and PR number when several
associations exist. The selected child's persisted `TaskPR.id` is the mutation
target. Close the hover CI popover while the context menu is open, preserve
single-click review and multi-click selector behavior, and allow final-link
removal to unmount the trigger.

### Card shortcut

Read `getTaskPRsForCurrentWorkspace` from the task's scoped store. On menu
opening, reuse task-scoped PR hydration when a compact summary reports a PR
but exact rows are absent. Extend `buildEditMenuEntry` with native unlink
children while preserving plugin edit contributions and the flat Edit item
for tasks without contributions. `useKanbanCardMenus` feeds the same entries
to desktop context and visible dots menus. Multi-selection does not change
the target: the opened card's task and selected PR identify the action.

### Sidebar shortcut

`TaskItemWithContextMenu` and `SingleEditGroup` own the separate sidebar row
menu pictured in the request. Keep `Rename` and `Duplicate` at root level.
Wrap the existing `Edit` action in a submenu only when exact PR choices exist
or are loading. Use the row task ID, a scoped association read, and on-open
hydration; hide unlink in the reduced bulk-selection menu. The row dots button
opens the same context menu on phone.

### Documentation

Update the how-to section in `docs/public/sessions-and-review.md` that
already explains PR detachment. State the new top-bar, sidebar, and card menu
paths, the multi-PR choice, and that GitHub itself is unchanged.

## ASCII UI preview

`UI-01: Desktop top-bar PR control; right-click; one linked PR`

```text
Task top bar       [ PR #3943 ]
                       right-click
                    +--------------------------+
                    | Edit                   >  |
                    +--------------------------+
                       +-----------------------+
                       | Unlink pull request   |
                       | #3943                 |
                       +-----------------------+
```

`UI-02: Desktop card; right-click or dots; two linked PRs`

```text
+----------------------------------+
| Task title  [PR 2]            ... |
+----------------------------------+
  context/dots menu       Edit >
                         +-------------------------------+
                         | Edit task                     |
                         | Unlink acme/web #3943        |
                         | Unlink acme/api #3943        |
                         | (existing plugin actions)     |
                         +-------------------------------+
```

`UI-03: Phone card; visible dots; two linked PRs`

```text
+-------------------------------+
| Task title  [PR 2]        ...  |
+-------------------------------+
                            tap dots
   +-------------------------------+
   | Edit                        > |
   +-------------------------------+
   | Edit task                     |
   | Unlink acme/web #3943        |
   | Unlink acme/api #3943        |
   +-------------------------------+
```

`UI-04: Sidebar task row; right-click or visible dots; one linked PR`

```text
Task title [PR]                       ...
  context/dots menu    Edit > Edit task
                             Unlink pull request #3943
                       Rename
                       Duplicate
```

`UI-05: Phone sidebar task row; visible dots; one linked PR`

```text
Task title [PR]                   [ ... ]
                                tap dots
   +-----------------------------------+
   | Edit                            >  |
   +-----------------------------------+
   | Edit task                         |
   | Unlink pull request #3943        |
   +-----------------------------------+
```

The phone sidebar menu rises from the bottom and owns its vertical scroll.

Card and sidebar row bodies remain primary tap destinations. The dots targets
and phone menu rows are at least 44 pixels. The phone menu owns its overflow;
the page does not scroll horizontally. If exact PR records are pending, the
`Edit` submenu shows a disabled `Loading pull requests...` row. The structure
and order are required; spacing and borders are illustrative. Copy uses locale
keys. These views map to `AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.1` through
`.7`.

## Tests

- `apps/web/components/github/pr-topbar-button.test.tsx` or the nearest PR
  top-bar test: right-click menu, exact association selection, overlay
  coexistence, pending and failure behavior (`AC-001.1`, `.3`, `.5`).
- `apps/web/components/kanban-card-edit-submenu.test.tsx` and
  `kanban-card-menu-items.test.tsx`: flat versus submenu behavior, exact rows,
  duplicate numbers, plugin entries, disabled loading, and both menu variants
  (`AC-001.2`, `.3`, `.6`, `.7`).
- `apps/web/components/task/task-switcher-context-menu.test.tsx`: sidebar
  Edit group, row-scoped target, multi-selection exclusion, and on-open
  hydration (`AC-001.2`, `.3`, `.6`, `.7`).
- Existing `use-task-pr.test.tsx` remains the mutation contract regression for
  HTTP success and failure, scoped state, and stale sync (`AC-001.4`, `.5`).

## E2E tests

- `apps/web/e2e/tests/pr/pr-topbar-unlink-context-menu.spec.ts` in desktop
  Chrome: unlink one PR from the top bar, verify sibling preservation and state
  after reload (`AC-001.1`, `.3` to `.5`).
- `apps/web/e2e/tests/kanban/pr-unlink-card-menu.spec.ts` in desktop Chrome:
  unlink one PR from a card context menu and verify the exact association
  remains absent after reload (`AC-001.2` to `.5`).
- `apps/web/e2e/tests/kanban/mobile-pr-unlink-card-menu.spec.ts` in
  `mobile-chrome`: use visible card dots, verify exact PR removed, 44-pixel
  targets and no page overflow (`AC-001.2`, `.3`, `.4`, `.7`).
- `apps/web/e2e/tests/task/mobile-pr-unlink-sidebar-menu.spec.ts` in
  `mobile-chrome`: use visible row dots to unlink its own PR without navigating
  to the task (`AC-001.2`, `.4`, `.7`).
- `apps/web/e2e/tests/task/pr-unlink-sidebar-menu.spec.ts` in desktop Chrome:
  use the sidebar row context menu, verify sibling preservation and reload
  persistence (`AC-001.2` to `.5`).

## Work orders

- [x] [Task 01: Top-bar unlink menu and shared action](task-01-topbar-unlink-menu.md)
- [x] [Task 02: Card unlink menu and phone path](task-02-card-unlink-menu.md)
- [x] [Task 03: Sidebar row unlink menu and guide](task-03-sidebar-unlink-menu.md)

Task 02 depends on Task 01. Task 03 depends on the shared action from Task 01
and follows Task 02 for integrated browser coverage. Execute sequentially in
the primary session.

## Verification results

- Task 01: 24 targeted Vitest tests passed; frontend typecheck passed; desktop
  top-bar unlink E2E passed.
- Task 02: 48 focused Vitest tests passed; frontend typecheck and i18n checks
  passed; desktop and mobile card unlink E2E passed after production build.
- Task 03: 108 focused Vitest tests passed across nine files; frontend
  typecheck and i18n checks passed; production Vite build passed; desktop
  top-bar, card, and sidebar E2E tests passed; mobile card and sidebar E2E tests
  passed; public-doc tests passed (62) and all 47 published pages validated.
- Review follow-up: 144 focused Vitest tests passed across eleven files; the
  status-summary package tests, frontend typecheck, i18n check and ratchet,
  desktop and mobile E2E suites, specification validation, public-doc tests,
  and diff check passed. The E2E cases cover sibling preservation and persisted
  unlink; the PR fixup follow-up below adds final-link indicator coverage.
- PR fixup follow-up: focused UI tests cover preserving a compact indicator
  while a deletion tombstone cannot establish that the final association was
  removed, and hiding it after the authoritative summary drops the final PR.
  Mutation tests cover one shared pending guard across the top bar and task
  menus, workspace-switch isolation, and localized workspace errors. The
  public guide already documents the desktop top-bar right-click path.
  Validation passed:

  ```bash
  (cd apps/web && pnpm exec vitest run components/github/pr-task-icon.render.test.tsx hooks/domains/github/use-task-pr.test.tsx hooks/domains/github/use-task-pr-unlink.test.tsx hooks/domains/github/use-task-pr-unlink-menu.test.tsx hooks/domains/github/task-pr-mutations.test.ts) # 47 tests
  (cd apps/backend && go test ./internal/task/statussummary)
  (cd apps/web && pnpm run typecheck)
  (cd apps/web && pnpm run build:vite)
  (cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
  (cd apps/web && pnpm exec eslint components/github/multi-pr-ci-popover.tsx components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.tsx hooks/domains/github/task-pr-mutations.ts hooks/domains/github/task-pr-mutations.test.ts hooks/domains/github/task-pr-unlink-registry.ts hooks/domains/github/use-task-pr-unlink.test.tsx hooks/domains/github/use-task-pr-unlink.ts hooks/domains/github/use-task-pr.ts hooks/domains/github/use-task-pr.test.tsx)
  git diff --check
  ```
- Targeted ESLint found no code-quality errors. One existing max-lines warning
  remains in `task-switcher-context-menu.test.tsx`, which already exceeded the
  configured file limit before this work.

## Risks

- The top-bar trigger already hosts a hover popover and multi-PR dropdown;
  context-menu composition must not create conflicting overlays or nested
  interactive elements.
- Card summary data has no association ID. An early or stale menu must wait for
  scoped exact rows and cannot unlink by PR number.
- Removing the final association unmounts the PR control, so focus return must
  tolerate a missing trigger.
