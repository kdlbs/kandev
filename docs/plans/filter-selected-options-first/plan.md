---
created: 2026-10-06
status: implemented
requirements:
  - REQ-UI-FILTER-SELECTED-FIRST-001
system_design:
  - ../../specs/ui/system-design/filter-selected-options-first.md
legacy_specs: []
---

# Implementation Plan: Selected Filter Options First

## Overview

Selected-first ordering is implemented in the shared multi-select view filter,
with passing unit, component, desktop, phone, and full-project TypeScript checks.
The full-project check uses Node 22 because Node 24 crashed during earlier attempts.

## Scope

In scope: shared view-filter option presentation, grouped context, controlled
selection updates, search restoration, and desktop/phone checks.

Out of scope: single-value selectors, backend matching, saved-view persistence,
new UI copy, option-source deduplication, and a broader overlay redesign.

## Technical approach

Add a stable selected/unselected partition helper to
`apps/web/components/task/sidebar-filter/filter-option-groups.ts`. Adapt
`FilterMultiSelect` to render the two partitions separately through
`GroupedOptions` when the query is empty, with one separator when both exist.
Track the input query and use the original grouped path for nonempty search.
Reset query state on close. Preserve `OptionRow` identity, indicators, color,
toggle semantics, and summary behavior.

Do not reuse `prioritizeValueOption` from `lib/utils/selector-options.ts`: it
accepts one current value, while this picker must partition multiple selections
before group bucketing. Keep `buildOptionGroups` unchanged for single-select
consumers. Check the actual DOM after query changes because `cmdk` reorders its
results independently of React's array order.

`TypedFilterClauseEditor` shares this picker with `ThreadsViewFilterRow`.
Component tests exercise the shared contract; no Threads query/store changes
are needed. Keep all delivery work in this session unless delegation is
explicitly authorized.

## ASCII UI preview

### UI-01: Repository multi-select, desktop

Entry: Tasks view filter gear, Repository clause with `in`, value trigger.
Illustrative source order: A, B, C, D. Selected values: B and D.

```text
Current                    Proposed (empty search)
+-------------------+      +-------------------+
| Search...         |      | Search...         |
| [ ] Repository A  |      | [x] Repository B  |
| [x] Repository B  |      | [x] Repository D  |
| [ ] Repository C  |      |-------------------|
| [x] Repository D  |      | [ ] Repository A  |
|                   |      | [ ] Repository C  |
+-------------------+      +-------------------+
```

Selected-first placement is required by AC-UI-FILTER-SELECTED-FIRST-001.1;
literal labels and spacing are illustrative. Search stays above the internally
scrolling list. Active search uses existing matching/ranking; clearing it
restores the proposed order (AC-UI-FILTER-SELECTED-FIRST-001.4).

### UI-02: Workflow-step multi-select, grouped

Entry: Workflow step clause with `not_in`; Alpha/Done and Beta/Review selected.

```text
+--------------------------+
| Search...                |
| Alpha                    |
| [x] Done                 |
| Beta                     |
| [x] Review               |
|--------------------------|
| Alpha                    |
| [ ] Ready                |
| Beta                     |
| [ ] Done                 |
+--------------------------+
```

Required: all selected rows precede unselected rows, while headings retain
workflow context (AC-UI-FILTER-SELECTED-FIRST-001.2). Repeated headings are
intentional when a group spans both partitions.

### UI-03: Phone entry and picker

Entry: mobile task picker, filter gear, existing Sidebar filters drawer.

```text
+----------------------------+
| Sidebar filters            |  fixed drawer title
| Repository | in | 2 chosen |  editor scroll body
|    +-------------------+   |
|    | Search...         |   |  existing value popover
|    | [x] Repository B  |   |
|    | [x] Repository D  |   |  internal option scroll
|    |-------------------|   |
|    | [ ] Repository A  |   |
|    | [ ] Repository C  |   |
|    +-------------------+   |
+----------------------------+
```

The existing drawer owns editor scrolling and safe-area clearance. The value
picker uses the same partition logic and contained click/search popover on
phone. Geometry is illustrative; touch selection, containment, dismissal, and
focus return are required by AC-UI-FILTER-SELECTED-FIRST-001.5.

## Tests

| Criteria | Evidence |
| --- | --- |
| .1, .3 | `filter-option-groups.test.ts`: stable partition, empty/all-selected lists, absent IDs, immutable inputs |
| .1, .2 | New `filter-multi-select.test.tsx`: selected rows across multiple groups, repeated labels, no added rows |
| .3, .5 | Component harness: controlled toggle, live reordering, keyboard activation, focus return, opening never calls onChange |
| .4 | Component harness: nonmatching selections excluded, no-results state, clear query and reopen |

All criterion suffixes refer to `AC-UI-FILTER-SELECTED-FIRST-001`.

## E2E tests

Add `apps/web/e2e/tests/task/sidebar-filter-selected-first.spec.ts` for Chromium
and `mobile-sidebar-filter-selected-first.spec.ts` for mobile-chrome. Reuse
`SidebarFilterPopoverPage`, `SessionPage`, and fixture APIs, with a small shared
helper only if the two entry-point scenarios require it.

Seed at least four available repository options with two later options selected;
open the actual value picker and assert DOM order and selected indicators before
scrolling. Toggle a row, search for a nonselected match, clear the query, then
close and reopen. Assert retained membership and selected-first restoration
(.1, .3, .4). Seed two workflows with same-titled steps and assert all selected
steps precede unselected steps with identifiable group context (.2).

On phone, enter through the task-switcher drawer and tap the filter/value
controls. Verify the same order, membership, viewport containment, no document
horizontal overflow, and dismissal back to the editor (.5). Capture a focused
phone screenshot for structural comparison with UI-03.

## Work orders

- [x] [Task 01: Prioritize selected filter options](task-01-prioritize-options.md)

## Verification results

See [Task 01 results](task-01-prioritize-options.md#results) for the exact commands
and evidence. Unit/component tests: 15 passed. Desktop and phone: one focused
test each passed. Lint, targeted TypeScript diagnostics for eight changed files,
i18n checks, specification gates, and public-document checks passed.

The full-project TypeScript check passed using Node 22.23.3 with a 4 GiB heap:
`cd apps/web && pnpm dlx node@22 --max-old-space-size=4096 node_modules/typescript/bin/tsc --noEmit`.
Earlier Node 24 attempts exhausted the default heap, trapped inside V8 with a
4 GiB heap, and segfaulted with jitless enabled. No compiler options or repository
dependencies were changed to obtain the passing result.

## Risks

`cmdk` search ranking and DOM moves need component/browser evidence. Partitioning
only inside each workflow group would fail the global ordering contract.
Missing selected IDs must not be dropped. Reordering can change keyboard focus
when a row crosses partition boundaries, so controlled keyboard toggles require
explicit coverage.

## Documentation impact

The workbench how-to in `docs/public/sessions-and-review.md` now explains
selected-first multi-select ordering and search behavior. No labels, APIs,
operators, or saved-view semantics changed.
