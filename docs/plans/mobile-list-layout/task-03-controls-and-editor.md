---
id: "03-controls-and-editor"
title: "Compact search and left-side view editing"
status: completed
wave: 3
depends_on:
  - "02-phone-list"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-LIST-VIEWS-002
  - REQ-UI-MOBILE-LIST-VIEWS-003
  - REQ-UI-MOBILE-LIST-VIEWS-004
acceptance_criteria:
  - AC-UI-MOBILE-LIST-VIEWS-002.1
  - AC-UI-MOBILE-LIST-VIEWS-002.2
  - AC-UI-MOBILE-LIST-VIEWS-002.3
  - AC-UI-MOBILE-LIST-VIEWS-002.4
  - AC-UI-MOBILE-LIST-VIEWS-002.5
  - AC-UI-MOBILE-LIST-VIEWS-002.6
  - AC-UI-MOBILE-LIST-VIEWS-002.7
  - AC-UI-MOBILE-LIST-VIEWS-002.8
  - AC-UI-MOBILE-LIST-VIEWS-003.1
  - AC-UI-MOBILE-LIST-VIEWS-003.2
  - AC-UI-MOBILE-LIST-VIEWS-003.3
  - AC-UI-MOBILE-LIST-VIEWS-003.4
  - AC-UI-MOBILE-LIST-VIEWS-003.5
  - AC-UI-MOBILE-LIST-VIEWS-003.6
  - AC-UI-MOBILE-LIST-VIEWS-003.7
  - AC-UI-MOBILE-LIST-VIEWS-004.2
  - AC-UI-MOBILE-LIST-VIEWS-004.3
  - AC-UI-MOBILE-LIST-VIEWS-004.6
system_design:
  - ../../specs/ui/system-design/mobile-list-saved-views.md
---

# Task 03: Compact search and left-side view editing

## Summary

Make the header search icon reveal temporary result controls. Compose the
shared editor inside the top-left List options sheet, with one save owner.

## In scope

- Add the search icon immediately left of the hamburger, with focus and expanded state.
- Reveal one search/Filters row below tabs; cancel and clear it on close.
- Add result-filter local draft, Apply/Clear, active count, and summaries.
- Reuse shared editor state/actions in the List options sheet.
- Keep saved filters, sort, group, row settings, and existing view management in that sheet.
- Preserve immediate automatic colors and existing view draft/save/discard semantics.
- Preserve explicit archive/link scope and eligible plugin facet overrides.
- Bind controller and deletion callbacks to their originating workspace.
- Migrate affected mobile tests to the new entry points without deleting scenarios.
- Localize any new copy in every required locale.

## Out of scope

New saved-view persistence, a second editor journal, global size changes,
Threads collection changes, or result-filter writes to portable settings.

## Acceptance

1. UI-02/UI-03 refine results without any saved-view/draft writes. Hide/clear,
   debounce cancellation, route changes, and stale responses retain correct results.
2. Only the left List control opens the editor from phone List. UI-04/UI-05
   retain full shared editing and view management, with no temporary refinements in saves.
3. Compatible links/facets remain usable and visibly separate, as in UI-09.
   Controls retain baseline heights, contained scrolling, focus return, and localized feedback.

## ASCII UI preview

Full previews: [UI-02](plan.md#ui-02-phone-list-search-open),
[UI-03](plan.md#ui-03-temporary-result-filter-drawer),
[UI-04](plan.md#ui-04-view-options-sections-collapsed),
[UI-05](plan.md#ui-05-view-options-sort-expanded), and
[UI-09](plan.md#ui-09-explicit-link-or-plugin-compatibility-state).

### UI-02: Phone List, search open

Entry: tap `[S]`. AC: 002.1-.6, 004.2, 004.6.

```text
+--------------------------------------+
| Default                              |
| List v                       [S] [=] |
+--------------------------------------+
| [All tasks] [Archived] [My view]      |
| [ Search tasks...        ] [Filters] |
| Status: Running [x]                   |
+--------------------------------------+
|                                      |
| Matching task rows                   |
+--------------------------------------+
```

The summary appears only for applied filters. Search focuses on open. A second
search-icon tap clears refinements and restores UI-01. No new saved view is written.

### UI-04: View options, sections collapsed

Entry: top-left List v. AC: 003.1-.3, 003.5-.7, 004.2-.3.

```text
+--------------------------------------+
| View options                     [x] |
+--------------------------------------+
| [Kanban] [Threads] [List]             |
|                                      |
| Saved view: All tasks          [...] |
| Filters                           > |
| Sort                              > |
| Group by                          > |
| Task row                          > |
| Automatic colors                  > |
|                                      |
+--------------------------------------+
| [Discard] [Save as new]       [Save]  |
|              safe area               |
+--------------------------------------+
```

The title is fixed; the settings body owns one vertical scroller. The draft
footer is fixed and appears only for a view draft. Section summaries can wrap.
No empty action wrapper creates a gap above the layout selector. View creation,
rename, and delete remain inside the view actions, with existing confirmation rules.

For expanded Sort, use UI-05's four stacked rules, existing grips, and More
menus. Do not squeeze desktop selectors into one phone row. UI-03 never enters
saved-view edit mode. UI-09 never becomes the default toolbar.

## Verification

```bash
pnpm --dir apps/web exec vitest run lib/tasks/mobile-list-query.test.ts app/tasks/mobile-list-toolbar.test.tsx components/kanban/kanban-header-mobile.test.tsx components/kanban/mobile-menu-sheet.test.tsx components/task/sidebar-filter/sidebar-filter-popover.test.tsx
pnpm --dir apps/web e2e:run --project mobile-chrome tests/task/mobile-list-saved-views.spec.ts tests/task/mobile-task-list-search.spec.ts tests/task/mobile-task-list-filters.spec.ts tests/task/mobile-task-listing-display.spec.ts tests/task/mobile-task-list-archive-navigation.spec.ts tests/plugins/mobile-task-list-facet.spec.ts
pnpm --dir apps/web e2e:run --project mobile-chrome tests/task/mobile-workspace-sidebar-views.spec.ts tests/task/mobile-sidebar-running-first-activity-sort.spec.ts
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run typecheck
git diff --check
```

The toolbar unit test is planned. The shared editor test file already exists.
Add focused controller cases there and keep the existing tests.

Inventory old mobile selectors before replacement:

```bash
rg -n 'mobile-tasks-list-sort|mobile-tasks-list-group|tasks-list-archive|sidebar-filter-gear|mobile-search-toggle' apps/web/e2e/tests -g 'mobile-*.spec.ts'
```

Retain each affected flow through its new visible opener. Add no-write assertions
for temporary filters, plus real saved-view reload and desktop/sidebar reads
for Save. Verify automatic colors independently from view Discard.
Open UI-02 through UI-05 and UI-09 screenshots and record any mismatch.

## Files likely touched

- `apps/web/components/kanban/kanban-header-mobile.tsx` and its test.
- `apps/web/components/kanban/mobile-menu-sheet.tsx`, `mobile-display-options.tsx`, and affected tests.
- `apps/web/components/kanban/mobile-menu-task-list-options.tsx` only for normal phone List removal/compatibility separation.
- New `apps/web/app/tasks/mobile-list-toolbar.tsx` and its test.
- `apps/web/app/tasks/tasks-list-filters-drawer.tsx` or a focused phone result-filter wrapper.
- New `apps/web/hooks/use-mobile-list-results.ts` and focused tests.
- `apps/web/components/task/sidebar-filter/sidebar-filter-popover.tsx`, `sidebar-view-editor.tsx`, and `view-manager.tsx`.
- A shared controller extracted under `apps/web/components/task/sidebar-filter/`.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` only for presentation capability wiring.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/` and generated pseudo as required.
- The E2E files named above and any additional old-opener consumers found by the inventory.

## Dependencies

Task 02 supplies the primary saved-view page. Task 01 supplies result query composition.

## Risks

The shared editor currently owns its own header actions. A wrapper must not add
a second save row. Automatic colors save immediately. Workspace callbacks and
drafts must survive only under their established identity. Legacy facet tests
must retain actual selection/grouping evidence, not just a renamed selector.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-list-saved-views.md), 002 through 004.
- [Design](../../specs/ui/system-design/mobile-list-saved-views.md), Result state, Editor ownership, and Compatibility.
- Existing sidebar filter controller, confirmation host, view manager, and mobile List scenarios.

## Results

Completed on 2026-10-27.

- The header search control reveals the temporary search/filter row and returns
  focus when closed. Search, applied filters, link scope, and plugin overrides
  remain outside the saved view draft.
- The left List control opens the shared selected-view editor. It retains
  grouping, ordered sort rules, row presentation, view management, draft/save/
  discard behavior, and the independent automatic-color owner.
- `pnpm e2e:run --host --no-build --project mobile-chrome` over the 16 affected
  mobile specs passed all 62 tests, including search cancellation and page
  reset, saved-view persistence, no-write filters, link/archive scope, plugin
  facets, and the four-rule Running/Red/Orange/activity chain.
- The 393x852 light/dark screenshots for UI-02 through UI-05 and UI-09 were
  captured and visually inspected. Search adds one 52px row; the selected-view
  tabs remain 40px; the editor retains one internal scroller and the full sort
  chain.
- Focused unit coverage (123 tests) and web typecheck/i18n checks passed in the
  preceding implementation session. Current wider responsive and full mobile
  E2E runs passed; see [visual acceptance](verification.md).
- Review remediation uses the page's effective group and sort metadata while
  keeping the saved view as the editor identity. Link grouping keeps collapse
  state temporary, including when the requested group matches the saved group.
- A link-override E2E verifies ungrouped repository labels, title ordering,
  workflow headers over an executor-grouped saved view, and unchanged saved
  view state after collapsing the temporary group.
- The settled-search regression confirms the 44px clear target stays inside
  the input bounds and clears the query when tapped.
- The four focused review E2E specs passed 12/12 tests. The typecheck and
  focused hook/component tests passed; see [visual acceptance](verification.md).
