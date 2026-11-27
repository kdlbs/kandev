---
id: "02-task-panel"
title: "Clarify the contextual task panel and compact rows"
status: done
wave: 2
depends_on:
  - "01-desktop-navigation"
plan: "plan.md"
requirements:
  - REQ-UI-NAV-HIERARCHY-002
acceptance_criteria:
  - AC-UI-NAV-HIERARCHY-002.1
  - AC-UI-NAV-HIERARCHY-002.2
  - AC-UI-NAV-HIERARCHY-002.3
  - AC-UI-NAV-HIERARCHY-002.4
  - AC-UI-NAV-HIERARCHY-002.5
  - AC-UI-NAV-HIERARCHY-002.6
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
---

# Task 02: Contextual task panel and compact rows

## Summary

Separate contextual Tasks from navigation, expose active filters, and make groups
and task rows easier to scan. Use the shared row/filter logic for desktop and phone.

## In scope

- TDD for effective saved/draft filtering indicators and group state semantics.
- Contextual heading with independent view/filter actions and a section divider.
- State-aware group indicator, count badge, accessible disclosure; neutral
  presentation for non-state grouping and unknown state keys.
- Plain task rows, preserved density, configured metadata/trailing content,
  nested tasks, selection, context actions, and touch hitboxes.
- Add targeted desktop/mobile rendering tests and localized accessibility copy.

## Out of scope

Default/saved grouping migrations, new state meanings, filter dimensions, APIs,
pagination algorithms, view persistence, or phone app-menu reordering.

## Acceptance

1. Saved active filters and unsaved edits have truthful cues, including after
   reload; sort/group-only changes do not claim filtering (002.1-002.2).
2. Correct state-group indicators/counts and compact row layout preserve existing
   data, configuration, subtree semantics, actions, and scroll behavior (002.3-002.6).
3. Desktop and mobile tests prove real navigation/filter actions plus containment
   of titles, badges, diff stats, and 44px phone action targets in both themes.

## ASCII UI preview

UI-01/UI-03 excerpt. Full [preview](plan.md#ascii-ui-preview).

```text
-------------------------------
TASKS v [All tasks v] [filter *]
(o) To do                  [1] v
+------------------------------+
| o Fix issue search           |
|   compass/app #125   32s      |
+------------------------------+
(o) In progress            [1] v
| o Improve empty states       |
|   compass/app       +733 -9  |
```

Same anatomy in the existing phone task drawer, with visible touch actions and
44px controls. Its app-menu host shares one content scroller; desktop Tasks owns
its internal scroller. State grouping shown for comparison is a saved-view choice.
An empty/failed task list retains the view/filter controls and retry. Maps to
AC-UI-NAV-HIERARCHY-002.1 through 002.6.

## Verification

From repository root:

```bash
(cd apps/web && pnpm exec vitest run components/app-sidebar/sections/tasks-section.test.tsx components/app-sidebar/sections/tasks-view-picker.test.tsx components/task/task-switcher-group.test.tsx components/task/task-item.test.tsx components/task/task-item-compact-layout.test.tsx components/task/task-item-trailing.test.tsx components/task/task-item-stats-row.test.tsx components/task/task-switcher-render-stability.test.tsx components/task/task-session-sidebar-grouped-view.test.ts lib/sidebar/apply-view.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-task-panel-hierarchy.spec.ts tests/task/sidebar-layout.spec.ts tests/task/sidebar-filter.spec.ts tests/task/sidebar-title-width.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-panel-hierarchy.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/app-sidebar/sections/tasks-section.tsx components/app-sidebar/sections/tasks-view-picker.tsx components/task/task-switcher-group.tsx components/task/task-switcher-tree.tsx components/task/task-switcher.tsx components/task/task-item.tsx components/task/sidebar-filter --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

New specs cover details-hidden/missing metadata, long titles and counts, saved
filtered view reload, status and repository grouping, continued pages, keyboard
collapse, and phone row-menu action independent of navigation. Include 393/767px
and narrow fine-pointer input; measure action hit targets and document overflow.

## Files likely touched

- `apps/web/components/app-sidebar/sections/tasks-section.tsx`, `tasks-view-picker.tsx` and tests.
- `apps/web/components/task/sidebar-filter/sidebar-filter-bar.tsx` and focused tests/helper if needed.
- `apps/web/components/task/task-switcher-group.tsx`, `task-switcher-tree.tsx`,
  `task-switcher.tsx`, `task-item.tsx` and relevant tests.
- `apps/web/lib/sidebar/apply-view.ts` only if a presentation dimension must be
  carried; preserve filtering/grouping semantics and test it.
- Existing locale namespaces for active-filter accessible copy.
- New `apps/web/e2e/tests/task/sidebar-task-panel-hierarchy.spec.ts` and
  `mobile-sidebar-task-panel-hierarchy.spec.ts`.

## Dependencies

Task 01 establishes section-header presentation. Keep this change compatible with
the existing phone composition so Task 03 can reorder it independently.

## Risks

Rows are shared across sidebar/task-picker surfaces. Never use the first row's
status as a group state or break descendant/paginated counts, inline dialogs,
task colors, or context-menu hit targets with decorative wrappers.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/navigation-hierarchy.md), REQ-UI-NAV-HIERARCHY-002.
- [Design](../../specs/ui/system-design/navigation-hierarchy.md), Task panel/State groups and task rows.
- Existing `TaskItem`, `useEffectiveSidebarView`, `GroupSection`, and task panel tests.

## Results

Implemented compact task cards, semantic state-group icons, count badges, accessible collapse state, and separate effective-filter and unsaved-draft indicators. Existing row presentation preferences, task-tree grouping, metadata, linked PRs, actions, and diff statistics remain wired to their existing sources.

Validation passed:

- Task-defined row, group, view, and filter unit tests. New filter-indicator cases cover saved filters, sort-only drafts, cleared filters, and drafts for another view. State-group cases cover known states and neutral non-state/unknown groups.
- All 35 desktop sidebar filter/layout/title-width/panel browser scenarios (34 initial regression passes plus the corrected keyboard-collapse scenario).
- Mobile task-action regression scenarios and the new status-panel scenario, including both themes at 393px/767px.
- Additional 767px mouse and 768px touch boundary checks. These exposed and verified fixes for task-action visibility and minimum group-header height.

Updated active-row assertions to reflect the intentional one-pixel card border on all four sides. The shared filter-indicator tests replace the planned duplication in the picker tests.

### Plain-row refinement (2026-09-29)

The user's subsequent visual review supersedes the initial card treatment.
Parent tasks and subtasks now use transparent, borderless rows. Hover and active
selection retain tonal fills; keyboard focus and multiselection retain their
rings. Metadata, grouping, nesting, row actions, and phone targets are preserved.
Active-row browser assertions now require a borderless highlighted row. The
latest validation and comparison refresh are recorded in the plan.


## PR review and CI remediation (2026-09-30)

Blocked, failed, and cancelled group headings use the established state-aware
icons instead of the task-row backlog fallback. Applied-filter and unsaved-draft
markers expose labelled image roles. Regressions cover their distinct semantics.
CI geometry assertions retain the accepted plain-row 8px inset and padding while
continuing to reject a reserved scrollbar gutter, clipped hover actions, and
trailing values outside the row.

A command-selected task could move below the list as hydrated group content grew
after its initial scroll. The brief selection cue now observes the viewport and
its content and recenters only the still-selected row when needed. Cancellation,
replacement, and expiry disconnect the observer. Unit regressions cover
viewport/content growth and cancellation; the delayed navigation-blocker browser
test exercises the complete route/list path. Final validation and CI evidence
are recorded in the plan and linked PR.
