---
status: active
system: ui
created: 2026-08-14
owners:
  - Kandev frontend
---
# Responsive PR Detail Header Requirements

## Overview

The PR Details panel can be much narrower than the browser viewport. When header actions consume that width, the pull-request title collapses until the review being shown is no longer identifiable.

## Requirements

### REQ-UI-PR-DETAIL-HEADER-WIDTH-001: Responsive PR Detail Header

**Intent:** The PR Details panel can be much narrower than the browser viewport. When header actions consume that width, the pull-request title collapses until the review being shown is no longer identifiable.

#### Acceptance criteria

- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.1:** The shared change-request detail header keeps the complete review title readable at every supported panel width. Long titles wrap within their available title area instead of clipping with an ellipsis. Wrapping follows normal line flow: each line uses its available width before text continues on the next line; the title is not balanced into equal-length lines.
- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.2:** Header actions, including GitHub's **Approve as...** and **Squash and merge** controls, may share the title row only when the complete title remains on one line beside them.
- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.3:** When the title and action cluster do not fit together, the complete action cluster moves below the title. The title then owns the full row and wraps only when it is longer than that full width. There is no fixed pixel breakpoint; the transition follows actual title and action content.
- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.4:** A stacked action cluster starts at the same leading edge as the title and may wrap internally when needed.
- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.5:** Responsive behavior follows available change-request detail width, independent of browser viewport width, so resized desktop panels, localized action labels, and phone Review surfaces obey the same invariant.
- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.6:** Stacking preserves action order, labels, enabled and pending states, click behavior, accessible names, and existing touch-target sizing.
- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.7:** The header and its controls remain contained without document-level horizontal overflow.
- **AC-UI-PR-DETAIL-HEADER-WIDTH-001.8:** **GIVEN** a desktop PR Details panel where the complete title and actions fit together, **WHEN** the header lays out, **THEN** the single-line title and action group appear inline and every control remains usable.

## Migrated source detail

## Why

The PR Details panel can be much narrower than the browser viewport. When
header actions consume that width, the pull-request title collapses until the
review being shown is no longer identifiable.

## What

- The shared change-request detail header keeps the complete review title
  readable at every supported panel width. Long titles wrap within their
  available title area instead of clipping with an ellipsis. Wrapping follows
  normal line flow: each line uses its available width before text continues
  on the next line; the title is not balanced into equal-length lines.
- Header actions, including GitHub's **Approve as...** and **Squash and merge**
  controls, may share the title row only when the complete title remains on one
  line beside them.
- When the title and action cluster do not fit together, the complete action
  cluster moves below the title. The title then owns the full row and wraps only
  when it is longer than that full width. There is no fixed pixel breakpoint;
  the transition follows actual title and action content.
- A stacked action cluster starts at the same leading edge as the title and may
  wrap internally when needed.
- Responsive behavior follows available change-request detail width,
  independent of browser viewport width, so resized desktop panels, localized
  action labels, and phone Review surfaces obey the same invariant.
- Stacking preserves action order, labels, enabled and pending states, click
  behavior, accessible names, and existing touch-target sizing.
- The header and its controls remain contained without document-level
  horizontal overflow.

## Scenarios

- **GIVEN** a desktop PR Details panel where the complete title and actions fit
  together, **WHEN** the header lays out, **THEN** the single-line title and
  action group appear inline and every control remains usable.
- **GIVEN** the same title and actions in a squeezed panel where sharing a row
  would wrap the title, **WHEN** the header lays out, **THEN** the action cluster
  moves below and the title recovers the full row before wrapping.
- **GIVEN** a title longer than the full available row, **WHEN** it wraps,
  **THEN** no header action occupies space to its right and all actions appear
  below from the same leading edge.
- **GIVEN** a phone-sized task with a linked pull request, **WHEN** the user
  opens Review, **THEN** the title appears above the touch-usable action cluster,
  which may wrap internally, and the document has no horizontal overflow.
- **GIVEN** panel width, title copy, localization, or visible action labels
  change, **WHEN** combined intrinsic width crosses the available width, **THEN**
  the same actions move between stacked and inline placement without being lost
  or reordered.

## Out of scope

- Changing approval or merge eligibility, mutations, labels, or provider data.
- Changing title copy, font size, or font weight.
- Moving actions into an overflow menu or changing the PR Details panel's
  navigation, placement, scrolling, or persistence.

## Implementation plan

[Implementation plan](../../../plans/pr-detail-header-width/plan.md)
