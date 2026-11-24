---
status: active
system: ui
created: 2026-10-28
owners:
  - Kandev
---

# Task Pagination Presentation Requirements

## Overview

List users need to move among result pages without losing controls outside the
visible surface. UI owns this independent presentation contract; task query,
filter, and pagination state remain with their existing owners.

## Requirements

### REQ-UI-TASK-PAGINATION-PRESENTATION-001: Reachable list pagination

**Intent:** Keep task-list page navigation usable on phones and desktop.

#### Acceptance criteria

- **AC-UI-TASK-PAGINATION-PRESENTATION-001.1:** With results spanning multiple
  pages, the task list shall show the current result range and page, rows-per-page
  choice, and available previous, next, and direct page actions. Actions shall
  select the intended page, and unavailable boundary actions shall be disabled.
- **AC-UI-TASK-PAGINATION-PRESENTATION-001.2:** On a phone, including a middle
  page with a long page-number window, every displayed pagination action shall
  be fully visible and reachable without horizontal panning. Phone action targets
  shall be at least 44 CSS pixels in both dimensions and shall not overlap.
  The task-list surface and document shall not gain horizontal overflow from
  pagination.
- **AC-UI-TASK-PAGINATION-PRESENTATION-001.3:** On desktop, the existing direct
  page navigation and first/last actions shall remain available with the current
  compact, single-row presentation. Keyboard users shall retain semantic controls,
  current-page indication, and activation.

## Out of scope

- Changes to task fetching, filtering, page size options, persisted preferences,
  shared control primitives, and navigation outside this pagination surface.
