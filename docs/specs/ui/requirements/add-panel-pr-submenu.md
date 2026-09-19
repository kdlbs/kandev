---
status: draft
system: ui
created: 2026-07-31
owners:
  - Kandev frontend
---
# Task Add-Panel PR Submenu Requirements

## Overview

The task-view "+" add-panel menu lists every GitHub pull request linked to the task as a flat menu row. A task can carry up to ten or more linked PRs (e.g. multi-repo work), which makes the dropdown too tall to use — it spills past the viewport or requires scrolling through a wall of identical rows.

## Requirements

### REQ-UI-ADD-PANEL-PR-SUBMENU-001: Task Add-Panel PR Submenu

**Intent:** The task-view "+" add-panel menu lists every GitHub pull request linked to the task as a flat menu row. A task can carry up to ten or more linked PRs (e.g. multi-repo work), which makes the dropdown too tall to use — it spills past the viewport or requires scrolling through a wall of identical rows.

#### Acceptance criteria

- **AC-UI-ADD-PANEL-PR-SUBMENU-001.1:** The task-view "+" add-panel menu (dockview group header and empty-group watermark) lists only linked GitHub PRs that do not already have a matching canonical or keyed review panel anywhere in the current Dockview layout.
- **AC-UI-ADD-PANEL-PR-SUBMENU-001.2:** A matching open panel is identified by the PR's stable task key, regardless of which split group owns it. The add-panel menu never offers that PR as a way to focus or relocate the existing tab; users move existing tabs through Dockview's tab and split controls.
- **AC-UI-ADD-PANEL-PR-SUBMENU-001.3:** The menu keeps its current behavior when zero or one linked PR remains after filtering: one available PR renders as one inline menu row, and no available PR renders nothing.
- **AC-UI-ADD-PANEL-PR-SUBMENU-001.4:** When more than one linked PR remains after filtering, the menu shows a single **Pull requests** sub-menu trigger instead of inline PR rows.
- **AC-UI-ADD-PANEL-PR-SUBMENU-001.5:** Opening the sub-menu reveals one row per available linked PR. Each row opens that PR's Dockview panel in the split whose add-panel menu was used. The single inline PR row uses the same invoking-split placement.
- **AC-UI-ADD-PANEL-PR-SUBMENU-001.6:** Per-PR rows inside the sub-menu keep the current disambiguating label used for multi-PR tasks (`PR #42 — owner/repo`), while single-PR tasks keep the plain `PR #42` label on the inline row.
- **AC-UI-ADD-PANEL-PR-SUBMENU-001.7:** The sub-menu trigger carries the same add-panel menu styling as other rows and an icon consistent with the PR rows.
- **AC-UI-ADD-PANEL-PR-SUBMENU-001.8:** Per-PR rows keep their existing stable test identifiers so automated tests can select them from within the sub-menu.

## Migrated source detail

## Why

The task-view "+" add-panel menu lists every GitHub pull request linked to the
task as a flat menu row. A task can carry up to ten or more linked PRs (e.g.
multi-repo work), which makes the dropdown too tall to use — it spills past the
viewport or requires scrolling through a wall of identical rows.

## What

- The task-view "+" add-panel menu (dockview group header and empty-group
  watermark) lists only linked GitHub PRs that do not already have a matching
  canonical or keyed review panel anywhere in the current Dockview layout.
- A matching open panel is identified by the PR's stable task key, regardless
  of which split group owns it. The add-panel menu never offers that PR as a
  way to focus or relocate the existing tab; users move existing tabs through
  Dockview's tab and split controls.
- The menu keeps its current behavior when zero or one linked PR remains after
  filtering: one available PR renders as one inline menu row, and no available
  PR renders nothing.
- When more than one linked PR remains after filtering, the menu shows a single
  **Pull requests** sub-menu trigger instead of inline PR rows.
- Opening the sub-menu reveals one row per available linked PR. Each row opens
  that PR's Dockview panel in the split whose add-panel menu was used. The
  single inline PR row uses the same invoking-split placement.
- Per-PR rows inside the sub-menu keep the current disambiguating label used for
  multi-PR tasks (`PR #42 — owner/repo`), while single-PR tasks keep the plain
  `PR #42` label on the inline row.
- The sub-menu trigger carries the same add-panel menu styling as other rows and
  an icon consistent with the PR rows.
- Per-PR rows keep their existing stable test identifiers so automated tests can
  select them from within the sub-menu.
- Closing a matching canonical or keyed review panel makes that PR available in
  the add-panel menu again. Other linked PRs remain independently available or
  hidden according to their own open-panel state.
- GitLab and registered-provider review rows use the same already-open filter,
  invoking-split placement, and existing inline presentation and labels.
- The sub-menu must work with the existing Radix dropdown primitives on desktop
  (pointer hover and keyboard). The feature is desktop-only: mobile renders
  `SessionMobileLayout`, which has no "+" add-panel entry point, so the sub-menu
  has no mobile presentation.

## Scenarios

- **GIVEN** a task with no linked GitHub PRs, **WHEN** the user opens the "+"
  add-panel menu, **THEN** no PR row or Pull requests trigger is shown.
- **GIVEN** a task with exactly one linked GitHub PR, **WHEN** the user opens
  the "+" add-panel menu, **THEN** one inline PR row labeled `PR #N` is shown
  and no Pull requests sub-menu trigger is shown.
- **GIVEN** a task whose only linked GitHub PR is already rendered by the
  canonical PR Details panel in another split, **WHEN** the user opens the "+"
  add-panel menu in any group, **THEN** no row or sub-menu trigger for that PR
  is shown and the existing panel is neither focused nor moved.
- **GIVEN** a task with two linked GitHub PRs whose primary PR is already open,
  **WHEN** the user opens the "+" add-panel menu, **THEN** only the other PR is
  shown as an inline row and no Pull requests sub-menu trigger is shown.
- **GIVEN** a task with three linked GitHub PRs and one matching panel already
  open, **WHEN** the user opens the "+" add-panel menu, **THEN** the Pull
  requests sub-menu contains only the two missing PRs.
- **GIVEN** a linked PR was omitted because its matching panel was open,
  **WHEN** the user closes that panel and reopens the "+" add-panel menu,
  **THEN** the PR is offered again.
- **GIVEN** a GitLab or registered-provider review already has a matching panel
  anywhere in Dockview, **WHEN** the user opens another group's add-panel menu,
  **THEN** that review row is omitted under the same identity rule.
- **GIVEN** a task with two linked GitHub PRs and neither is already open,
  **WHEN** the user opens the "+" add-panel menu, **THEN** the menu shows a Pull
  requests sub-menu trigger and no inline PR rows.
- **GIVEN** a task with two available linked GitHub PRs and the Pull requests
  sub-menu open, **WHEN** the user selects a PR row, **THEN** that PR's dockview
  panel opens in the split whose add-panel menu was used and the row is labeled
  `PR #N — owner/repo`.
- **GIVEN** no matching PR Details tab is open, **WHEN** the user selects a
  linked GitHub PR from a non-default split's add-panel menu, **THEN** the new
  review tab opens in that non-default split rather than the default group.
- **GIVEN** a task with three available linked GitHub PRs, **WHEN** the user
  opens the "+" add-panel menu, **THEN** the main menu height is bounded by the
  single Pull requests trigger instead of three PR rows.

## Out of scope

- Changing PR ordering, linking, or provider data.
- Grouping GitLab merge requests into a sub-menu; MR rows keep their current
  inline rendering.
- Changing the PR top-bar button, the multi-PR CI popover, or the PR picker
  dialog. Those surfaces may continue to focus an already-open review in place.
- Mobile presentation of the sub-menu: `SessionMobileLayout` has no "+"
  add-panel entry point, so there is no mobile sub-menu to style.

## Implementation plans

- [Task Add-Panel PR Submenu implementation plan](../../../plans/add-panel-pr-submenu/plan.md)
- [Dockview Review Add Menu repair plan](../../../plans/dockview-review-add-menu/plan.md)
