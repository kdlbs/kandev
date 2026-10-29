---
status: current
system: ui
requirements:
  - REQ-UI-SHORTCUT-SETTINGS-FILTER-001
---

# Keyboard Shortcut Settings Filter System Design

## Purpose and boundaries

The general keyboard shortcut card owns a transient presentation query. Its
parent `KeyboardShortcutsSettings` and `useShortcutDraft` retain the route draft
and save ownership. Integration and plugin shortcut contracts remain in their
[existing design](integration-page-shortcuts.md) and [plugin design](../../plugins/system-design/plugin-shortcut-settings.md).
No API, persistent state, telemetry, or permission boundary changes.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SHORTCUT-SETTINGS-FILTER-001` | Filtering and conflicts; Responsive interaction; Evidence |

## Filtering and conflicts

`KeyboardShortcutsCard` in `apps/web/components/settings/keyboard-shortcuts-card.tsx`
keeps the query in local React state. Derive displayed core labels from
`CONFIGURABLE_SHORTCUTS` and the current translation function and integration
labels from the current `integrationEntries`. Normalize the query with `trim()`
and `toLocaleLowerCase()` and compare with each displayed label normalized the
same way, as in `sidebar-layout-editor-shortcut-picker.tsx`. Derive both visible
collections from the complete current lists on render so translations and dynamic
registrations update naturally. Preserve their original relative ordering.

`useShortcutConflictLabels` must still receive the complete integrations,
plugin entries, and overrides, and derive its core entries independently of the
visible lists. Apply the query only to rendered rows, never to conflict inputs or
the override map. The input and clear handlers set only local query state; existing
record, reset, clear, and parent `onChange` paths remain unchanged. When the filter receives a key event while a recorder is active, the recorder
ends capture and yields that event to the input without modifying an override.
Clearing the query restores focus to the input. Unmounting a filtered recorder
must not discard an already recorded route draft; ordinary recorder cleanup
applies.

Use shared `Input` and `Button`, existing `settings:searchShortcuts`,
`settings:noMatchingShortcuts`, and suitable existing common search/clear copy.
When a nonblank query has no matches across both collections, render a single
empty message, suppressing an empty integration group. When blank, preserve the
current grouping and help text. Do not add translations if the existing keys
meet the accessible naming needs.

## Responsive interaction

The entry point is Settings > Preferences > Keyboard Shortcuts. Put the input
and query-clear action inline above the rows on both viewports, with a
one-dimensional flow in the existing settings scroll owner. The closest filter
exemplar is `SidebarShortcutPicker`; the curated phone navigation baseline is
`mobile-picker-sheet.tsx`, but this persistent settings content needs no drawer
or overlay. Keep the clear action visible whenever the query has text, with a
localized accessible name; keep the row within the card width. Use the existing
control sizing conventions: 28px at desktop fine pointer and at least 44px for
phone or coarse pointer, including a narrow fine-pointer phone. Preserve the
existing recorder composition and scroll behavior without a recorder sweep.

## Evidence

Before implementation, a real mounted-card component test must fail for the
missing filter, not just for a helper. Cover translated core and integration
labels, normalized queries, blank restoration and order, one no-match state,
clear, no `onChange` from filtering, retained pending override, and conflicts
with hidden core, integration, and plugin entries. Targeted desktop and
configured `mobile-chrome` Playwright tests exercise the actual settings route
and a pending draft, and verify search, clear, empty state, conflict visibility,
phone hitboxes and overflow. Inspect captured desktop and phone screenshots in
addition to assertions. The work order specifies execution and build evidence.

## Implementation Plans

- [Keyboard shortcut settings filter](../../../plans/keyboard-shortcut-settings-filter/plan.md)
