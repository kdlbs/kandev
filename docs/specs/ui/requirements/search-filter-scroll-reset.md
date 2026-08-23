---
status: active
system: ui
created: 2026-08-06
owners:
  - kandev
---
# Search/filter dropdown scroll reset Requirements

## Overview

Every search/filter dropdown in the web UI renders its results inside the shared
`@kandev/ui/command` list (a `cmdk` list wrapped in a scrollable `div`). When the
user scrolls that list and then types (or edits) the search query, `cmdk`
re-filters and re-sorts the items and re-renders, but it never resets the list's
scroll offset. The viewport stays parked at the previous `scrollTop`, so the
newly filtered results can render with the top entries scrolled out of view and a
scrollbar thumb sitting in the middle of a now-short list. This is visible in the
Command Center (command palette) and the model selector, and affects the shared
combobox and filter multi-select that build on the same primitive.

## Requirements

### REQ-UI-SEARCH-FILTER-SCROLL-RESET-001: Search/filter dropdown scroll reset

**Intent:** Every search/filter dropdown in the web UI renders its results inside the shared
`@kandev/ui/command` list (a `cmdk` list wrapped in a scrollable `div`). When the user scrolls that
list and then types (or edits) the search query, `cmdk` re-filters and re-sorts the items and
re-renders, but it never resets the list's scroll offset. The viewport stays parked at the previous
`scrollTop`, so the newly filtered results can render with the top entries scrolled out of view and
a scrollbar thumb sitting in the middle of a now-short list. This is visible in the Command Center
(command palette) and the model selector, and affects the shared combobox and filter multi-select
that build on the same primitive.

#### Acceptance criteria

- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.1:** Whenever the active search query of a `@kandev/ui/command` list changes, the list's scroll position resets to the top (`scrollTop = 0`) so the highest-ranked filtered result is visible.
- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.2:** This behavior is provided centrally by the shared `CommandList` primitive, so it applies to every consumer without per-call-site changes: the Command Center (`command-panel`), the task model selector (`model-config-selector`), the settings model/mode comboboxes, the generic `combobox`, and the sidebar filter multi-select.
- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.3:** The reset fires only when the query text changes. Highlighting a different item with the arrow keys, opening/closing the popover, or selecting an item does not force the list back to the top, so `cmdk`'s existing keyboard auto-scroll to the active item is preserved.
- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.4:** Clearing the query (query becomes empty) also resets the list to the top.
- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.5:** Existing filtering, sorting, item selection, keyboard navigation, grouping, `forceMount`, and `shouldFilter={false}` behavior are unchanged.
- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.6:** **GIVEN** the Command Center is open with more results than fit in the list and the user has scrolled part-way down, **WHEN** the user types another character that changes the filter, **THEN** the results list is scrolled back to the top and the first matching result is visible.
- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.7:** **GIVEN** the task model selector dropdown is open and scrolled down a long model list, **WHEN** the user types in the filter box, **THEN** the filtered model list is scrolled to the top.
- **AC-UI-SEARCH-FILTER-SCROLL-RESET-001.8:** **GIVEN** a filtered list scrolled to the top, **WHEN** the user presses the Down arrow to move the highlight past the visible area, **THEN** the list scrolls to follow the highlighted item and is NOT forced back to the top.

## Migrated source detail

## Why

Every search/filter dropdown in the web UI renders its results inside the shared
`@kandev/ui/command` list (a `cmdk` list wrapped in a scrollable `div`). When the
user scrolls that list and then types (or edits) the search query, `cmdk`
re-filters and re-sorts the items and re-renders, but it never resets the list's
scroll offset. The viewport stays parked at the previous `scrollTop`, so the
newly filtered results can render with the top entries scrolled out of view and a
scrollbar thumb sitting in the middle of a now-short list. This is visible in the
Command Center (command palette) and the model selector, and affects the shared
combobox and filter multi-select that build on the same primitive.

## What

- Whenever the active search query of a `@kandev/ui/command` list changes, the
  list's scroll position resets to the top (`scrollTop = 0`) so the highest-ranked
  filtered result is visible.
- This behavior is provided centrally by the shared `CommandList` primitive, so it
  applies to every consumer without per-call-site changes: the Command Center
  (`command-panel`), the task model selector (`model-config-selector`), the
  settings model/mode comboboxes, the generic `combobox`, and the sidebar filter
  multi-select.
- The reset fires only when the query text changes. Highlighting a different item
  with the arrow keys, opening/closing the popover, or selecting an item does not
  force the list back to the top, so `cmdk`'s existing keyboard auto-scroll to the
  active item is preserved.
- Clearing the query (query becomes empty) also resets the list to the top.
- Existing filtering, sorting, item selection, keyboard navigation, grouping,
  `forceMount`, and `shouldFilter={false}` behavior are unchanged.

## Scenarios

- **GIVEN** the Command Center is open with more results than fit in the list and
  the user has scrolled part-way down, **WHEN** the user types another character
  that changes the filter, **THEN** the results list is scrolled back to the top
  and the first matching result is visible.
- **GIVEN** the task model selector dropdown is open and scrolled down a long
  model list, **WHEN** the user types in the filter box, **THEN** the filtered
  model list is scrolled to the top.
- **GIVEN** a filtered list scrolled to the top, **WHEN** the user presses the
  Down arrow to move the highlight past the visible area, **THEN** the list scrolls
  to follow the highlighted item and is NOT forced back to the top.
- **GIVEN** a search query with results scrolled down, **WHEN** the user clears the
  query, **THEN** the (unfiltered) list is shown scrolled to the top.
- **GIVEN** a `CommandList` given an external `ref` or scroll props by a consumer,
  **WHEN** the scroll-reset behavior runs, **THEN** the consumer's `ref` still
  receives the underlying list element and consumer scroll props still apply.

## Out of scope

- Persisting or restoring scroll position across dropdown open/close cycles.
- Changing filtering/sorting/scoring, grouping, or selection semantics of any
  dropdown.
- Non-`cmdk` scrollable surfaces (chat transcript, file tree, settings pages).
- Visual restyling of the scrollbar or list.

## Failure modes

- `CommandList` renders inside a `cmdk` `Command` context in every current
  consumer; the scroll-reset subscription depends on that context. Rendering
  `CommandList` outside a `Command` is unsupported and out of scope.
