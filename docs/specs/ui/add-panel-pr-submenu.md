---
status: draft
created: 2026-07-31
owner: Kandev frontend
---

# Task Add-Panel PR Submenu

## Why

The task-view "+" add-panel menu lists every GitHub pull request linked to the
task as a flat menu row. A task can carry up to ten or more linked PRs (e.g.
multi-repo work), which makes the dropdown too tall to use — it spills past the
viewport or requires scrolling through a wall of identical rows.

## What

- The task-view "+" add-panel menu (dockview group header and empty-group
  watermark) keeps its current behavior for tasks with zero or one linked
  GitHub PR: a single PR renders as one inline menu row, and no PR renders
  nothing.
- When a task has more than one linked GitHub PR, the menu shows a single
  **Pull requests** sub-menu trigger instead of inline PR rows.
- Opening the sub-menu reveals one row per linked PR. Each row opens that PR's
  dockview panel when selected, exactly as the current inline row does.
- Per-PR rows inside the sub-menu keep the current disambiguating label used for
  multi-PR tasks (`PR #42 — owner/repo`), while single-PR tasks keep the plain
  `PR #42` label on the inline row.
- The sub-menu trigger carries the same add-panel menu styling as other rows and
  an icon consistent with the PR rows.
- Per-PR rows keep their existing stable test identifiers so automated tests can
  select them from within the sub-menu.
- The sub-menu must work with the existing Radix dropdown primitives on desktop
  (pointer hover and keyboard) and on the sub-640px mobile bottom-sheet
  presentation that `app/globals.css` already applies to nested dropdown menus.

## Scenarios

- **GIVEN** a task with no linked GitHub PRs, **WHEN** the user opens the "+"
  add-panel menu, **THEN** no PR row or Pull requests trigger is shown.
- **GIVEN** a task with exactly one linked GitHub PR, **WHEN** the user opens
  the "+" add-panel menu, **THEN** one inline PR row labeled `PR #N` is shown
  and no Pull requests sub-menu trigger is shown.
- **GIVEN** a task with two linked GitHub PRs, **WHEN** the user opens the "+"
  add-panel menu, **THEN** the menu shows a Pull requests sub-menu trigger and
  no inline PR rows.
- **GIVEN** a task with two linked GitHub PRs and the Pull requests sub-menu
  open, **WHEN** the user selects a PR row, **THEN** that PR's dockview panel
  opens and the row is labeled with its repo (`PR #N — owner/repo`).
- **GIVEN** a task with three linked GitHub PRs, **WHEN** the user opens the "+"
  add-panel menu, **THEN** the main menu height is bounded by the single Pull
  requests trigger instead of three PR rows.

## Out of scope

- Changing PR ordering, linking, or provider data.
- Grouping GitLab merge requests into a sub-menu; MR rows keep their current
  inline rendering.
- Changing the PR top-bar button, the multi-PR CI popover, or the PR picker
  dialog.
- Changing the mobile bottom-sheet styling itself; this feature only reuses the
  existing nested-menu treatment.

## Implementation plan

[Task Add-Panel PR Submenu implementation plan](../../plans/add-panel-pr-submenu/plan.md)
