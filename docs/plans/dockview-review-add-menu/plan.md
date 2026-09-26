---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-08-14
status: completed
---

# Fix Plan: Dockview Review Add Menu

## Overview

Make each task Dockview `+` menu list only reviews whose panel is missing from
the live layout. A matching canonical or keyed review tab in any split removes
that review from every add-panel menu; it stays where the user placed it. This
matches the Layout editor's existing missing-panel behavior and preserves the
current contract that explicit review opens focus existing tabs in place rather
than relocating them. When a review is missing, selecting it from a group's `+`
menu creates it in that invoking group.

The durable behavior is recorded in
[`task-layout-profiles.md`](../../specs/ui/requirements/task-layout-profiles.md), with the
GitHub list and submenu details amended in
[`add-panel-pr-submenu.md`](../../specs/ui/requirements/add-panel-pr-submenu.md).

## Confirmed Root Cause

`AddPanelMenuItems` renders every linked PR, MR, and registered-provider review
without consulting the live Dockview panels. Its state only tracks whether the
single-instance Files and Changes panels exist. In contrast,
`addPRPanel`, `addMRPanel`, and `addReviewPanel` deliberately deduplicate a
matching canonical or keyed review and focus it in place. The menu therefore
offers an add action whose click cannot add to the invoking group or move the
existing tab.

The mismatch became user-visible when `cc6eb4dd5` restored automatic canonical
PR Details insertion for linked reviews but did not update the older add-panel
rows. A throwaway Vitest regression placed canonical `pr-detail` for
`acme/kandev/42` in `group-right`, rendered another group's menu, and expected
`add-panel-pr-item-acme-kandev-42` to be absent. Current code failed exactly at
that assertion: the menu item remained present. The throwaway test was removed;
the worktree was clean before these design artifacts were written.

User retesting confirmed a second placement defect after the missing-only menu
repair: `AddPanelMenuItems` receives the invoking `groupId`, but its GitHub,
GitLab, and registered-provider review callbacks discard it. The corresponding
store actions accept no placement option and fall back to `centerGroupId` when
no canonical `pr-detail` panel exists. The focused RED regression requested
`group-invoking-add-menu` and observed `group-center`.

## Frontend

### Live review-panel availability

- Update `apps/web/components/task/dockview-add-panel-items.tsx` with one pure
  exact-identity predicate over the live `DockviewApi`.
- For GitHub, recognize both canonical `pr-detail` parameters for the same
  `prKey` and keyed `pr-detail|<prKey>` panels.
- For GitLab, recognize canonical `pr-detail` or legacy `mr-detail` parameters
  for the same `mrKey` and keyed `mr-detail|<mrKey>` panels.
- For registered providers, compare the canonical provider-neutral identity
  (`providerId`, `connectionScope`, `repositoryId`, and
  `changeRequestNumber`) and the existing `reviewPanelId(review)` keyed panel.
- A mixed-provider canonical selector without one exact review identity does
  not hide any individual review. Only an exact matching review panel counts as
  open.
- Read the current store API when the dropdown content mounts. Do not add
  persistence or cached availability state; closing a panel must make its row
  available the next time the menu opens.

### Missing-only menu rows

- Filter `state.prs`, `state.mrs`, and registered-provider reviews through the
  predicate before rendering their menu rows.
- Feed only missing GitHub PRs into `PRPanelMenuItems`, so zero missing PRs
  render nothing, one renders inline, and two or more use the existing submenu.
- Keep labels, icons, stable test IDs, ordering, and the existing add actions
  unchanged for reviews that remain available.
- Forward the invoking `groupId` for each available GitHub, GitLab, and
  registered-provider row. Extend the three review actions with an optional
  placement option so an explicit group wins for newly created tabs, while
  callers that omit it retain canonical/center fallback behavior.
- For an exact open match, omit the row instead of invoking its existing
  focus-in-place action; do not add relocation logic. Topbar and other
  explicit-open surfaces retain their current focus-in-place behavior.

No backend, API, persistence, feature-flag, localization, or layout-editor code
changes are required.

## Public Documentation

- Update the PR Details paragraph in
  `docs/public/sessions-and-review.md` to state that a group's `+` menu lists a
  linked review only while its exact panel is missing, and that moving an open
  review between splits uses Dockview's tab and layout controls.
- Keep the page's primary how-to purpose and existing terminology. No new page,
  navigation entry, screenshot, or media change is required.

## Tests

- **What:** an exact canonical PR in another group and an exact keyed PR are
  omitted, while a different linked PR remains available.
  **File:** `apps/web/components/task/dockview-add-panel-items.test.tsx`.
  **How:** first add the failing render/helper cases against unchanged
  production code, then implement the predicate and rerun them.
- **What:** filtering occurs before the inline/submenu threshold.
  **File:** the same Vitest file.
  **How:** cover two linked PRs with one open (one inline row, no submenu) and
  three linked PRs with one open (submenu containing only the two missing
  rows).
- **What:** GitLab and registered-provider exact matches use the same rule, and
  a mixed canonical selector does not suppress individual rows.
  **File:** the same Vitest file.
  **How:** exercise the exported pure identity predicate with small Dockview API
  fakes and assert matching versus non-matching identities.
- **What:** an add-menu request creates a missing review in the invoking group,
  while existing focus-in-place and non-menu fallback behavior remain
  unchanged.
  **File:** `apps/web/lib/state/dockview-panel-actions-extra.test.ts`.
  **How:** first assert the explicit-group failure, then cover GitHub, GitLab,
  and registered-provider placement plus the existing fallback guards.

## E2E Tests

- **Scenario:** **GIVEN** the Layout editor places canonical PR Details in the
  Files/Changes split and a fresh task links that PR, **WHEN** the user opens
  the Agent split's `+` menu, **THEN** that PR row is absent and the canonical
  panel remains in the configured split.
  **File:** `apps/web/e2e/tests/settings/layout-profiles.spec.ts`.
- **Scenario:** **GIVEN** two linked PRs with the primary PR already represented
  by canonical PR Details, **WHEN** the user opens the `+` menu, **THEN** the
  primary is absent, the secondary is the only inline PR row, and selecting it
  opens one keyed tab. Reopening the menu after both are open shows no PR rows.
  **File:** `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`.

Both scenarios assert user-visible menu state and live Dockview placement. API
calls remain setup only.

## Mobile Design Contract

- Desktop entry: the per-group Dockview `+` menu and empty-group watermark.
- Phone/tablet task entry: unchanged. `SessionMobileLayout` uses its existing
  review destination and does not render Dockview's add-panel menu.
- Nearest shipped mobile behavior: the dedicated mobile task review surface in
  `apps/web/components/task/task-layout.tsx`; it already exposes linked review
  content without split-group placement.
- Mobile settings behavior: unchanged. The responsive Layout editor already
  lists only globally missing reusable panels and remains the touch path for
  configuring future PR Details placement.
- No mobile Playwright case is added because the changed menu has no mobile
  task-layout entry point and no composition, scrolling, navigation, safe-area,
  or touch behavior changes.

## Verification Results

- RED: the focused add-panel unit file ran 18 tests with the five new hiding
  assertions failing as expected and 13 existing/guard assertions passing.
- GREEN: the final add-panel and panel-action regression suites passed 41 tests
  across 2 files.
- Targeted ESLint and frontend TypeScript checks passed. `i18n:check` passed
  with only the repository's advisory locale-parity report, and the new-code
  i18n ratchet reported 0 added plus 3 modified production files clean.
- Playwright listed exactly the two intended Chromium scenarios. The first
  production-build run passed the multi-PR case and exposed an unstable Agent
  text locator in the layout case; after switching to the existing session-tab
  group selector, a fresh disposable-backend rerun against the unchanged build
  passed both scenarios in 22.0 seconds.
- The placement repair's final production build passed its focused Chromium
  scenario: topbar opening retained the center fallback, then the Terminal
  split's `+` menu opened the missing review in Terminal (1 test, 12.3 seconds).
- Public-doc validation passed 61 tests and validated 41 published pages.
  `git diff --check` passed.

## Implementation Task

- [x] [Task 01: Filter open review menu entries](task-01-filter-open-review-menu-entries.md)
- [x] [Task 02: Place reviews in invoking group](task-02-place-reviews-in-invoking-group.md)

Execution is sequential in the primary conversation. This single task is not a
parallel candidate and does not authorize subagent use.

## Risks and Boundaries

- Canonical panels have legacy built-in keys and newer provider-neutral params;
  matching must accept both without treating a mixed-review selector as one
  exact review.
- Filter before computing the PR submenu threshold or a single remaining PR
  will stay hidden behind a misleading multi-PR trigger.
- Read live Dockview state when the menu opens; a cached list could stay stale
  after close, restore, or environment switch.
- Review linking, ordering, topbar behavior, relocation of existing panels,
  saved layouts, mobile review navigation, and non-review add-panel rows remain
  out of scope.
