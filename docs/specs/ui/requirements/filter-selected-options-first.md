---
status: active
system: ui
created: 2026-10-06
owners:
  - kandev
---

# Selected Options First in View Filters

## Overview

Users editing a task view need to find its selected filter values without
scanning the whole option list. UI owns this presentation contract because
task views and the reusable filter picker are personal presentation controls;
task identity, filter matching, and workspace access retain their existing owners.
This complements [single-choice picker prominence](selected-option-picker-prominence.md),
whose contract explicitly excludes multi-select lists.

## Requirements

### REQ-UI-FILTER-SELECTED-FIRST-001: Selected filter value prominence

**Intent:** Make the existing selections immediately discoverable in multi-select
view filters, including repository and workflow-step filters.

#### Acceptance criteria

- **AC-UI-FILTER-SELECTED-FIRST-001.1:** When a multi-select view filter opens with an empty search, all available selected options shall precede every unselected option. Moving selected options first shall not add duplicate rows. Within each partition, existing group order and option order shall remain stable. With no selections, the existing order shall remain unchanged.
- **AC-UI-FILTER-SELECTED-FIRST-001.2:** Grouped filters shall retain the group context of both selected and unselected options, including same-named workflow steps from different workflows. Selected options from a later group shall precede unselected options from an earlier group.
- **AC-UI-FILTER-SELECTED-FIRST-001.3:** Toggling an option shall update membership and its selected appearance through the existing filter behavior, keep the picker open, and update the selected-first order while search is empty. Opening or reordering shall not change selection membership, selection-array order, or the saved view. Unavailable selected values shall remain in the filter without inventing selectable options or silently removing values.
- **AC-UI-FILTER-SELECTED-FIRST-001.4:** With nonempty search, the picker shall retain its existing matching and relevance ordering. A selected option excluded by the query shall remain excluded. Clearing search or reopening shall restore selected-first order. An empty result shall use the existing no-options feedback.
- **AC-UI-FILTER-SELECTED-FIRST-001.5:** Desktop and phone view editors shall expose the same ordering through keyboard and touch controls. The picker shall retain selected indicators, distinguish keyboard focus from selection membership, support dismissal and focus return, and remain within the viewport with internal option scrolling.

## Out of scope

Single-value selectors, filter operators and matching rules, saved-view persistence,
repository eligibility, task ordering, new ranking heuristics, a broader picker
redesign, and new user-facing labels. The shared view-filter picker may apply
this presentation to Threads without changing Threads query semantics.

## Implementation plans

- [Selected filter options first](../../../plans/filter-selected-options-first/plan.md)
