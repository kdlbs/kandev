---
status: implemented
created: 2026-08-22
owner: kandev
---

# Selected option prominence in single-choice pickers

## Why

When a picker contains several models, branches, agents, or related choices,
the current value can appear in the middle of a long list. The check mark alone
does not make the current choice easy to find, so users must scan or scroll
before they can confirm the active configuration.

## What

- Single-choice option lists show the current value as the first row when the
  list is opened without an active search query. The remaining options keep
  their existing source order.
- The ordering behavior applies to the model/configuration picker, the shared
  `Combobox` used by agent, branch, repository, executor, and profile pickers,
  the compact `Pill` used by task-create repository/branch chips, the task mode
  picker, and the dedicated branch picker list.
- If the current value is represented by an unavailable or disabled option, it
  still appears first and remains non-selectable. If there is no current value,
  the source order is unchanged.
- The current row has a persistent selected appearance: a neutral or card-based
  background, a primary boundary or inset ring, normal readable foreground
  text, and the existing check indicator. Hover and keyboard-highlight states
  remain distinct transient states and do not remove the persistent selected
  appearance.
- Selection styling reserves its boundary space so moving between rows does
  not change the list layout. Existing selection, filtering, loading, disabled,
  and dependent-option behavior remains unchanged.
- With an active search or filter query, the picker keeps its existing filtering
  and ranking behavior. When the selected value matches the query, it may be
  prioritized within the matching results; a query that excludes it does not
  force it into the results.
- The same selected-first and selected-state behavior is available on the
  existing phone surfaces. The current choice is visible before scrolling, and
  the change does not add document-level horizontal scrolling or require hover.

## Scenarios

- **GIVEN** the model list is ordered `Default`, `Sonnet`, `Fable`, `Opus` and
  `Opus` is current, **WHEN** the model picker opens without a search query,
  **THEN** `Opus` is the first option, has the persistent selected surface and
  check indicator, and the other models retain their relative order.
- **GIVEN** a dynamic configuration list has `Low`, `Medium`, and `High` and
  `Medium` is current, **WHEN** its nested picker opens, **THEN** `Medium` is
  the first option with the same selected treatment and all three values remain
  selectable according to their existing rules.
- **GIVEN** an agent or repository `Combobox` has its second option selected,
  **WHEN** the list opens without a query, **THEN** that option is first and
  the remaining options retain their original order.
- **GIVEN** a task-create repository or branch chip has a current value after
  other options, **WHEN** its compact picker opens without a query, **THEN**
  the current value is first with the same selected treatment.
- **GIVEN** a branch picker contains the current `main` branch after other
  branches, **WHEN** the branch list opens in a popover or phone sheet,
  **THEN** `main` is the first row and exposes its selected state without
  changing branch selection behavior.
- **GIVEN** a task mode picker has a current mode that is not first in the
  provider list, **WHEN** the menu opens, **THEN** the current mode is first and
  remains visually distinct from the keyboard-highlighted row.
- **GIVEN** a picker has no current value, **WHEN** it opens, **THEN** its
  options retain the existing source order and no row receives selected styling
  solely because it is first.
- **GIVEN** the current value is represented by a disabled unavailable option,
  **WHEN** the picker opens, **THEN** that option is first, visibly selected,
  and still cannot be chosen.
- **GIVEN** a user types a filter query, **WHEN** the current value does not
  match the query, **THEN** the current value is not injected into the results
  and the existing filter ranking is preserved.
- **GIVEN** a narrow touch viewport, **WHEN** the user opens the model or
  branch picker, **THEN** the current row is visible without scrolling, its
  selected treatment is readable, and the picker remains contained within the
  viewport.

## Out of scope

- Changing model, branch, agent, executor, repository, or mode persistence,
  APIs, labels, or selection side effects.
- Reordering multi-select lists, chips, checkboxes, tabs, navigation menus, or
  lists whose order communicates a domain ranking rather than a current choice.
- Adding a new mobile drawer, route, or picker composition. Existing responsive
  surfaces and scroll owners remain in place.
- Ranking options by popularity, capability, provider, or any other heuristic
  beyond prioritizing the current value when no query is active.
