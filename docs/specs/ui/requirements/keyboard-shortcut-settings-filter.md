---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Keyboard Shortcut Settings Filter Requirements

## Overview

Users can narrow the general keyboard shortcut settings list to find an action
without scanning every row. UI owns this local presentation interaction. The
existing [integration navigation](integration-page-shortcuts.md), [plugin
shortcut settings](../../plugins/requirements/plugin-shortcut-settings.md), and
[manual save](settings-manual-save.md) contracts retain their ownership.

## Requirements

### REQ-UI-SHORTCUT-SETTINGS-FILTER-001: Find an action by label

**Intent:** Find and edit a core or integration navigation shortcut on desktop
and phone without changing the underlying shortcut draft by searching.

#### Acceptance criteria

- **AC-UI-SHORTCUT-SETTINGS-FILTER-001.1:** The keyboard shortcut settings card
  shall provide an accessible, localized inline text filter. It shall match a
  substring of the displayed translated core and integration action labels,
  ignoring case and leading and trailing query whitespace. Matching preserves
  the existing row and group order, including dynamically registered integration
  destinations.
- **AC-UI-SHORTCUT-SETTINGS-FILTER-001.2:** An empty or whitespace-only query
  shall show the ordinary complete list. A nonempty query with no matching
  core or integration rows shall show one localized no-match state. A visible,
  accessible clear action for a nonempty query shall restore the ordinary list.
- **AC-UI-SHORTCUT-SETTINGS-FILTER-001.3:** Entering, changing, or clearing the
  query shall only alter visibility; it shall not modify shortcut overrides,
  dirty state, or trigger a save. Pending edits to hidden rows shall remain
  intact and reappear when those rows become visible. Recording, clearing, and
  resetting visible bindings shall retain their existing semantics.
- **AC-UI-SHORTCUT-SETTINGS-FILTER-001.4:** Conflict warnings for visible rows
  shall continue to compare effective bindings against the full core,
  integration, and installed plugin action set, including hidden peers.
- **AC-UI-SHORTCUT-SETTINGS-FILTER-001.5:** Desktop and phone shall expose the
  same search and recovery outcomes in the existing settings scroll surface.
  The search field and clear action shall be keyboard accessible and localized;
  their phone or coarse-pointer hit areas shall be at least 44px, with no
  document horizontal overflow. Fine-pointer desktop controls shall follow the
  standard 28px size.

## Out of scope

Key-chord search, filtering plugin-owned shortcut editors, shortcut dispatch or
precedence changes, recorder redesign, backend, storage, API, or save-lifecycle
changes, and unrelated settings loading or layout work.

## Implementation Plans

- [Keyboard shortcut settings filter](../../../plans/keyboard-shortcut-settings-filter/plan.md)
