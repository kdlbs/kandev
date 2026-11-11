---
id: "01-sidebar-facet-grouping"
title: "Expose plugin facets in saved sidebar views"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SIDEBAR-TASK-FACETS-001
acceptance_criteria:
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.1
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.2
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.3
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.4
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.5
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.6
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.7
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.8
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.9
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.10
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.11
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.12
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.13
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.14
  - AC-PLUGINS-SIDEBAR-TASK-FACETS-001.15
system_design:
  - ../../specs/plugins/system-design/sidebar-task-facets.md
---

# Task 01: Expose Plugin Facets in Saved Sidebar Views

## Summary

Deliver saved sidebar facet grouping through the existing plugin contract,
including repeated rows, section-local hierarchy, persistence, live updates,
availability recovery, and safe task interaction.

## Scope

- Extend sidebar Group by choices and saved-view/draft group typing.
- Reproject the accepted bounded page, preserving source filters and ordering.
- Render independent value trees, counts, swatches, and collapse identities.
- Resolve range endpoints by occurrence and deduplicate canonical bulk targets.
- Disable root reordering when a facet section contains promoted child roots.
- Preserve unavailable facet intent and safely display unknown saved options.
- Update durable requirements, design, delivery record, and relevant public docs.

## Exclusions and dependencies

No facet sorting/filtering, workspace-wide fetches, server facet query, backend
schema migration, SDK extension, or unrelated plugin-repository changes.
The existing task-list facet registry and resolver are the only prerequisites.

## Acceptance

- AC .1-.10: saved workspace selections produce the correct page-local sections,
  hierarchy, colors, and independent collapse on desktop and phones.
- AC .11 and .15: repeated occurrences retain canonical actions, correct range
  selection, and safe sibling-order persistence.
- AC .12-.14: live updates and missing plugin recovery work without rewriting
  saved intent; unusual saved values leave the editor usable.

## ASCII UI preview

UI-01 from the [plan](plan.md#ascii-ui-preview), shared by desktop and phone:

```text
GROUP BY [Tag v]
v blue       2
  Parent
    Child
v urgent     1
  Child
v Unassigned 1
  Other task
```

The phone uses the existing settings drawer and grouped task switcher. Parent
collapse affects only its own section. Promoted Child stays a root only in the
urgent section and cannot be dragged into persisted root order. Maps to AC .1,
.6-.10, .13-.15; spacing is illustrative.

## Files and acceptance evidence

All web paths below are relative to `apps/web/`.

| Criteria | Source and regression boundary |
| --- | --- |
| .1, .13, .14 | `components/task/sidebar-filter/group-picker.tsx`, `group-picker.test.tsx`; the editor summary uses `useSidebarGroupLabel` |
| .2, .10 | `lib/state/slices/ui/sidebar-view-wire.test.ts`: saved view and draft round trips, facet collapse keys |
| .3, .5, .7-.10 | `lib/sidebar/sidebar-facet-groups.ts` and `.test.ts`: source order, memberships, hierarchy, section keys |
| .3 | `hooks/domains/kanban/use-sidebar-task-page.test.tsx`: built-in query and local disclosure |
| .4, .12, .13 | `hooks/domains/sidebar/use-sidebar-facet-grouping.ts` and `.test.tsx`: workspace, subscriptions, registry lifecycle |
| .11 | `components/task/task-switcher-facet-selection.test.tsx`, `hooks/use-task-multi-select-range.test.ts`: repeated range endpoints and deduplicated canonical targets |
| .15 | `components/task/task-switcher-facet-dnd.test.tsx`: promoted-root sections, valid real-root sections, ordinary sibling reorder |
| .1, .6-.13 | Real desktop/phone sidebar demonstration recorded in Results; task-list facet browser suites provide separate `/tasks` compatibility coverage |

## Verification

Run from the repository root, sequentially, after integration. These commands
are the verification recipe, not claims that this worker executed them.

```bash
(cd apps/web && pnpm exec vitest run components/task/sidebar-filter/group-picker.test.tsx lib/sidebar/sidebar-facet-groups.test.ts hooks/domains/sidebar/use-sidebar-facet-grouping.test.tsx lib/state/slices/ui/sidebar-view-wire.test.ts hooks/domains/kanban/use-sidebar-task-page.test.tsx components/task/task-switcher-facet-selection.test.tsx components/task/task-switcher-facet-dnd.test.tsx hooks/use-task-multi-select-range.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
node --test .github/scripts/pr-docs.test.cjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Additionally review the actual desktop/phone sidebar with an installed test
plugin: select Tag, save/reload, inspect repeated tasks and empty-value omission,
rename values live, disable/re-enable, and range-select a later duplicate. Check
heading swatches against host validation and verify source requests remain bounded
and use built-in grouping. The existing `task-list-facet.spec.ts` and
`mobile-task-list-facet.spec.ts` exercise `/tasks`, not this sidebar contract.

For the PR gate, the changed work order must be included with its sibling plan,
linked requirements and criteria, and system design. The local `validateCoverage`
entry point in `.github/scripts/pr-docs.cjs` accepts changed-file metadata and
referenced document contents; a live PR event runs the required coverage status.
Do not add a bypass label or weaken the path/artifact validator.

## Results

Implementation sources deliver the grouping and interaction contract above.
Review remediation removes the unsafe built-in label lookup and makes the
picker display None for unknown saved strings without emitting a write. The
regression covers legacy strings, malformed facet strings, and empty strings,
then explicitly selects Repository. Promoted-root drag regression and protection
are integrated separately in the same work order.

Previously recorded PR validation, supplied by the integrating agent:

- A real demo used four mock tasks with the actual Tags test plugin. The sidebar
  selected Tag, saved and reloaded it, displayed multi-value rows, omitted an
  unused tag, reacted to a live rename, and recovered after disable/re-enable.
- The demo exercised the mobile grouped switcher and range selection from a later
  duplicate occurrence.
- 133 targeted host tests passed. TypeScript passed with an 8192 MiB heap;
  lint, i18n checks, and the production bundle passed.

Post-remediation validation, executed and reported by the integrating agent:

- 37 regression tests passed across five files.
- Documentation catalog and specification validation commands exited successfully.
- The package's documentation coverage preflight returned `covered`.
- Scoped lint identified a repeated test-selector literal; the picker regression
  now uses one shared selector constant. No recheck is claimed for that cleanup.

The earlier 133-test and demo results above remain prior implementation evidence,
distinct from these review-fix regressions. This documentation/picker worker ran
no builds, tests, linters, or formatters.

The failed required run `40105653181` reported: `PR documentation coverage failed
(missing): No changed work order covers the triggering paths.` This package
supplies the missing work order and linked contracts. The CodeRabbit docstring
coverage percentage is a separate advisory; the required artifact gate does not
measure docstrings and was not fixed by blanket comments or PR prose alone.
