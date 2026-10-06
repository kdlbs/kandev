---
status: current
system: ui
requirements:
  - REQ-UI-FILTER-SELECTED-FIRST-001
---

# Selected View Filter Options System Design

## Purpose and boundaries

The existing `FilterMultiSelect` renders source options through `GroupedOptions`.
`TypedFilterClauseEditor` chooses it for `in` and `not_in` enum clauses. The same
typed editor serves task views and `ThreadsViewFilterRow`. UI owns this reusable
presentation behavior. Keep the option providers, clause operators, store
mutations, workspace guards, and persistence contracts unchanged.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-FILTER-SELECTED-FIRST-001` | Ordering, Search and interaction, Responsive surface, Verification |

## Ordering

Partition the supplied `MultiSelectOption[]` using the current selected-value
set into two new arrays: selected and unselected. Preserve input order and
option objects; never sort or mutate the input array or `selected` prop.
Keep the helper alongside `filter-option-groups.ts` and its existing tests.

`FilterMultiSelectOptions` in `filter-multi-select-options.tsx` owns the option
rows and grouping, keeping the trigger and summary component focused.
When search is empty, render selected options through `GroupedOptions`, followed
by unselected options through `GroupedOptions`. Use a separator only when both
partitions contain options. Build groups independently within each partition:
sorting a combined list before `buildOptionGroups` would pull unselected members
of a selected group ahead of selected members of later groups.

Preserve existing headings, color dots, unique item identity, and checked state.
A workflow heading can appear in both partitions. No new section headings or
locale keys are needed. Missing selected values are absent from both arrays;
the existing summary and membership continue to retain them.

## Search and interaction

Keep `cmdk` as the matching and ranking owner. Track the `CommandInput` query
locally in `FilterMultiSelect`; with nonempty query render the original options
through the existing grouped path. Do not force-render selected nonmatches or
disable built-in filtering. Clear the local query when the picker closes so
reopening cannot retain stale query-dependent ordering.

Selection stays controlled by `selected` and `onChange`. Preserve the existing
toggle implementation, trigger summary, and open state. Recompute partitions
after controlled selection updates. Item values must still include group,
label, and option identity to avoid collisions between same-named steps.
Keyboard assertions must inspect membership using `data-checked` or
`data-active`, since `cmdk`'s `aria-selected` denotes the active keyboard row.
Test focus continuity across moves between partitions.

## Responsive surface

Reuse `SidebarFilterPopover`: desktop uses its anchored editor, while phone
uses `MobileSidebarFilterSurface` inside the existing task-switcher flow.
The nearest shipped exemplar is that filter drawer, reached from
`mobile-task-picker-trigger` and the drawer's filter gear. It contributes the
fixed title, safe-area padding, and single editor-body scroller.

The value picker keeps its current searchable click popover, an allowed mobile
combobox pattern in the mobile UI language when touch-usable and contained.
This ordering change adds no new navigation or overlay composition. Search
stays above the option list, and `CommandList` remains the option scroll owner.
Option rows use 48px minimum height on phone and coarse pointers so drawer
scaling leaves an actual hit target of at least 44px; desktop keeps its current
density. Preserve shared selection logic and current touch behavior; verify phone
containment and selection by tapping, without relying on hover.

## Persistence and failure behavior

No schema, API, migration, access, or telemetry changes are required. Merely
opening, searching, or reordering cannot call `onChange`. Existing empty-result
feedback and unknown-value retention apply. Actual toggles continue through
the existing workspace-guarded draft update. A returning option becomes eligible
for selected-first presentation on the next render.

## Verification

Unit tests cover stable partitioning, empty/all-selected inputs, unknown IDs,
and input immutability. Component tests exercise actual `cmdk` rendering,
grouped cross-workflow ordering, live toggles, keyboard activation, search and
reopening. Focused browser tests prove the task view editor's real desktop and
phone entry points; shared component coverage proves the Threads reuse.

## Related contracts

- [Workspace task views](workspace-sidebar-task-views.md)
- [Single-choice prominence](../requirements/selected-option-picker-prominence.md)
